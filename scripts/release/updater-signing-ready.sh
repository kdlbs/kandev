#!/usr/bin/env bash
# Validate the signing prerequisites for producing updater artifacts on one platform.
set -euo pipefail

platform="${1:?Usage: updater-signing-ready.sh <platform>}"

if [ -z "${TAURI_SIGNING_PRIVATE_KEY:-}" ]; then
  echo "Updater artifacts require TAURI_SIGNING_PRIVATE_KEY." >&2
  exit 1
fi

case "$platform" in
  macos-aarch64 | macos-x86_64 | windows-x86_64 | linux-x86_64 | linux-aarch64) ;;
  *)
    echo "Unsupported updater platform: $platform" >&2
    exit 1
    ;;
esac

echo "Tauri updater signing prerequisite complete for $platform."
