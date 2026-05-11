#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log() {
  printf '[bootstrap] %s\n' "$1"
}

fail() {
  printf '[bootstrap] ERROR: %s\n' "$1" >&2
  exit 1
}

ensure_golangci_lint() {
  local gobin=""

  if command -v golangci-lint >/dev/null 2>&1; then
    log "golangci-lint already installed: $(golangci-lint version | head -n1)"
    return
  fi

  command -v go >/dev/null 2>&1 || fail "Go is required. Install it from https://go.dev/dl/ and rerun."

  gobin="$(go env GOBIN)"
  if [ -z "$gobin" ]; then
    gobin="$(go env GOPATH)/bin"
  fi
  if [ -x "${gobin}/golangci-lint" ]; then
    case ":$PATH:" in
      *":$gobin:"*) ;;
      *) export PATH="$gobin:$PATH" ;;
    esac
    log "golangci-lint already installed at ${gobin}/golangci-lint"
    return
  fi

  log "Installing golangci-lint via go install..."
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

  case ":$PATH:" in
    *":$gobin:"*) ;;
    *) export PATH="$gobin:$PATH" ;;
  esac

  if ! command -v golangci-lint >/dev/null 2>&1; then
    fail "golangci-lint was installed to ${gobin} but is not on PATH. Add it and rerun."
  fi

  log "Installed golangci-lint: $(golangci-lint version | head -n1)"
}

ensure_pnpm() {
  if command -v pnpm >/dev/null 2>&1; then
    log "pnpm already installed: v$(pnpm --version)"
    return
  fi

  command -v corepack >/dev/null 2>&1 || fail "pnpm is missing and Corepack is unavailable. Run: corepack enable && corepack prepare pnpm@latest --activate"

  log "pnpm not found; enabling Corepack..."
  corepack enable
  log "Preparing latest pnpm via Corepack..."
  corepack prepare pnpm@latest --activate
  hash -r

  command -v pnpm >/dev/null 2>&1 || fail "Corepack could not activate pnpm. Run: corepack enable && corepack prepare pnpm@latest --activate"
  log "pnpm ready: v$(pnpm --version)"
}

install_node_dependencies() {
  log "Installing root Node dependencies..."
  (
    cd "$ROOT_DIR"
    CI=1 pnpm install --no-frozen-lockfile --ignore-scripts
  )

  log "Installing frontend Node dependencies..."
  (
    cd "$ROOT_DIR/frontend"
    CI=1 pnpm install --no-frozen-lockfile
  )
}

install_husky_hooks() {
  log "Installing Husky hooks..."
  (
    cd "$ROOT_DIR"
    pnpm exec husky >/dev/null
  )

  if [ ! -f "$ROOT_DIR/.husky/_/pre-commit" ]; then
    fail "Husky hook installation failed: .husky/_/pre-commit not found."
  fi

  log "Husky hooks installed."
}

main() {
  log "Starting bootstrap..."
  ensure_golangci_lint
  ensure_pnpm
  install_node_dependencies
  install_husky_hooks
  log "Bootstrap complete."
}

main "$@"
