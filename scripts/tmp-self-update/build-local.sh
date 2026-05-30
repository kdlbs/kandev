#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
CURRENT_VERSION="${KANDEV_TEST_CURRENT_VERSION:-v0.53.0}"

if [ ! -d "$ROOT/apps/node_modules" ]; then
  echo "[self-update-test] installing pnpm workspace deps"
  (cd "$ROOT/apps" && pnpm install --frozen-lockfile)
fi

echo "[self-update-test] building CLI dist"
(cd "$ROOT/apps" && pnpm --filter kandev build)

echo "[self-update-test] building backend/web with VERSION=$CURRENT_VERSION"
(cd "$ROOT" && VERSION="$CURRENT_VERSION" make build)

echo "[self-update-test] built $ROOT/apps/backend/bin/kandev"
