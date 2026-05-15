#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
backend_root="$(cd "$script_dir/.." && pwd)"

cd "$backend_root"

go run github.com/unclebob/mutate4go/cmd/mutate4go@latest internal/language/normalize.go --scan
go run github.com/unclebob/mutate4go/cmd/mutate4go@latest internal/reader/preview.go --scan
go run github.com/unclebob/mutate4go/cmd/mutate4go@latest internal/httpapi/tag_handlers.go --scan
go run github.com/unclebob/mutate4go/cmd/mutate4go@latest internal/httpapi/person_identity_handlers.go --scan
