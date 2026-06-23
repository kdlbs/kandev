#!/usr/bin/env bash
# Verify desktop installer artifacts and checksums before publishing a release.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: verify-desktop-assets.sh <assets-dir> [platform...]

Checks that each required platform has at least one desktop artifact named:

  kandev-desktop-<platform>-*

and that every matching artifact has a sibling .sha256 file. If no platform is
given, all supported desktop platforms are required.
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

shopt -s nullglob

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
    (
      cd "$ASSETS_DIR"
      shasum -a 256 -c "$(basename "$checksum_file")" >/dev/null
    ) || {
      echo "Checksum verification failed for: $artifact" >&2
      exit 1
    }
  done

  if [ "$found" -eq 0 ]; then
    echo "Missing desktop artifact for platform: $platform" >&2
    exit 1
  fi
done

echo "Desktop release assets verified in $ASSETS_DIR"
