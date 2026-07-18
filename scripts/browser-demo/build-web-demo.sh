#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_DIR="${1:-$ROOT_DIR/apps/web/dist-browser-demo}"

cd "$ROOT_DIR/apps/web"
VITE_KANDEV_BROWSER_DEMO=true \
VITE_KANDEV_BASE_PATH=/browser-demo/app/ \
  pnpm exec vite build --outDir "$OUTPUT_DIR"

printf 'Browser demo written to %s\n' "$OUTPUT_DIR"

