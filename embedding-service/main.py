#!/usr/bin/env python3
"""
Embedding service for scoop.

Supports two request formats:
- POST /embed with {"texts": ["..."], "max_length": 512}
- POST /v1/embeddings with {"input": ["..."]} (OpenAI-compatible)

Backends:
- transformers: local HF model inference (default)
- deterministic: fast local/test fallback with deterministic 4096-d vectors
"""

from __future__ import annotations

import argparse
import hashlib
import json
import logging
import math
import os
import signal
import sys
import threading
import time
from collections.abc import Sequence
from dataclasses import dataclass
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Protocol

DEFAULT_HOST = "0.0.0.0"
DEFAULT_PORT = 8844
DEFAULT_MODEL_NAME = "Qwen/Qwen3-Embedding-8B"
DEFAULT_MODEL_KEY = "Qwen3-Embedding-8B"
DEFAULT_DTYPE = "float16"
DEFAULT_BACKEND = "transformers"
DEFAULT_MAX_LENGTH = 512
DEFAULT_MAX_ITEMS = 64
DEFAULT_MAX_BODY_BYTES = 2_000_000
PIPELINE_VECTOR_DIMENSIONS = 4096


def _env(name: str, fallback: str) -> str:
    value = os.getenv(name, "").strip()
    if value == "":
        return fallback
    return value


def _env_int(name: str, fallback: int) -> int:
    raw = os.getenv(name, "").strip()
    if raw == "":
        return fallback
    try:
        return int(raw)
    except ValueError:
        return fallback


@dataclass(frozen=True)
class Settings:
    host: str = DEFAULT_HOST
    port: int = DEFAULT_PORT
    backend: str = DEFAULT_BACKEND
    model_name: str = DEFAULT_MODEL_NAME
    model_key: str = DEFAULT_MODEL_KEY
    dtype: str = DEFAULT_DTYPE
    max_length: int = DEFAULT_MAX_LENGTH
    max_items: int = DEFAULT_MAX_ITEMS
    max_body_bytes: int = DEFAULT_MAX_BODY_BYTES
    log_level: str = "INFO"


class EmbeddingBackend(Protocol):
    name: str
    dimensions: int

    def embed(self, texts: Sequence[str], *, max_length: int) -> list[list[float]]:
        raise NotImplementedError

    def health(self) -> dict[str, Any]:
        raise NotImplementedError


class DeterministicBackend:
    name = "deterministic"
    dimensions = PIPELINE_VECTOR_DIMENSIONS

    def __init__(self, model_name: str) -> None:
        self.model_name = model_name

    def embed(self, texts: Sequence[str], *, max_length: int) -> list[list[float]]:
        _ = max_length
        return [self._vector_for_text(text) for text in texts]

    def health(self) -> dict[str, Any]:
        return {
            "backend": self.name,
            "model": self.model_name,
            "device": "cpu",
            "dimensions": self.dimensions,
        }

    def _vector_for_text(self, text: str) -> list[float]:
        values: list[float] = []
        counter = 0
        while len(values) < self.dimensions:
            digest = hashlib.sha256(f"{text}\n{counter}".encode()).digest()
            for idx in range(0, len(digest), 2):
                pair = digest[idx : idx + 2]
                if len(pair) < 2:
                    pair = pair + b"\x00"
                raw = int.from_bytes(pair, byteorder="little", signed=False)
                value = (raw / 32767.5) - 1.0
                values.append(value)
                if len(values) >= self.dimensions:
                    break
            counter += 1

        norm = math.sqrt(sum(value * value for value in values))
        if norm == 0:
            values[0] = 1.0
            norm = 1.0
        return [value / norm for value in values]


class TransformersBackend:
    name = "transformers"

    def __init__(self, model_name: str, dtype: str) -> None:
        self.model_name = model_name
        self.dtype = dtype
        self._lock = threading.Lock()
        self._torch: Any | None = None
        self._tokenizer: Any | None = None
        self._model: Any | None = None
        self.device = "cpu"
        self.dimensions = 0
        self._load_model()

    def _load_model(self) -> None:
        import torch
        from transformers import AutoModel, AutoTokenizer

        self._torch = torch
        self.device = "cuda" if torch.cuda.is_available() else "cpu"

        dtype_map = {
            "float16": torch.float16,
            "bfloat16": torch.bfloat16,
            "float32": torch.float32,
        }
        torch_dtype = dtype_map.get(self.dtype, torch.float16)

        started = time.perf_counter()
        logging.info(
            "Loading model backend=transformers model=%s device=%s dtype=%s",
            self.model_name,
            self.device,
            self.dtype,
        )
        self._tokenizer = AutoTokenizer.from_pretrained(self.model_name, trust_remote_code=True)
        self._model = AutoModel.from_pretrained(
            self.model_name,
            dtype=torch_dtype,
            trust_remote_code=True,
        ).to(self.device)
        self._model.eval()
        loaded_ms = (time.perf_counter() - started) * 1000

        hidden_size = getattr(getattr(self._model, "config", None), "hidden_size", 0)
        self.dimensions = int(hidden_size) if int(hidden_size) > 0 else PIPELINE_VECTOR_DIMENSIONS
        logging.info("Model loaded in %.1fms dimensions=%d", loaded_ms, self.dimensions)

    def embed(self, texts: Sequence[str], *, max_length: int) -> list[list[float]]:
        if len(texts) == 0:
            return []
        if self._torch is None or self._tokenizer is None or self._model is None:
            raise RuntimeError("transformers backend not initialized")

        torch = self._torch
        with self._lock:
            encoded = self._tokenizer(
                list(texts),
                padding=True,
                truncation=True,
                max_length=max_length,
                return_tensors="pt",
            ).to(self.device)

            with torch.no_grad():
                outputs = self._model(**encoded)
                attention_mask = encoded["attention_mask"]
                last_token_indices = attention_mask.sum(dim=1) - 1
                batch_size = outputs.last_hidden_state.shape[0]
                embeddings = outputs.last_hidden_state[
                    torch.arange(batch_size, device=self.device),
                    last_token_indices,
                ]
                embeddings = torch.nn.functional.normalize(embeddings, p=2, dim=1)
            return _coerce_embedding_vectors(embeddings.cpu().float().tolist())

    def health(self) -> dict[str, Any]:
        return {
            "backend": self.name,
            "model": self.model_name,
            "device": self.device,
            "dimensions": self.dimensions,
        }


def parse_input_texts(data: dict[str, Any], *, max_items: int) -> list[str]:
    payload = data.get("texts")
    if payload is None and "input" in data:
        payload = data["input"]
    if payload is None:
        raise ValueError("missing 'texts' or 'input' field")

    if isinstance(payload, str):
        texts = [payload]
    elif isinstance(payload, list):
        texts = payload
    else:
        raise ValueError("'texts'/'input' must be a string or array of strings")

    cleaned: list[str] = []
    for index, item in enumerate(texts):
        if not isinstance(item, str):
            raise ValueError(f"text at index {index} must be a string")
        text = item.strip()
        if text != "":
            cleaned.append(text)

    if len(cleaned) == 0:
        raise ValueError("request contains no non-empty texts")
    if len(cleaned) > max_items:
        raise ValueError(f"too many texts: got {len(cleaned)}, max {max_items}")
    return cleaned


def _coerce_embedding_vectors(value: Any) -> list[list[float]]:
    if not isinstance(value, list):
        raise RuntimeError("embedding backend returned a non-list payload")

    vectors: list[list[float]] = []
    for row_index, row in enumerate(value):
        if not isinstance(row, list):
            raise RuntimeError(f"embedding row {row_index} is not a list")
        vector: list[float] = []
        for column_index, item in enumerate(row):
            if not isinstance(item, int | float):
                raise RuntimeError(f"embedding value [{row_index}][{column_index}] is not numeric")
            vector.append(float(item))
        vectors.append(vector)
    return vectors


def parse_max_length(data: dict[str, Any], default_value: int) -> int:
    raw = data.get("max_length", default_value)
    try:
        value = int(raw)
    except (TypeError, ValueError):
        return default_value
    if value < 8:
        return 8
    if value > 4096:
        return 4096
    return value


@dataclass
class ServiceState:
    settings: Settings
    backend: EmbeddingBackend
    started_at: float


def create_handler(state: ServiceState) -> type[BaseHTTPRequestHandler]:
    class Handler(BaseHTTPRequestHandler):
        server_version = "embedding-service/1.0"

        def do_GET(self) -> None:
            if self.path != "/health":
                self._write_error(HTTPStatus.NOT_FOUND, "not found")
                return
            self._write_json(HTTPStatus.OK, self._health_payload())

        def do_POST(self) -> None:
            if self.path not in ("/embed", "/v1/embeddings"):
                self._write_error(HTTPStatus.NOT_FOUND, "not found")
                return

            try:
                body = self._read_json()
                texts = parse_input_texts(body, max_items=state.settings.max_items)
                max_length = parse_max_length(body, state.settings.max_length)
            except ValueError as exc:
                self._write_error(HTTPStatus.BAD_REQUEST, str(exc))
                return
            except json.JSONDecodeError:
                self._write_error(HTTPStatus.BAD_REQUEST, "invalid JSON body")
                return

            started = time.perf_counter()
            try:
                vectors = state.backend.embed(texts, max_length=max_length)
            except Exception as exc:  # noqa: BLE001
                logging.exception("embedding inference failed")
                self._write_error(HTTPStatus.INTERNAL_SERVER_ERROR, f"inference failed: {exc}")
                return
            elapsed_ms = round((time.perf_counter() - started) * 1000, 2)

            if len(vectors) != len(texts):
                self._write_error(
                    HTTPStatus.INTERNAL_SERVER_ERROR,
                    f"backend returned {len(vectors)} embeddings for {len(texts)} texts",
                )
                return
            if len(vectors) > 0 and len(vectors[0]) != state.backend.dimensions:
                self._write_error(
                    HTTPStatus.INTERNAL_SERVER_ERROR,
                    "dimension mismatch: "
                    f"got {len(vectors[0])}, expected {state.backend.dimensions}",
                )
                return

            payload: dict[str, Any]
            if self.path == "/v1/embeddings":
                payload = {
                    "object": "list",
                    "data": [
                        {"object": "embedding", "index": index, "embedding": vector}
                        for index, vector in enumerate(vectors)
                    ],
                    "model": state.settings.model_name,
                    "usage": {"prompt_tokens": 0, "total_tokens": 0},
                }
            else:
                payload = {
                    "embeddings": vectors,
                    "model": state.settings.model_name,
                    "dimensions": state.backend.dimensions,
                    "count": len(vectors),
                    "elapsed_ms": elapsed_ms,
                }
            self._write_json(HTTPStatus.OK, payload)

        def _health_payload(self) -> dict[str, Any]:
            return {
                "status": "ok",
                "uptime_seconds": round(time.time() - state.started_at, 3),
                "pipeline_dimensions": PIPELINE_VECTOR_DIMENSIONS,
                **state.backend.health(),
            }

        def _read_json(self) -> dict[str, Any]:
            raw_length = self.headers.get("Content-Length", "0").strip()
            try:
                content_length = int(raw_length)
            except ValueError as exc:
                raise ValueError("invalid Content-Length header") from exc

            if content_length <= 0:
                raise ValueError("empty request body")
            if content_length > state.settings.max_body_bytes:
                raise ValueError(
                    "request body too large: "
                    f"{content_length} bytes (max {state.settings.max_body_bytes})"
                )
            raw_body = self.rfile.read(content_length)
            decoded = json.loads(raw_body.decode("utf-8"))
            if not isinstance(decoded, dict):
                raise ValueError("request body must be a JSON object")
            return decoded

        def _write_json(self, status: HTTPStatus, payload: dict[str, Any]) -> None:
            body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
            self.send_response(status.value)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def _write_error(self, status: HTTPStatus, message: str) -> None:
            self._write_json(status, {"error": message})

        def log_message(self, fmt: str, *args: Any) -> None:
            logging.info("%s - - %s", self.address_string(), fmt % args)

    return Handler


def create_backend(settings: Settings) -> EmbeddingBackend:
    if settings.backend == "deterministic":
        backend: EmbeddingBackend = DeterministicBackend(settings.model_name)
    elif settings.backend == "transformers":
        backend = TransformersBackend(settings.model_name, settings.dtype)
    else:
        raise ValueError(f"unsupported backend: {settings.backend}")

    if backend.dimensions != PIPELINE_VECTOR_DIMENSIONS:
        raise RuntimeError(
            "backend dimension mismatch: "
            f"got {backend.dimensions}, expected {PIPELINE_VECTOR_DIMENSIONS} "
            "for scoop pgvector schema"
        )
    return backend


def run_server(settings: Settings) -> int:
    backend = create_backend(settings)
    state = ServiceState(settings=settings, backend=backend, started_at=time.time())
    server = ThreadingHTTPServer((settings.host, settings.port), create_handler(state))
    server.daemon_threads = True

    def _shutdown(_signum: int, _frame: Any) -> None:
        logging.info("Signal received, shutting down server")
        threading.Thread(target=server.shutdown, daemon=True).start()

    signal.signal(signal.SIGINT, _shutdown)
    signal.signal(signal.SIGTERM, _shutdown)

    logging.info(
        "Embedding service started host=%s port=%d backend=%s model=%s",
        settings.host,
        settings.port,
        settings.backend,
        settings.model_name,
    )
    logging.info("Endpoints: GET /health, POST /embed, POST /v1/embeddings")
    try:
        server.serve_forever(poll_interval=0.5)
    finally:
        server.server_close()
        logging.info("Embedding service stopped")
    return 0


def run_check(settings: Settings) -> int:
    print("Embedding service config")
    print(f"  backend: {settings.backend}")
    print(f"  model: {settings.model_name}")
    print(f"  dtype: {settings.dtype}")
    print(f"  max_length: {settings.max_length}")
    print(f"  expected_dimensions: {PIPELINE_VECTOR_DIMENSIONS}")
    if settings.backend == "transformers":
        try:
            import torch
        except Exception as exc:  # noqa: BLE001
            print(f"  torch: unavailable ({exc})")
            return 1
        print(f"  torch_version: {torch.__version__}")
        print(f"  cuda_available: {torch.cuda.is_available()}")
        if torch.cuda.is_available():
            print(f"  cuda_version: {torch.version.cuda}")
            print(f"  gpu_name: {torch.cuda.get_device_name(0)}")
    return 0


def run_cli(settings: Settings) -> int:
    raw = sys.stdin.read().strip()
    if raw == "":
        print("[]")
        return 0

    try:
        parsed = json.loads(raw)
    except json.JSONDecodeError:
        parsed = {"texts": [line.strip() for line in raw.splitlines() if line.strip() != ""]}

    if isinstance(parsed, str):
        parsed = {"texts": [parsed]}
    elif isinstance(parsed, list):
        parsed = {"texts": parsed}
    elif not isinstance(parsed, dict):
        print("[]")
        return 0

    texts = parse_input_texts(parsed, max_items=settings.max_items)
    max_length = parse_max_length(parsed, settings.max_length)
    backend = create_backend(settings)
    vectors = backend.embed(texts, max_length=max_length)
    print(json.dumps(vectors))
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Embedding service for scoop")
    parser.add_argument("--server", action="store_true", help="Run HTTP embedding service")
    parser.add_argument("--check", action="store_true", help="Print runtime check info and exit")
    parser.add_argument("--host", default=_env("EMBED_HOST", DEFAULT_HOST))
    parser.add_argument("--port", type=int, default=_env_int("EMBED_PORT", DEFAULT_PORT))
    parser.add_argument(
        "--backend",
        choices=["transformers", "deterministic"],
        default=_env("EMBED_BACKEND", DEFAULT_BACKEND),
    )
    parser.add_argument(
        "--model",
        dest="model_name",
        default=_env("EMBED_MODEL_NAME", DEFAULT_MODEL_NAME),
    )
    parser.add_argument("--model-key", default=_env("EMBED_MODEL_KEY", DEFAULT_MODEL_KEY))
    parser.add_argument(
        "--dtype",
        choices=["float16", "bfloat16", "float32"],
        default=_env("EMBED_DTYPE", DEFAULT_DTYPE),
    )
    parser.add_argument(
        "--max-length",
        type=int,
        default=_env_int("EMBED_MAX_LENGTH", DEFAULT_MAX_LENGTH),
    )
    parser.add_argument(
        "--max-items",
        type=int,
        default=_env_int("EMBED_MAX_ITEMS", DEFAULT_MAX_ITEMS),
    )
    parser.add_argument(
        "--max-body-bytes",
        type=int,
        default=_env_int("EMBED_MAX_BODY_BYTES", DEFAULT_MAX_BODY_BYTES),
    )
    parser.add_argument("--log-level", default=_env("EMBED_LOG_LEVEL", "INFO"))
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    settings = Settings(
        host=args.host,
        port=args.port,
        backend=args.backend,
        model_name=args.model_name,
        model_key=args.model_key,
        dtype=args.dtype,
        max_length=max(8, int(args.max_length)),
        max_items=max(1, int(args.max_items)),
        max_body_bytes=max(1024, int(args.max_body_bytes)),
        log_level=args.log_level.upper(),
    )

    logging.basicConfig(
        level=getattr(logging, settings.log_level, logging.INFO),
        format="%(asctime)s %(levelname)s %(message)s",
    )

    try:
        if args.check:
            return run_check(settings)
        if args.server:
            return run_server(settings)
        return run_cli(settings)
    except Exception as exc:  # noqa: BLE001
        logging.error("embedding service failed: %s", exc)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
