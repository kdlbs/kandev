#!/usr/bin/env bash
set -euo pipefail

TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-real-npm}"
ENV_PATH="$TEST_HOME/real-npm-env.sh"
if [ -f "$ENV_PATH" ]; then
  # shellcheck disable=SC1090
  source "$ENV_PATH"
fi

NPM_PREFIX="${KANDEV_TEST_NPM_PREFIX:-$TEST_HOME/npm-global}"
KANDEV_BIN="${KANDEV_TEST_KANDEV_BIN:-$NPM_PREFIX/bin/kandev}"
export PATH="$NPM_PREFIX/bin:$PATH"
export npm_config_prefix="$NPM_PREFIX"
export NPM_CONFIG_PREFIX="$NPM_PREFIX"

if [ -x "$KANDEV_BIN" ]; then
  "$KANDEV_BIN" service uninstall || true
elif command -v kandev >/dev/null 2>&1; then
  kandev service uninstall || true
fi

rm -rf "$TEST_HOME"
echo "[self-update-real-npm] removed $TEST_HOME"
