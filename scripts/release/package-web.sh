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

# Copy with symlinks dereferenced so pnpm's .pnpm/ structure survives
# zip/unzip (adm-zip can't handle symlinks). The standalone output may
# contain broken symlinks from partial tracing — --safe-links skips those
# and rsync exits with code 23 (partial transfer), which is expected.
rsync -a --copy-links --safe-links "$WEB_DIR/.next/standalone/" "$OUT_DIR/" || {
  rc=$?
  if [ "$rc" -eq 23 ]; then
    echo "Warning: some broken symlinks were skipped (expected for standalone output)"
  else
    exit "$rc"
  fi
}
mkdir -p "$OUT_DIR/.next"
cp -R "$WEB_DIR/.next/static" "$OUT_DIR/.next/"

if [ -d "$WEB_DIR/public" ]; then
  cp -R "$WEB_DIR/public" "$OUT_DIR/public"
fi

echo "Web bundle packaged at $OUT_DIR"
