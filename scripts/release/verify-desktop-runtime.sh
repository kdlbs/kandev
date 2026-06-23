#!/usr/bin/env bash
# Validate the runtime resource layout consumed by the Tauri desktop app.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: verify-desktop-runtime.sh [runtime-dir]

Validate a desktop runtime directory with this layout:

  kandev/
    bin/
      kandev[.exe]
      agentctl[.exe]
      agentctl-linux-amd64

If runtime-dir is omitted, apps/desktop/src-tauri/resources/kandev is checked.
EOF
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_DIR="${ROOT_DIR}/apps/desktop/src-tauri/resources/kandev"

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  "")
    ;;
  *)
    RUNTIME_DIR="$1"
    ;;
esac

BIN_DIR="$RUNTIME_DIR/bin"

require_one() {
  local label="$1"
  shift
  local candidate
  for candidate in "$@"; do
    if [ -f "$candidate" ]; then
      return 0
    fi
  done
  printf 'Missing %s in %s\n' "$label" "$BIN_DIR" >&2
  exit 1
}

require_file() {
  local label="$1"
  local path="$2"
  if [ ! -f "$path" ]; then
    printf 'Missing %s at %s\n' "$label" "$path" >&2
    exit 1
  fi
}

if [ ! -d "$BIN_DIR" ]; then
  printf 'Missing desktop runtime bin directory at %s\n' "$BIN_DIR" >&2
  exit 1
fi

require_one "Kandev launcher binary" "$BIN_DIR/kandev" "$BIN_DIR/kandev.exe"
require_one "agentctl binary" "$BIN_DIR/agentctl" "$BIN_DIR/agentctl.exe"
require_file "agentctl linux/amd64 helper" "$BIN_DIR/agentctl-linux-amd64"

printf 'Desktop runtime verified at %s\n' "$RUNTIME_DIR"
