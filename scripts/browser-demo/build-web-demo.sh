#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_PATH="${1:-apps/web/dist-browser-demo}"
if [[ "$OUTPUT_PATH" = /* ]]; then
  OUTPUT_DIR="$OUTPUT_PATH"
else
  OUTPUT_DIR="$ROOT_DIR/$OUTPUT_PATH"
fi

cd "$ROOT_DIR/apps/web"
VITE_KANDEV_BROWSER_DEMO=true \
VITE_KANDEV_BASE_PATH=/browser-demo/app/ \
  pnpm exec vite build --outDir "$OUTPUT_DIR"

printf 'Browser demo written to %s\n' "$OUTPUT_DIR"
