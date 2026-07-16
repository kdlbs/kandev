#!/usr/bin/env bash
# Validate the signing prerequisites for producing updater artifacts on one platform.
set -euo pipefail

platform="${1:?Usage: updater-signing-ready.sh <platform>}"

if [ -z "${TAURI_SIGNING_PRIVATE_KEY:-}" ]; then
  echo "Updater artifacts require TAURI_SIGNING_PRIVATE_KEY." >&2
  exit 1
fi

case "$platform" in
  macos-arm64 | macos-x64 | linux-x64 | linux-arm64 | windows-x64) ;;
  *)
    echo "Unsupported updater platform: $platform" >&2
    echo "Expected one of: macos-arm64, macos-x64, linux-x64, linux-arm64, windows-x64" >&2
    exit 1
    ;;
esac

echo "Tauri updater signing prerequisite complete for $platform."
