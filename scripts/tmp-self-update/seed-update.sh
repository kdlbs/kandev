#!/usr/bin/env bash
set -euo pipefail

TEST_HOME="${TEST_HOME:-$HOME/.kandev-selfupdate-test}"
TARGET_TAG="${KANDEV_TEST_TARGET_TAG:-v99.0.0}"
TARGET_URL="${KANDEV_TEST_TARGET_URL:-https://example.com/$TARGET_TAG}"
DB_PATH="$TEST_HOME/data/kandev.db"

if [ ! -f "$DB_PATH" ]; then
  echo "[self-update-test] missing DB: $DB_PATH" >&2
  echo "[self-update-test] start the backend once with run-backend.sh first" >&2
  exit 1
fi

python3 - "$DB_PATH" "$TARGET_TAG" "$TARGET_URL" <<'PY'
import sqlite3
import sys
import time

db_path, target_tag, target_url = sys.argv[1:]
rows = {
    "latest_version": target_tag,
    "latest_version_url": target_url,
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

echo "[self-update-test] seeded $TARGET_TAG in $DB_PATH"
echo "[self-update-test] refresh /settings/system/updates"
