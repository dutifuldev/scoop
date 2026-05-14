from __future__ import annotations

import json
import math
import threading
import time
from http.client import HTTPConnection, HTTPResponse
from http.server import ThreadingHTTPServer
from typing import Any

import pytest

from main import (
    PIPELINE_VECTOR_DIMENSIONS,
    DeterministicBackend,
    ServiceState,
    Settings,
    _coerce_embedding_vectors,
    create_backend,
    create_handler,
    parse_input_texts,
    parse_max_length,
)


def test_parse_input_texts_accepts_embed_and_openai_payloads() -> None:
    assert parse_input_texts({"texts": [" alpha ", "", "beta"]}, max_items=3) == [
        "alpha",
        "beta",
    ]
    assert parse_input_texts({"input": " single "}, max_items=1) == ["single"]


@pytest.mark.parametrize(
    ("payload", "message"),
    [
        ({}, "missing 'texts' or 'input' field"),
        ({"texts": 7}, "'texts'/'input' must be a string or array of strings"),
        ({"texts": ["ok", 7]}, "text at index 1 must be a string"),
        ({"texts": ["", "   "]}, "request contains no non-empty texts"),
        ({"texts": ["a", "b"]}, "too many texts: got 2, max 1"),
    ],
)
def test_parse_input_texts_rejects_invalid_payloads(payload: dict[str, Any], message: str) -> None:
    with pytest.raises(ValueError, match=message):
        parse_input_texts(payload, max_items=1)


@pytest.mark.parametrize(
    ("payload", "expected"),
    [
        ({}, 512),
        ({"max_length": "128"}, 128),
        ({"max_length": "bad"}, 512),
        ({"max_length": 2}, 8),
        ({"max_length": 10_000}, 4096),
    ],
)
def test_parse_max_length_clamps_to_supported_range(payload: dict[str, Any], expected: int) -> None:
    assert parse_max_length(payload, 512) == expected


def test_deterministic_backend_returns_unit_vectors_with_stable_dimensions() -> None:
    backend = DeterministicBackend("test-model")

    first = backend.embed(["same text"], max_length=32)[0]
    second = backend.embed(["same text"], max_length=4096)[0]

    assert first == second
    assert len(first) == PIPELINE_VECTOR_DIMENSIONS
    assert math.isclose(math.sqrt(sum(value * value for value in first)), 1.0, rel_tol=1e-6)


def test_coerce_embedding_vectors_validates_dynamic_backend_output() -> None:
    assert _coerce_embedding_vectors([[1, 2.5]]) == [[1.0, 2.5]]

    with pytest.raises(RuntimeError, match="embedding value \\[0\\]\\[0\\] is not numeric"):
        _coerce_embedding_vectors([["bad"]])


def test_create_backend_rejects_unknown_backend() -> None:
    settings = Settings(backend="missing")

    with pytest.raises(ValueError, match="unsupported backend: missing"):
        create_backend(settings)


def test_handler_serves_health_and_embedding_endpoints() -> None:
    backend = DeterministicBackend("test-model")
    settings = Settings(backend="deterministic", model_name="test-model", max_items=2)
    state = ServiceState(settings=settings, backend=backend, started_at=time.time())
    server = ThreadingHTTPServer(("127.0.0.1", 0), create_handler(state))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    try:
        host, port = server.server_address
        health = _json_request(host, port, "GET", "/health")
        assert health["status"] == "ok"
        assert health["backend"] == "deterministic"
        assert health["dimensions"] == PIPELINE_VECTOR_DIMENSIONS

        embed = _json_request(
            host,
            port,
            "POST",
            "/embed",
            {"texts": ["one", "two"], "max_length": 16},
        )
        assert embed["model"] == "test-model"
        assert embed["count"] == 2
        assert embed["dimensions"] == PIPELINE_VECTOR_DIMENSIONS
        assert len(embed["embeddings"]) == 2
        assert len(embed["embeddings"][0]) == PIPELINE_VECTOR_DIMENSIONS

        openai = _json_request(host, port, "POST", "/v1/embeddings", {"input": "one"})
        assert openai["object"] == "list"
        assert openai["data"][0]["index"] == 0
        assert len(openai["data"][0]["embedding"]) == PIPELINE_VECTOR_DIMENSIONS
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def _json_request(
    host: str,
    port: int,
    method: str,
    path: str,
    payload: dict[str, Any] | None = None,
) -> dict[str, Any]:
    body = b""
    headers: dict[str, str] = {}
    if payload is not None:
        body = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
        headers["Content-Length"] = str(len(body))

    connection = HTTPConnection(host, port, timeout=5)
    try:
        connection.request(method, path, body=body, headers=headers)
        response = connection.getresponse()
        return _decode_json_response(response)
    finally:
        connection.close()


def _decode_json_response(response: HTTPResponse) -> dict[str, Any]:
    assert response.status == 200
    decoded = json.loads(response.read().decode("utf-8"))
    assert isinstance(decoded, dict)
    return decoded
