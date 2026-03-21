#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ENV_FILE=${ENV_FILE:-"$ROOT_DIR/.env"}
BIN_PATH=${BIN_PATH:-"$ROOT_DIR/build/cligrep-server"}
RUN_DIR="$ROOT_DIR/run"
LOG_DIR="$ROOT_DIR/logs"
PID_FILE="$RUN_DIR/cligrep-server.pid"
LOG_FILE="$LOG_DIR/cligrep-server.log"

mkdir -p "$RUN_DIR" "$LOG_DIR"

if [ -f "$PID_FILE" ]; then
  PID=$(cat "$PID_FILE")
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    printf 'cligrep-server already running with pid %s\n' "$PID"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

if [ ! -x "$BIN_PATH" ]; then
  printf 'missing binary: %s\n' "$BIN_PATH" >&2
  printf 'run ./build.sh first\n' >&2
  exit 1
fi

if [ -f "$ENV_FILE" ]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

DB_PATH=${CLIGREP_DB_PATH:-"$ROOT_DIR/data/cligrep.db"}
case "$DB_PATH" in
  /*) DB_DIR=$(dirname "$DB_PATH") ;;
  *) DB_DIR=$(dirname "$ROOT_DIR/$DB_PATH") ;;
esac
mkdir -p "$DB_DIR"

cd "$ROOT_DIR"
nohup "$BIN_PATH" >>"$LOG_FILE" 2>&1 &
PID=$!
printf '%s\n' "$PID" > "$PID_FILE"

sleep 1

if kill -0 "$PID" 2>/dev/null; then
  PROC_STATE=$(ps -p "$PID" -o stat= 2>/dev/null | tr -d ' ')
  if [ -n "$PROC_STATE" ] && [ "${PROC_STATE#Z}" = "$PROC_STATE" ]; then
    printf 'cligrep-server started with pid %s\n' "$PID"
    printf 'log file: %s\n' "$LOG_FILE"
    exit 0
  fi
fi

rm -f "$PID_FILE"
printf 'failed to start cligrep-server, check %s\n' "$LOG_FILE" >&2
exit 1
