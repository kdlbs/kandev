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
# archiving (symlinks cause issues on Windows even inside tar). The
# standalone output may contain broken symlinks from partial tracing —
# --safe-links skips those and rsync exits with code 23 (partial transfer),
# which is expected.
rsync -a --copy-links --safe-links "$WEB_DIR/.next/standalone/" "$OUT_DIR/" || {
  rc=$?
  if [ "$rc" -eq 23 ]; then
    echo "Warning: some broken symlinks were skipped (expected for standalone output)"
  else
    exit "$rc"
  fi
}
# pnpm's .pnpm virtual store keeps hoisted packages in
# node_modules/.pnpm/node_modules/ which is unreachable via normal Node
# module resolution from web/node_modules/next/. Copy everything into
# web/node_modules/ so Next.js (and its deps) can find them at runtime.
PNPM_HOISTED="$OUT_DIR/node_modules/.pnpm/node_modules"
NEXT_MODULES="$OUT_DIR/web/node_modules"
if [ -d "$PNPM_HOISTED" ] && [ -d "$NEXT_MODULES" ]; then
  for pkg in "$PNPM_HOISTED"/*; do
    name="$(basename "$pkg")"
    if [ ! -e "$NEXT_MODULES/$name" ]; then
      cp -R "$pkg" "$NEXT_MODULES/$name"
    fi
  done
  echo "Hoisted pnpm packages into web/node_modules/"
fi

# Static assets and public/ must be alongside server.js (inside web/)
# so Next.js can serve them. The standalone trace puts build artifacts
# at web/.next/ but doesn't include static/ — copy it there.
cp -R "$WEB_DIR/.next/static" "$OUT_DIR/web/.next/"

if [ -d "$WEB_DIR/public" ]; then
  cp -R "$WEB_DIR/public" "$OUT_DIR/web/public"
fi

echo "Web bundle packaged at $OUT_DIR"
