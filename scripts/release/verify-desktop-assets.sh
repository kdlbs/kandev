#!/usr/bin/env bash
# Verify desktop installer artifacts and checksums before publishing a release.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: verify-desktop-assets.sh <assets-dir> [platform...]

Checks that each required platform has at least one desktop artifact named:

  kandev-desktop-<platform>-*

and that every matching artifact has a sibling .sha256 file. If no platform is
given, all supported desktop platforms are required. Signed Tauri updater
artifacts (`*.app.tar.gz`, `*.AppImage.tar.gz`, and `*.nsis.zip`) must have a
matching `*.sig` file, and signatures without their updater bundle are rejected.
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

ASSETS_DIR="${1:?Usage: verify-desktop-assets.sh <assets-dir> [platform...]}"
shift || true

if [ "$#" -gt 0 ]; then
  REQUIRED_PLATFORMS=("$@")
else
  REQUIRED_PLATFORMS=(macos-arm64 macos-x64 linux-x64 linux-arm64 windows-x64)
fi

if [ ! -d "$ASSETS_DIR" ]; then
  echo "Missing desktop assets directory: $ASSETS_DIR" >&2
  exit 1
fi

verify_checksum() {
  local assets_dir="$1"
  local checksum_file="$2"

  if command -v shasum >/dev/null 2>&1; then
    (cd "$assets_dir" && shasum -a 256 -c "$checksum_file")
  elif command -v sha256sum >/dev/null 2>&1; then
    (cd "$assets_dir" && sha256sum -c "$checksum_file")
  else
    echo "Missing checksum tool: need shasum or sha256sum" >&2
    return 1
  fi
}

shopt -s nullglob

updater_suffix_for_platform() {
  case "$1" in
    macos-*) printf '%s\n' '*.app.tar.gz' ;;
    linux-*) printf '%s\n' '*.AppImage.tar.gz' ;;
    windows-*) printf '%s\n' '*.nsis.zip' ;;
    *) return 1 ;;
  esac
}

for platform in "${REQUIRED_PLATFORMS[@]}"; do
  artifacts=("$ASSETS_DIR"/kandev-desktop-"$platform"-*)
  found=0

  for artifact in "${artifacts[@]}"; do
    if [[ "$artifact" == *.sha256 ]]; then
      continue
    fi
    found=$((found + 1))
    checksum_file="$artifact.sha256"
    if [ ! -f "$checksum_file" ]; then
      echo "Missing desktop checksum: $checksum_file" >&2
      exit 1
    fi
    verify_checksum "$ASSETS_DIR" "$(basename "$checksum_file")" >/dev/null || {
      echo "Checksum verification failed for: $artifact" >&2
      exit 1
    }
  done

  if [ "$found" -eq 0 ]; then
    echo "Missing desktop artifact for platform: $platform" >&2
    exit 1
  fi

  updater_suffix="$(updater_suffix_for_platform "$platform")"
  updater_bundles=("$ASSETS_DIR"/kandev-desktop-"$platform"-$updater_suffix)
  updater_signatures=("$ASSETS_DIR"/kandev-desktop-"$platform"-$updater_suffix.sig)
  for updater_bundle in "${updater_bundles[@]}"; do
    if [ ! -f "$updater_bundle.sig" ]; then
      echo "Missing updater signature: $updater_bundle.sig" >&2
      exit 1
    fi
  done
  for updater_signature in "${updater_signatures[@]}"; do
    updater_bundle="${updater_signature%.sig}"
    if [ ! -f "$updater_bundle" ]; then
      echo "Missing updater bundle for signature: $updater_signature" >&2
      exit 1
    fi
  done
done

echo "Desktop release assets verified in $ASSETS_DIR"
