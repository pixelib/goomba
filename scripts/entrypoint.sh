#!/usr/bin/env bash
set -euo pipefail

source_dir="${GOOMBA_SOURCE_DIR:-.}"
platforms="${GOOMBA_PLATFORMS:-linux,macos,windows}"
arch="${GOOMBA_ARCH:-x64,arm64}"
cgo_enabled="${GOOMBA_CGO_ENABLED:-false}"
go_args="${GOOMBA_GO_ARGS:-}"

if [ -n "$source_dir" ]; then
  cd "$source_dir"
fi

args=(build --no-tui --platforms "$platforms" --arch "$arch")
if [ "$cgo_enabled" = "true" ]; then
  args+=(--cgo-enabled)
fi
if [ -n "$go_args" ]; then
  args+=(--go-args "$go_args")
fi

goomba "${args[@]}"
