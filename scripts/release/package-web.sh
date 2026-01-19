#!/usr/bin/env bash
# Used by .github/workflows/release.yml to package the Next.js standalone output.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WEB_DIR="$ROOT_DIR/apps/web"
OUT_DIR="$ROOT_DIR/dist/web"

echo "Packaging Next.js standalone output for release..."

if [ ! -d "$WEB_DIR/.next/standalone" ]; then
  echo "Missing standalone output at $WEB_DIR/.next/standalone"
  echo "Run: pnpm -C apps --filter @kandev/web build"
  exit 1
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

cp -R "$WEB_DIR/.next/standalone/." "$OUT_DIR/"
mkdir -p "$OUT_DIR/.next"
cp -R "$WEB_DIR/.next/static" "$OUT_DIR/.next/"

if [ -d "$WEB_DIR/public" ]; then
  cp -R "$WEB_DIR/public" "$OUT_DIR/public"
fi

echo "Web bundle packaged at $OUT_DIR"
