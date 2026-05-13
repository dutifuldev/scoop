#!/usr/bin/env bash
set -euo pipefail

maximum_candidates="${GO_DRY_MAX_CANDIDATES:-55}"
report_file="$(mktemp)"
trap 'rm -f "$report_file"' EXIT

mapfile -t go_files < <(find cmd internal schema -name '*.go' ! -name '*_test.go' -print | sort)

go run github.com/unclebob/dry4go/cmd/dry4go@latest \
  --format json \
  "${go_files[@]}" >"$report_file"

candidate_count="$(python3 - "$report_file" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    payload = json.load(handle)
print(len(payload.get("candidates", [])))
PY
)"

if [ "$candidate_count" -gt "$maximum_candidates" ]; then
  cat "$report_file"
  echo "DRY candidates $candidate_count exceeds maximum $maximum_candidates" >&2
  exit 1
fi

echo "DRY candidates: $candidate_count; maximum: $maximum_candidates"
