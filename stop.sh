#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PID_FILE="$ROOT_DIR/run/cligrep-server.pid"

if [ ! -f "$PID_FILE" ]; then
  printf 'cligrep-server is not running\n'
  exit 0
fi

PID=$(cat "$PID_FILE")

if [ -z "$PID" ]; then
  rm -f "$PID_FILE"
  printf 'removed empty pid file\n'
  exit 0
fi

if ! kill -0 "$PID" 2>/dev/null; then
  rm -f "$PID_FILE"
  printf 'stale pid file removed\n'
  exit 0
fi

kill "$PID"

i=0
while kill -0 "$PID" 2>/dev/null; do
  i=$((i + 1))
  if [ "$i" -ge 20 ]; then
    printf 'process %s did not exit after 10 seconds\n' "$PID" >&2
    exit 1
  fi
  sleep 0.5
done

rm -f "$PID_FILE"
printf 'cligrep-server stopped\n'
