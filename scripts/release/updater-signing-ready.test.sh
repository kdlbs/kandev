#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/release/updater-signing-ready.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

valid_platforms=(macos-arm64 macos-x64 linux-x64 linux-arm64 windows-x64)
for platform in "${valid_platforms[@]}"; do
  TAURI_SIGNING_PRIVATE_KEY=test-key \
    MACOS_SIGNING_ENABLED=false \
    WINDOWS_SIGNING_ENABLED=false \
    bash "$SCRIPT" "$platform" >"$TMP_DIR/out" 2>"$TMP_DIR/err" || \
    fail "updater signing readiness rejected release platform $platform"
  grep -q "complete for $platform" "$TMP_DIR/out" || \
    fail "updater signing readiness did not confirm $platform"
done

unsupported_platforms=(
  macos-aarch64
  macos-x86_64
  linux-x86_64
  linux-aarch64
  windows-x86_64
)
misspelled_platforms=(linux-x65)
for platform in "${unsupported_platforms[@]}" "${misspelled_platforms[@]}"; do
  if TAURI_SIGNING_PRIVATE_KEY=test-key bash "$SCRIPT" "$platform" \
    >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
    fail "updater signing readiness accepted unsupported platform $platform"
  fi
  grep -q "Unsupported updater platform: $platform" "$TMP_DIR/err" || \
    fail "unsupported platform error did not identify $platform"
  grep -q "macos-arm64, macos-x64, linux-x64, linux-arm64, windows-x64" \
    "$TMP_DIR/err" || fail "unsupported platform error did not list accepted values"
done

if env -u TAURI_SIGNING_PRIVATE_KEY bash "$SCRIPT" linux-x64 \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "updater signing readiness accepted a missing updater key"
fi
grep -q "require TAURI_SIGNING_PRIVATE_KEY" "$TMP_DIR/err" || \
  fail "missing updater key error was not actionable"

echo "PASS: updater signing readiness accepts the release matrix and only requires the updater key"
