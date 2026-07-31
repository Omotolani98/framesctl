#!/bin/sh
set -eu

binary="${1:-apps/framesplayer/dist/framesplayer-darwin-arm64}"
output_file="${TMPDIR:-/tmp}/framesplayer-smoke.$$.out"

cleanup() {
  rm -f "$output_file"
}
trap cleanup EXIT

if [ ! -x "$binary" ]; then
  printf 'framesplayer binary is not executable: %s\n' "$binary" >&2
  exit 1
fi

set +e
"$binary" >"$output_file" 2>&1
status=$?
set -e

if [ "$status" -ne 2 ]; then
  printf 'expected framesplayer to exit 2 without arguments, got %s\n' "$status" >&2
  cat "$output_file" >&2
  exit 1
fi

if ! grep -q 'usage: framesplayer <share-url-or-token>' "$output_file"; then
  printf 'expected framesplayer usage output\n' >&2
  cat "$output_file" >&2
  exit 1
fi
