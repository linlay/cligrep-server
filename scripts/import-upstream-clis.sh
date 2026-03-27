#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-"$ROOT_DIR/.env"}
IMPORT_TARGET_HOST=${IMPORT_TARGET_HOST:-singapore02}
IMPORT_TARGET_ROOT=${IMPORT_TARGET_ROOT:-/docker/cli-releases}
IMPORT_STAGE_ROOT=${IMPORT_STAGE_ROOT:-}
IMPORT_KEEP_STAGE=${IMPORT_KEEP_STAGE:-0}
IMPORT_REUSE_STAGE=${IMPORT_REUSE_STAGE:-0}
IMPORT_DRY_RUN=0

usage() {
  cat <<'EOF'
Usage: ./scripts/import-upstream-clis.sh [--dry-run] [--keep-stage] [--reuse-stage] [--stage-root <dir>] [--target-host <host>] [--target-root <dir>]

Options:
  --dry-run            Stage releases locally and write a manifest, but skip upload, MySQL seed, and release-sync.
  --keep-stage         Do not delete the staging directory after the script exits.
  --reuse-stage        Reuse an existing stage-root and manifest instead of downloading upstream assets again.
  --stage-root <dir>   Reuse an explicit staging directory instead of mktemp.
  --target-host <host> Remote SSH host for release mirroring. Use "local" or an empty value for local sync.
  --target-root <dir>  Destination release root. Defaults to /docker/cli-releases.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run)
      IMPORT_DRY_RUN=1
      shift
      ;;
    --keep-stage)
      IMPORT_KEEP_STAGE=1
      shift
      ;;
    --reuse-stage)
      IMPORT_REUSE_STAGE=1
      shift
      ;;
    --stage-root)
      IMPORT_STAGE_ROOT=${2:?missing value for --stage-root}
      shift 2
      ;;
    --target-host)
      IMPORT_TARGET_HOST=${2:?missing value for --target-host}
      shift 2
      ;;
    --target-root)
      IMPORT_TARGET_ROOT=${2:?missing value for --target-root}
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ -f "$ENV_FILE" ]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

for cmd in python3 go rsync curl; do
  require_cmd "$cmd"
done

if [ "$IMPORT_DRY_RUN" -eq 0 ]; then
  if [ -n "$IMPORT_TARGET_HOST" ] && [ "$IMPORT_TARGET_HOST" != "local" ]; then
    require_cmd ssh
  fi
fi

if [ -z "$IMPORT_STAGE_ROOT" ]; then
  IMPORT_STAGE_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/cligrep-upstream-import.XXXXXX")
fi
MANIFEST_PATH="$IMPORT_STAGE_ROOT/manifest.json"

cleanup() {
  if [ "$IMPORT_KEEP_STAGE" -eq 0 ]; then
    rm -rf "$IMPORT_STAGE_ROOT"
  fi
}
trap cleanup EXIT

printf 'staging upstream releases under %s\n' "$IMPORT_STAGE_ROOT"
if [ "$IMPORT_REUSE_STAGE" -eq 1 ]; then
  if [ ! -f "$MANIFEST_PATH" ]; then
    printf 'missing manifest for --reuse-stage: %s\n' "$MANIFEST_PATH" >&2
    exit 1
  fi
  printf 'reusing existing stage and manifest at %s\n' "$IMPORT_STAGE_ROOT"
else
  python3 "$ROOT_DIR/scripts/import_upstream_clis.py" \
    --stage-root "$IMPORT_STAGE_ROOT" \
    --manifest "$MANIFEST_PATH"
fi

SUCCESS_SLUGS=()
while IFS= read -r slug; do
  SUCCESS_SLUGS+=("$slug")
done < <(python3 - "$MANIFEST_PATH" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    manifest = json.load(fh)
for result in manifest.get("results", []):
    print(result["slug"])
PY
)

if [ "${#SUCCESS_SLUGS[@]}" -eq 0 ]; then
  printf 'no upstream CLI releases were staged successfully\n' >&2
  exit 1
fi

python3 - "$MANIFEST_PATH" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    manifest = json.load(fh)

for result in manifest.get("results", []):
    versions = ", ".join(result.get("versions", []))
    print(f"ready {result['slug']}: {versions}")

for failure in manifest.get("failures", []):
    print(f"warning {failure['slug']}: {failure['error']}", file=sys.stderr)
PY

if [ "$IMPORT_DRY_RUN" -eq 1 ]; then
  printf 'dry-run complete; staged files kept at %s\n' "$IMPORT_STAGE_ROOT"
  printf 'manifest: %s\n' "$MANIFEST_PATH"
  exit 0
fi

sync_slug_remote() {
  local slug=$1
  ssh "$IMPORT_TARGET_HOST" "mkdir -p '$IMPORT_TARGET_ROOT/$slug'"
  rsync -az --delete "$IMPORT_STAGE_ROOT/$slug/" "$IMPORT_TARGET_HOST:$IMPORT_TARGET_ROOT/$slug/"
}

sync_slug_local() {
  local slug=$1
  mkdir -p "$IMPORT_TARGET_ROOT/$slug"
  rsync -az --delete "$IMPORT_STAGE_ROOT/$slug/" "$IMPORT_TARGET_ROOT/$slug/"
}

if [ -n "$IMPORT_TARGET_HOST" ] && [ "$IMPORT_TARGET_HOST" != "local" ]; then
  ssh "$IMPORT_TARGET_HOST" "mkdir -p '$IMPORT_TARGET_ROOT'"
  for slug in "${SUCCESS_SLUGS[@]}"; do
    printf 'syncing %s to %s:%s\n' "$slug" "$IMPORT_TARGET_HOST" "$IMPORT_TARGET_ROOT"
    sync_slug_remote "$slug"
  done
else
  mkdir -p "$IMPORT_TARGET_ROOT"
  for slug in "${SUCCESS_SLUGS[@]}"; do
    printf 'syncing %s to %s\n' "$slug" "$IMPORT_TARGET_ROOT"
    sync_slug_local "$slug"
  done
fi

printf 'refreshing scripts/mysql/seed-clis.sql via Go catalog seeder\n'
(
  cd "$ROOT_DIR"
  go run ./cmd/seed-catalog
)

printf 'running release-sync from staged releases\n'
(
  cd "$ROOT_DIR"
  CLIGREP_RELEASES_ROOT="$IMPORT_STAGE_ROOT" go run ./cmd/release-sync "${SUCCESS_SLUGS[@]}"
)

printf 'import complete; mirrored slugs: %s\n' "${SUCCESS_SLUGS[*]}"
