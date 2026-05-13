#!/usr/bin/env bash
set -euo pipefail

maximum_score="${GO_CRAP_MAX_SCORE:-30}"
if [[ -n "${GO_CRAP_FILTERS:-}" ]]; then
  # shellcheck disable=SC2206
  filters=(${GO_CRAP_FILTERS})
else
  filters=(
    internal/auth
    internal/cli
    internal/config
    internal/globaltime
    internal/langdetect
    internal/language
    internal/logging
    internal/normalize
    internal/reader
    schema
  )
fi

go run github.com/unclebob/crap4go/cmd/crap4go@latest "${filters[@]}" | awk -v maximum="$maximum_score" '
  BEGIN { failed = 0 }
  /^[[:space:]]*$/ || /^CRAP Report/ || /^=+/ || /^Function/ || /^-+/ { print; next }
  {
    score = $NF + 0
    print
    if (score > maximum) {
      failed = 1
    }
  }
  END {
    if (failed) {
      printf "CRAP score exceeds maximum %.1f\n", maximum > "/dev/stderr"
      exit 1
    }
  }
'
