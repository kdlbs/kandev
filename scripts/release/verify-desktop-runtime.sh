#!/usr/bin/env bash
# Validate the runtime resource layout consumed by the Tauri desktop app.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: verify-desktop-runtime.sh [--platform PLATFORM] [runtime-dir]

Validate a desktop runtime directory with this layout:

  kandev/
    bin/
      kandev[.exe]
      agentctl[.exe]
      agentctl-linux-amd64
      agentctl-linux-arm64
      agentctl-darwin-arm64
      agentctl-darwin-amd64

If runtime-dir is omitted, apps/desktop/src-tauri/resources/kandev is checked.
EOF
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_DIR="${ROOT_DIR}/apps/desktop/src-tauri/resources/kandev"
RUNTIME_DIR_SET=false
PLATFORM=""
REMOTE_AGENTCTL_HELPERS=(
  "agentctl-linux-amd64:agentctl linux/amd64 helper"
  "agentctl-linux-arm64:agentctl linux/arm64 helper"
  "agentctl-darwin-arm64:agentctl darwin/arm64 helper"
  "agentctl-darwin-amd64:agentctl darwin/amd64 helper"
)

while [ "$#" -gt 0 ]; do
  case "$1" in
    --platform)
      PLATFORM="${2:?Missing value for --platform}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      printf 'Unknown argument: %s\n\n' "$1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [ "$RUNTIME_DIR_SET" = true ]; then
        printf 'Unexpected extra runtime directory: %s\n\n' "$1" >&2
        usage >&2
        exit 2
      fi
      RUNTIME_DIR="$1"
      RUNTIME_DIR_SET=true
      shift
      ;;
  esac
done

BIN_DIR="$RUNTIME_DIR/bin"

is_executable_file() {
  local path="$1"
  if [ -x "$path" ]; then
    return 0
  fi
  # MSYS/Git Bash can report Windows executables differently from Unix mode bits.
  if [ "${OS:-}" = "Windows_NT" ]; then
    case "$(basename "$path")" in
      *.exe|agentctl-linux-amd64|agentctl-linux-arm64|agentctl-darwin-arm64|agentctl-darwin-amd64) return 0 ;;
    esac
  fi
  return 1
}

require_one() {
  local label="$1"
  shift
  local candidate
  local found=""
  for candidate in "$@"; do
    if [ -f "$candidate" ]; then
      found="$candidate"
      if is_executable_file "$candidate"; then
        return 0
      fi
    fi
  done
  if [ -n "$found" ]; then
    printf '%s is not executable: %s\n' "$label" "$found" >&2
    exit 1
  fi
  printf 'Missing %s in %s\n' "$label" "$BIN_DIR" >&2
  exit 1
}

require_executable() {
  local label="$1"
  local path="$2"
  if [ ! -f "$path" ]; then
    printf 'Missing %s at %s\n' "$label" "$path" >&2
    exit 1
  fi
  if ! is_executable_file "$path"; then
    printf '%s is not executable: %s\n' "$label" "$path" >&2
    exit 1
  fi
}

if [ ! -d "$BIN_DIR" ]; then
  printf 'Missing desktop runtime bin directory at %s\n' "$BIN_DIR" >&2
  exit 1
fi

require_one "Kandev launcher binary" "$BIN_DIR/kandev" "$BIN_DIR/kandev.exe"
require_one "agentctl binary" "$BIN_DIR/agentctl" "$BIN_DIR/agentctl.exe"
for helper_spec in "${REMOTE_AGENTCTL_HELPERS[@]}"; do
  helper="${helper_spec%%:*}"
  label="${helper_spec#*:}"
  require_executable "$label" "$BIN_DIR/$helper"
done

if [ -n "$PLATFORM" ]; then
  printf 'Desktop runtime verified for %s at %s\n' "$PLATFORM" "$RUNTIME_DIR"
else
  printf 'Desktop runtime verified at %s\n' "$RUNTIME_DIR"
fi
