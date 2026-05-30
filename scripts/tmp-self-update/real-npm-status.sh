#!/usr/bin/env bash
set -euo pipefail

TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-real-npm}"
ENV_PATH="$TEST_HOME/real-npm-env.sh"
if [ -f "$ENV_PATH" ]; then
  # shellcheck disable=SC1090
  source "$ENV_PATH"
fi

PORT="${KANDEV_TEST_PORT:-38429}"
NPM_PREFIX="${KANDEV_TEST_NPM_PREFIX:-$TEST_HOME/npm-global}"
KANDEV_BIN="${KANDEV_TEST_KANDEV_BIN:-$NPM_PREFIX/bin/kandev}"
export PATH="$NPM_PREFIX/bin:$PATH"
export npm_config_prefix="$NPM_PREFIX"
export NPM_CONFIG_PREFIX="$NPM_PREFIX"

echo "[self-update-real-npm] backend info:"
INFO=""
for _ in $(seq 1 60); do
  if INFO="$(curl -fsS "http://localhost:$PORT/api/v1/system/info" 2>/dev/null)"; then
    break
  fi
  sleep 1
done
if [ -z "$INFO" ]; then
  echo "[self-update-real-npm] backend did not answer on port $PORT" >&2
  exit 1
fi
echo "$INFO"
echo
echo "[self-update-real-npm] isolated npm kandev:"
npm list -g --prefix "$NPM_PREFIX" kandev --depth=0 || true
echo "[self-update-real-npm] service status:"
if [ -x "$KANDEV_BIN" ]; then
  "$KANDEV_BIN" service status || true
else
  echo "[self-update-real-npm] missing $KANDEV_BIN" >&2
fi
