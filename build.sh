#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
OUTPUT_DIR="$ROOT_DIR/build"
OUTPUT_BIN="$OUTPUT_DIR/cligrep-server"

mkdir -p "$OUTPUT_DIR"

cd "$ROOT_DIR"
go build -o "$OUTPUT_BIN" ./cmd/server

printf 'built %s\n' "$OUTPUT_BIN"
