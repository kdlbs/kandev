#!/usr/bin/env bash
# Used by .github/workflows/release.yml to package the Vite SPA output.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WEB_DIR="$ROOT_DIR/apps/web"
OUT_DIR="$ROOT_DIR/dist/web"

echo "Packaging Vite web output for release..."

if [ ! -f "$WEB_DIR/dist/index.html" ]; then
  echo "Missing Vite output at $WEB_DIR/dist/index.html"
  echo "Run: pnpm -C apps --filter @kandev/web build:vite"
  exit 1
fi
"$ROOT_DIR/scripts/release/validate-web-bundle.sh" "$WEB_DIR/dist"

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

cp -R "$WEB_DIR/dist/." "$OUT_DIR/"
"$ROOT_DIR/scripts/release/validate-web-bundle.sh" "$OUT_DIR"

echo "Web bundle packaged at $OUT_DIR"
