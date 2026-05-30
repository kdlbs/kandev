#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"

echo "[self-update-test] run-backend.sh is now a wrapper."
echo "[self-update-test] starting backend + frontend via run-app.sh"
exec "$ROOT/scripts/tmp-self-update/run-app.sh"
