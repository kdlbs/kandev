#!/usr/bin/env bash
# Finalize the dist/kandev/ release layout from already-built pieces.
# Caller must have run, in this order:
#   - scripts/release/package-web.sh  (produces dist/web/)
#   - scripts/release/package-cli.sh  (produces dist/kandev/cli/)
#   - go build ./cmd/{kandev,agentctl} -o dist/kandev/bin/...
# After this: dist/kandev/{bin,web,cli} is ready to install or tar.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WEB_SRC="$ROOT_DIR/dist/web"
BUNDLE="$ROOT_DIR/dist/kandev"

if [ ! -d "$WEB_SRC" ]; then
  echo "Missing $WEB_SRC; run scripts/release/package-web.sh first" >&2
  exit 1
fi

mkdir -p "$BUNDLE/web"
cp -R "$WEB_SRC/." "$BUNDLE/web/"

if [ ! -f "$BUNDLE/cli/bin/cli.js" ]; then
  echo "Missing $BUNDLE/cli/bin/cli.js; run scripts/release/package-cli.sh first" >&2
  exit 1
fi

echo "Bundle assembled at $BUNDLE"
