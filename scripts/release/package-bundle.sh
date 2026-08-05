#!/usr/bin/env bash
# Validate a release layout assembled from already-built pieces.
# Usage: package-bundle.sh [--bundle-dir DIR]
# Caller must have run, in this order:
#   - Vite assets synced into apps/backend/internal/webapp/embedded/generated
#   - go build ./cmd/{kandev,agentctl} plus remote agentctl helpers into the bundle's bin directory
# The default bundle is dist/kandev; package managers may select another path.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUNDLE="$ROOT_DIR/dist/kandev"
REMOTE_AGENTCTL_HELPERS=(
  agentctl-linux-amd64
  agentctl-linux-arm64
  agentctl-darwin-arm64
  agentctl-darwin-amd64
)

usage() {
  echo "usage: $0 [--bundle-dir DIR]" >&2
}

case "$#" in
  0) ;;
  2)
    if [ "$1" != "--bundle-dir" ] || [ -z "$2" ]; then
      usage
      exit 2
    fi
    BUNDLE="$2"
    ;;
  *)
    usage
    exit 2
    ;;
esac

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

for helper in "${REMOTE_AGENTCTL_HELPERS[@]}"; do
  if [ ! -f "$BUNDLE/bin/$helper" ]; then
    echo "Missing remote agentctl helper $helper in $BUNDLE/bin; run make -C apps/backend build-agentctl-remote first" >&2
    exit 1
  fi
  if [ ! -x "$BUNDLE/bin/$helper" ]; then
    echo "Runtime binary $helper is not executable in $BUNDLE/bin" >&2
    exit 1
  fi
done

echo "Bundle assembled at $BUNDLE"
