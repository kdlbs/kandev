#!/usr/bin/env bash
# Validate that a directory contains a complete Vite SPA bundle.
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <web-bundle-dir>" >&2
  exit 2
fi

WEB_DIR="$1"

if [ ! -f "$WEB_DIR/index.html" ]; then
  echo "Missing Vite SPA index: $WEB_DIR/index.html" >&2
  exit 1
fi

if [ ! -d "$WEB_DIR/assets" ]; then
  echo "Missing Vite SPA assets directory: $WEB_DIR/assets" >&2
  exit 1
fi

if ! find "$WEB_DIR/assets" -type f -name '*.js' -print -quit | grep -q .; then
  echo "Missing Vite SPA JavaScript assets under: $WEB_DIR/assets" >&2
  exit 1
fi
