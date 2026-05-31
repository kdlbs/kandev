#!/usr/bin/env bash
set -euo pipefail

TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-real-npm}"
ENV_PATH="$TEST_HOME/real-npm-env.sh"
if [ -f "$ENV_PATH" ]; then
  # shellcheck disable=SC1090
  source "$ENV_PATH"
fi

ROOT="$(git rev-parse --show-toplevel)"
PORT="${KANDEV_TEST_PORT:-38429}"
NPM_PREFIX="${KANDEV_TEST_NPM_PREFIX:-$TEST_HOME/npm-global}"
LATEST_VERSION="${KANDEV_TEST_TARGET_VERSION:-$(npm view kandev version)}"
TIMEOUT_SECONDS="${KANDEV_TEST_APPLY_TIMEOUT_SECONDS:-180}"

export PATH="$NPM_PREFIX/bin:$PATH"
export npm_config_prefix="$NPM_PREFIX"
export NPM_CONFIG_PREFIX="$NPM_PREFIX"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "[self-update-real-npm] $1 is required" >&2
    exit 1
  fi
}

json_field() {
  node -e 'const fs=require("fs"); const key=process.argv[1]; const data=JSON.parse(fs.readFileSync(0,"utf8")); const value=data[key]; if (value !== undefined && value !== null) console.log(value);' "$1"
}

require_command curl
require_command node
require_command npm
require_command python3

"$ROOT/scripts/tmp-self-update/real-npm-seed-latest.sh"

TARGET="v${LATEST_VERSION#v}"
echo "[self-update-real-npm] target: $TARGET"

UPDATE_JSON="$(curl -fsS "http://localhost:$PORT/api/v1/system/updates")"
echo "$UPDATE_JSON"

APPLY_SUPPORTED="$(printf '%s' "$UPDATE_JSON" | json_field apply_supported)"
UPDATE_AVAILABLE="$(printf '%s' "$UPDATE_JSON" | json_field update_available)"
if [ "$APPLY_SUPPORTED" != "true" ] || [ "$UPDATE_AVAILABLE" != "true" ]; then
  echo "[self-update-real-npm] update is not applyable" >&2
  "$ROOT/scripts/tmp-self-update/real-npm-status.sh" >&2 || true
  exit 1
fi

APPLY_JSON="$(curl -fsS -X POST "http://localhost:$PORT/api/v1/system/updates/apply" -H "Content-Type: application/json" --data '{"confirm":"UPDATE"}')"
JOB_ID="$(printf '%s' "$APPLY_JSON" | json_field job_id)"
echo "[self-update-real-npm] apply job: $JOB_ID"

for _ in $(seq 1 "$TIMEOUT_SECONDS"); do
  INFO_JSON="$(curl -fsS "http://localhost:$PORT/api/v1/system/info" 2>/dev/null || true)"
  if [ -n "$INFO_JSON" ]; then
    VERSION="$(printf '%s' "$INFO_JSON" | json_field version)"
    if [ "$VERSION" = "$TARGET" ]; then
      echo "[self-update-real-npm] updated and reachable at http://localhost:$PORT"
      echo "$INFO_JSON"
      "$ROOT/scripts/tmp-self-update/real-npm-status.sh"
      exit 0
    fi
  fi
  sleep 1
done

echo "[self-update-real-npm] service did not report $TARGET within ${TIMEOUT_SECONDS}s" >&2
"$ROOT/scripts/tmp-self-update/real-npm-status.sh" >&2 || true
exit 1
