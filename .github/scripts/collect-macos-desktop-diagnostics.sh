#!/usr/bin/env bash
set -euo pipefail

label="${1:-failure}"
platform="${DESKTOP_DIAGNOSTICS_PLATFORM:?DESKTOP_DIAGNOSTICS_PLATFORM is required}"
rust_target="${DESKTOP_DIAGNOSTICS_RUST_TARGET:?DESKTOP_DIAGNOSTICS_RUST_TARGET is required}"
diag_dir="$GITHUB_WORKSPACE/dist/desktop-diagnostics"
bundle_root="$GITHUB_WORKSPACE/apps/desktop/src-tauri/target/${rust_target}/release/bundle"

mkdir -p "$diag_dir"
if [ ! -d "$bundle_root" ]; then
  bundle_root="$GITHUB_WORKSPACE/apps/desktop/src-tauri/target/release/bundle"
fi

{
  echo "label=$label"
  echo "platform=$platform"
  echo "rust_target=$rust_target"
  echo "date=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo
  echo "## system"
  uname -a || true
  sw_vers || true
  echo
  echo "## disk"
  df -h || true
  echo
  echo "## hdiutil info"
  hdiutil info || true
  echo
  echo "## mounts"
  mount || true
  echo
  echo "## related processes"
  ps aux | grep -E 'hdiutil|diskimages|bundle_dmg|pnpm|cargo' | grep -v grep || true
  echo
  echo "## bundle files"
  if [ -d "$bundle_root" ]; then
    find "$bundle_root" -maxdepth 5 -print || true
  else
    echo "Missing bundle root: $bundle_root"
  fi
} > "$diag_dir/macos-${platform}-${label}.txt"

if [ -f "$bundle_root/dmg/bundle_dmg.sh" ]; then
  cp "$bundle_root/dmg/bundle_dmg.sh" "$diag_dir/bundle_dmg-${platform}-${label}.sh"
fi
