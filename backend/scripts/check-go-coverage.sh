#!/usr/bin/env bash
set -euo pipefail

minimum_coverage="${GO_COVERAGE_THRESHOLD:-85}"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

coverage_file="$workdir/coverage.out"

go test ./... -count=1 -covermode=atomic -coverprofile="$coverage_file"
total="$(go tool cover -func="$coverage_file" | awk '/^total:/ {print substr($3, 1, length($3)-1)}')"

awk -v total="$total" -v minimum="$minimum_coverage" 'BEGIN {
  if (total + 0 < minimum + 0) {
    printf "coverage %.1f%% is below required %.1f%%\n", total, minimum > "/dev/stderr"
    exit 1
  }
  printf "coverage %.1f%% meets required %.1f%%\n", total, minimum
}'
