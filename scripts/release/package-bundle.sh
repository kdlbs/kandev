#!/usr/bin/env bash
# Validate a release layout assembled from already-built pieces.
# Usage: package-bundle.sh [--bundle-dir DIR] [--variant standard|full]
# Caller must have run, in this order:
#   - Vite assets synced into apps/backend/internal/webapp/embedded/generated
#   - go build ./cmd/{kandev,agentctl} plus remote agentctl helpers into the bundle's bin directory
# The default bundle is dist/kandev; package managers may select another path.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUNDLE="$ROOT_DIR/dist/kandev"
DARWIN_HELPER_VALIDATOR="$ROOT_DIR/scripts/release/validate-darwin-arm64-helper.mjs"
REMOTE_HELPER_ASSET_VALIDATOR="$ROOT_DIR/scripts/release/remote-helper-assets.mjs"
VARIANT="full"
REMOTE_AGENTCTL_HELPERS=(
  agentctl-linux-amd64
  agentctl-linux-arm64
  agentctl-darwin-arm64
  agentctl-darwin-amd64
)

usage() {
  echo "usage: $0 [--bundle-dir DIR] [--variant standard|full]" >&2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --bundle-dir)
      if [ "$#" -lt 2 ] || [ -z "$2" ]; then
        usage
        exit 2
      fi
      BUNDLE="$2"
      shift 2
      ;;
    --variant)
      if [ "$#" -lt 2 ]; then
        usage
        exit 2
      fi
      VARIANT="$2"
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

if [ "$VARIANT" != "standard" ] && [ "$VARIANT" != "full" ]; then
  usage
  exit 2
fi

if [ "$BUNDLE" = "/" ]; then
  echo "bundle directory must not be /" >&2
  exit 2
fi

launcher="kandev"
if [ ! -f "$BUNDLE/bin/$launcher" ] && [ -f "$BUNDLE/bin/kandev.exe" ]; then
  launcher="kandev.exe"
fi
if [ ! -f "$BUNDLE/bin/$launcher" ]; then
  echo "Missing native launcher in $BUNDLE/bin; build cmd/kandev first" >&2
  exit 1
fi
if [ ! -x "$BUNDLE/bin/$launcher" ]; then
  echo "Runtime binary $launcher is not executable in $BUNDLE/bin" >&2
  exit 1
fi

agentctl="agentctl"
if [ ! -f "$BUNDLE/bin/$agentctl" ] && [ -f "$BUNDLE/bin/agentctl.exe" ]; then
  agentctl="agentctl.exe"
fi
if [ ! -f "$BUNDLE/bin/$agentctl" ]; then
  echo "Missing agentctl in $BUNDLE/bin; build cmd/agentctl first" >&2
  exit 1
fi
if [ ! -x "$BUNDLE/bin/$agentctl" ]; then
  echo "Runtime binary $agentctl is not executable in $BUNDLE/bin" >&2
  exit 1
fi

if [ "$VARIANT" = "full" ]; then
  for helper in "${REMOTE_AGENTCTL_HELPERS[@]}"; do
    if [ ! -f "$BUNDLE/bin/$helper" ]; then
      echo "Missing remote agentctl helper $helper in $BUNDLE/bin; run make -C apps/backend build-agentctl-remote first" >&2
      exit 1
    fi
    if [ ! -x "$BUNDLE/bin/$helper" ]; then
      echo "Runtime binary $helper is not executable in $BUNDLE/bin" >&2
      exit 1
    fi
    if [ "$helper" = "agentctl-darwin-arm64" ]; then
      if ! command -v node >/dev/null 2>&1; then
        echo "Node.js is required to validate $helper" >&2
        exit 1
      fi
      node "$DARWIN_HELPER_VALIDATOR" "$BUNDLE/bin/$helper"
    fi
  done
fi

if ! command -v node >/dev/null 2>&1; then
  echo "Node.js is required to validate runtime bundle metadata" >&2
  exit 1
fi
node "$REMOTE_HELPER_ASSET_VALIDATOR" verify-bundle --bundle-dir "$BUNDLE" --variant "$VARIANT"

while IFS= read -r -d '' entry; do
  artifact="${entry##*/}"
  expected=false
  expected_artifacts=("$launcher" "$agentctl")
  if [ "$VARIANT" = "full" ]; then
    expected_artifacts+=("${REMOTE_AGENTCTL_HELPERS[@]}")
  fi
  for required in "${expected_artifacts[@]}"; do
    if [ "$artifact" = "$required" ]; then
      expected=true
      break
    fi
  done
  if [ "$expected" != true ]; then
    echo "Unexpected runtime artifact $artifact in $BUNDLE/bin" >&2
    exit 1
  fi
done < <(find "$BUNDLE/bin" -mindepth 1 -maxdepth 1 -print0)

echo "Bundle assembled at $BUNDLE"
