#!/usr/bin/env bash
# Copy the native release bundle into the resource layout used by Tauri.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: prepare-desktop-runtime.sh [--bundle-dir DIR] [--output-dir DIR] [--platform PLATFORM]

Prepare apps/desktop/src-tauri/resources/kandev from an existing release
runtime bundle. The input bundle must contain:

  bin/kandev[.exe]
  bin/agentctl[.exe]
  bin/agentctl-linux-amd64 (required unless PLATFORM is linux-x64)

Options:
  --bundle-dir DIR  Source runtime bundle. Defaults to dist/kandev.
  --output-dir DIR  Destination runtime resource directory.
                    Defaults to apps/desktop/src-tauri/resources/kandev.
  --platform NAME   Release platform, such as linux-x64 or macos-arm64.
                    Defaults to the current host platform.
  -h, --help        Show this help.
EOF
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUNDLE_DIR="${ROOT_DIR}/dist/kandev"
OUTPUT_DIR="${ROOT_DIR}/apps/desktop/src-tauri/resources/kandev"
VERIFY_SCRIPT="${ROOT_DIR}/scripts/release/verify-desktop-runtime.sh"

detect_platform() {
  local os arch
  case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="macos" ;;
    MINGW*|MSYS*|CYGWIN*) os="windows" ;;
    *) os="unknown" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch="x64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) arch="$(uname -m)" ;;
  esac
  printf '%s-%s\n' "$os" "$arch"
}

PLATFORM="$(detect_platform)"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --bundle-dir)
      BUNDLE_DIR="${2:?Missing value for --bundle-dir}"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="${2:?Missing value for --output-dir}"
      shift 2
      ;;
    --platform)
      PLATFORM="${2:?Missing value for --platform}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown argument: %s\n\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

refuse_dangerous_output_dir() {
  case "$OUTPUT_DIR" in
    ""|"/"|"/."|"/..")
      printf 'Refusing dangerous desktop runtime output directory: %s\n' "${OUTPUT_DIR:-<empty>}" >&2
      exit 1
      ;;
  esac
}

refuse_dangerous_output_dir
chmod +x "$BUNDLE_DIR/bin/kandev" "$BUNDLE_DIR/bin/agentctl" "$BUNDLE_DIR/bin/agentctl-linux-amd64" 2>/dev/null || true
chmod +x "$BUNDLE_DIR/bin/kandev.exe" "$BUNDLE_DIR/bin/agentctl.exe" 2>/dev/null || true
"$VERIFY_SCRIPT" --platform "$PLATFORM" "$BUNDLE_DIR" >/dev/null

rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR/bin"
printf '*\n!.gitignore\n' > "$OUTPUT_DIR/.gitignore"

requires_agentctl_linux_amd64() {
  [ "$PLATFORM" != "linux-x64" ]
}

copy_one() {
  local label="$1"
  shift
  local source
  for source in "$@"; do
    if [ -f "$source" ]; then
      cp "$source" "$OUTPUT_DIR/bin/$(basename "$source")"
      chmod +x "$OUTPUT_DIR/bin/$(basename "$source")" 2>/dev/null || true
      return 0
    fi
  done
  printf 'Missing %s in %s\n' "$label" "$BUNDLE_DIR/bin" >&2
  exit 1
}

copy_one "Kandev launcher binary" "$BUNDLE_DIR/bin/kandev" "$BUNDLE_DIR/bin/kandev.exe"
copy_one "agentctl binary" "$BUNDLE_DIR/bin/agentctl" "$BUNDLE_DIR/bin/agentctl.exe"
if requires_agentctl_linux_amd64; then
  cp "$BUNDLE_DIR/bin/agentctl-linux-amd64" "$OUTPUT_DIR/bin/agentctl-linux-amd64"
  chmod +x "$OUTPUT_DIR/bin/agentctl-linux-amd64" 2>/dev/null || true
fi

"$VERIFY_SCRIPT" --platform "$PLATFORM" "$OUTPUT_DIR" >/dev/null
printf 'Desktop runtime prepared at %s\n' "$OUTPUT_DIR"
