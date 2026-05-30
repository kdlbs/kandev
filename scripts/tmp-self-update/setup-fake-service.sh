#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-test}"
PORT="${KANDEV_TEST_PORT:-38429}"
MODE="${KANDEV_TEST_MODE:-user}"
KIND="${KANDEV_TEST_KIND:-npm}"
NODE_PATH="${KANDEV_TEST_NODE:-$(command -v node)}"
CLI_ENTRY="${KANDEV_TEST_CLI_ENTRY:-$ROOT/apps/cli/dist/cli.js}"

case "$(uname -s)" in
  Darwin) DEFAULT_MANAGER="launchd" ;;
  *) DEFAULT_MANAGER="systemd" ;;
esac
MANAGER="${KANDEV_TEST_MANAGER:-$DEFAULT_MANAGER}"

mkdir -p "$TEST_HOME/service" "$TEST_HOME/logs" "$TEST_HOME/data"

if [ ! -f "$CLI_ENTRY" ]; then
  echo "[self-update-test] missing CLI entry: $CLI_ENTRY" >&2
  echo "[self-update-test] run scripts/tmp-self-update/build-local.sh first" >&2
  exit 1
fi

SERVICE_PATH="$TEST_HOME/service/fake-$MANAGER.service"
METADATA_PATH="$TEST_HOME/service/install.json"
ENV_PATH="$TEST_HOME/service/test-env.sh"

cat >"$SERVICE_PATH" <<EOF
# managed by kandev
Environment=KANDEV_RUNNING_AS_SERVICE=true
Environment=KANDEV_SERVICE_MODE=$MODE
Environment=KANDEV_SERVICE_MANAGER=$MANAGER
Environment=KANDEV_INSTALL_KIND=$KIND
Environment=KANDEV_SERVICE_METADATA=$METADATA_PATH
EOF

python3 - "$METADATA_PATH" "$MANAGER" "$MODE" "$KIND" "$TEST_HOME" "$SERVICE_PATH" "$NODE_PATH" "$CLI_ENTRY" "$PORT" <<'PY'
import json
import sys
from datetime import datetime, timezone

metadata_path, manager, mode, kind, home_dir, service_path, node_path, cli_entry, port = sys.argv[1:]
metadata = {
    "version": 1,
    "manager": manager,
    "mode": mode,
    "kind": kind,
    "home_dir": home_dir,
    "log_dir": f"{home_dir}/logs",
    "service_path": service_path,
    "node_path": node_path,
    "cli_entry": cli_entry,
    "port": int(port),
    "installed_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
}
with open(metadata_path, "w", encoding="utf-8") as f:
    json.dump(metadata, f, indent=2)
    f.write("\n")
PY

cat >"$ENV_PATH" <<EOF
export KANDEV_HOME_DIR="$TEST_HOME"
export KANDEV_SERVER_PORT="$PORT"
export KANDEV_RUNNING_AS_SERVICE=true
export KANDEV_SERVICE_MODE="$MODE"
export KANDEV_SERVICE_MANAGER="$MANAGER"
export KANDEV_INSTALL_KIND="$KIND"
export KANDEV_SERVICE_METADATA="$METADATA_PATH"
export KANDEV_E2E_MOCK=true
EOF

echo "[self-update-test] TEST_HOME=$TEST_HOME"
echo "[self-update-test] wrote $METADATA_PATH"
echo "[self-update-test] wrote $ENV_PATH"
echo "[self-update-test] next: scripts/tmp-self-update/run-backend.sh"
