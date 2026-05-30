#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-test}"
ENV_PATH="$TEST_HOME/service/test-env.sh"

if [ ! -f "$ENV_PATH" ]; then
  echo "[self-update-test] missing $ENV_PATH" >&2
  echo "[self-update-test] run scripts/tmp-self-update/setup-fake-service.sh first" >&2
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_PATH"

if [ ! -f "$ROOT/apps/cli/dist/cli.js" ]; then
  echo "[self-update-test] missing CLI build at apps/cli/dist/cli.js" >&2
  echo "[self-update-test] run scripts/tmp-self-update/build-local.sh first" >&2
  exit 1
fi

echo "[self-update-test] TEST_HOME=$TEST_HOME"
echo "[self-update-test] DB=$KANDEV_DATABASE_PATH"
echo "[self-update-test] KANDEV_E2E_MOCK=$KANDEV_E2E_MOCK"
echo "[self-update-test] starting backend + frontend"
echo "[self-update-test] open http://localhost:${KANDEV_SERVER_PORT}/settings/system/updates"

exec node "$ROOT/apps/cli/dist/cli.js" start \
  --port "$KANDEV_SERVER_PORT" \
  --web-internal-port "$KANDEV_WEB_PORT"
