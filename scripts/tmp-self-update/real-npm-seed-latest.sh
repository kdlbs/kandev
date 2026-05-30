#!/usr/bin/env bash
set -euo pipefail

TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-real-npm}"
ENV_PATH="$TEST_HOME/real-npm-env.sh"
if [ -f "$ENV_PATH" ]; then
  # shellcheck disable=SC1090
  source "$ENV_PATH"
fi

PORT="${KANDEV_TEST_PORT:-38429}"
LATEST_VERSION="${KANDEV_TEST_TARGET_VERSION:-$(npm view kandev version)}"
DB_PATH="$TEST_HOME/data/kandev.db"

if [ ! -f "$DB_PATH" ]; then
  echo "[self-update-real-npm] missing DB: $DB_PATH" >&2
  echo "[self-update-real-npm] run real-npm-setup.sh first and wait for service start" >&2
  exit 1
fi

python3 - "$DB_PATH" "$LATEST_VERSION" <<'PY'
import sqlite3
import sys
import time

db_path, latest = sys.argv[1:]
rows = {
    "latest_version": f"v{latest.lstrip('v')}",
    "latest_version_url": f"https://github.com/kdlbs/kandev/releases/tag/v{latest.lstrip('v')}",
    "latest_version_checked_at": str(int(time.time())),
}
with sqlite3.connect(db_path) as db:
    db.execute("CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')")
    for key, value in rows.items():
        db.execute(
            """
            INSERT INTO kandev_meta(key, value) VALUES(?, ?)
            ON CONFLICT(key) DO UPDATE SET value=excluded.value
            """,
            (key, value),
        )
PY

echo "[self-update-real-npm] seeded latest_version=v${LATEST_VERSION#v}"
echo "[self-update-real-npm] open http://localhost:$PORT/settings/system/updates"
echo "[self-update-real-npm] click Apply update. This will run npm install -g kandev@${LATEST_VERSION#v}."
