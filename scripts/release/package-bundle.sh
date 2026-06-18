#!/usr/bin/env bash
# Finalize the dist/kandev/ release layout from already-built pieces.
# Caller must have run, in this order:
#   - scripts/release/package-cli.sh  (produces dist/kandev/cli/)
#   - go build ./cmd/{kandev,agentctl} -o dist/kandev/bin/... with Vite
#     assets synced into the backend embed package before building kandev.
# After this: dist/kandev/{bin,cli} is ready to install or tar.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUNDLE="$ROOT_DIR/dist/kandev"

if [ ! -f "$BUNDLE/cli/bin/cli.js" ]; then
  echo "Missing $BUNDLE/cli/bin/cli.js; run scripts/release/package-cli.sh first" >&2
  exit 1
fi

echo "Bundle assembled at $BUNDLE"
