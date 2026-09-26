#!/usr/bin/env bash
# Generate per-platform npm runtime packages from built dist/release-assets.
#
# Each platform bundle (kandev-{platform}.tar.gz) is repackaged as an npm package
# containing the native bin/ directory. These are the @kdlbs/runtime-* packages
# that the main kandev npm package declares as optionalDependencies.
#
# Usage:
#   package-npm-runtime.sh <version> <release-assets-dir> <output-dir>
#
# Arguments:
#   version            SemVer string (e.g. 0.17.0)
#   release-assets-dir Directory containing kandev-*.tar.gz files
#   output-dir         Directory where per-platform npm packages are written
#
# Output: one scoped directory per package in npm-packages.sh.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/release/npm-packages.sh
source "$ROOT_DIR/scripts/release/npm-packages.sh"

VERSION="${1:?Usage: $0 <version> <release-assets-dir> <output-dir>}"
ASSETS_DIR="${2:?Usage: $0 <version> <release-assets-dir> <output-dir>}"
OUT_DIR="${3:?Usage: $0 <version> <release-assets-dir> <output-dir>}"

echo "Packaging npm runtime packages for version $VERSION..."
echo "  assets dir: $ASSETS_DIR"
echo "  output dir: $OUT_DIR"

for platform in "${RUNTIME_PLATFORMS[@]}"; do
  archive="$ASSETS_DIR/kandev-${platform}.tar.gz"
  if [[ ! -f "$archive" ]]; then
    echo "Error: missing archive $archive" >&2
    exit 1
  fi

  package_name="${RUNTIME_PACKAGE_BY_PLATFORM[$platform]}"
  # @kdlbs/runtime-linux-x64 → scope=@kdlbs, name=runtime-linux-x64
  scope_dir="${package_name%%/*}"    # @kdlbs
  pkg_dir="${package_name##*/}"      # runtime-linux-x64
  pkg_out="$OUT_DIR/${scope_dir}/${pkg_dir}"

  rm -rf "$pkg_out"
  mkdir -p "$pkg_out"

  # Extract the native runtime bundle from the archive.
  local_tmp="$pkg_out/.extract_tmp"
  mkdir -p "$local_tmp"
  tar -xzf "$archive" -C "$local_tmp"

  # Find the extracted bundle root (named "kandev" inside the archive)
  bundle_root="$local_tmp/kandev"
  if [[ ! -d "$bundle_root" ]]; then
    echo "Error: expected kandev/ directory in $archive" >&2
    exit 1
  fi

  cp -R "$bundle_root/bin" "$pkg_out/bin"
  rm -rf "$local_tmp"

  case "$platform" in
    linux-*) os_field='["linux"]' ;;
    macos-*) os_field='["darwin"]' ;;
    windows-*) os_field='["win32"]' ;;
    *) echo "Error: unsupported runtime platform $platform" >&2; exit 1 ;;
  esac
  case "$platform" in
    *-x64) cpu_field='["x64"]' ;;
    *-arm64) cpu_field='["arm64"]' ;;
    *) echo "Error: unsupported runtime architecture $platform" >&2; exit 1 ;;
  esac

  # Note: `repository` is required when publishing with `npm publish --provenance`.
  # npm's sigstore attestation embeds repo info from the OIDC token and refuses
  # to publish if the package.json's repository.url doesn't match.
  cat > "$pkg_out/package.json" <<EOF
{
  "name": "$package_name",
  "version": "$VERSION",
  "description": "Kandev runtime bundle for $platform",
  "license": "AGPL-3.0-only",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/kdlbs/kandev.git"
  },
  "homepage": "https://github.com/kdlbs/kandev",
  "os": $os_field,
  "cpu": $cpu_field,
  "files": [
    "bin"
  ]
}
EOF

  echo "  packaged $package_name@$VERSION"
done

echo "Done. Runtime packages written to $OUT_DIR"
