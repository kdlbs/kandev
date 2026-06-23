#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
OUT_FILE="$TMP_DIR/out"
ERR_FILE="$TMP_DIR/err"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

pass() {
  echo "PASS: $*"
}

write_runtime() {
  local dir="$1"
  local helper="${2:-with-helper}"
  mkdir -p "$dir/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/kandev"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/agentctl"
  if [ "$helper" = "with-helper" ]; then
    printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/agentctl-linux-amd64"
  fi
}

chmod_runtime() {
  local dir="$1"
  chmod +x "$dir/bin/kandev" "$dir/bin/agentctl"
  if [ -f "$dir/bin/agentctl-linux-amd64" ]; then
    chmod +x "$dir/bin/agentctl-linux-amd64"
  fi
}

runtime_dir="$TMP_DIR/runtime"
write_runtime "$runtime_dir"

if "$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform macos-arm64 "$runtime_dir" >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "verify-desktop-runtime should reject non-executable binaries"
fi
grep -q "not executable" "$ERR_FILE" || fail "verify-desktop-runtime did not explain non-executable binaries"
pass "verify-desktop-runtime rejects non-executable binaries"

chmod_runtime "$runtime_dir"
"$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform macos-arm64 "$runtime_dir" >"$OUT_FILE"
grep -q "verified for macos-arm64" "$OUT_FILE" || fail "verify-desktop-runtime did not include platform output"
pass "verify-desktop-runtime accepts executable runtime"

missing_helper_runtime_dir="$TMP_DIR/missing-helper-runtime"
write_runtime "$missing_helper_runtime_dir" without-helper
chmod_runtime "$missing_helper_runtime_dir"
if "$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform linux-x64 "$missing_helper_runtime_dir" >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "verify-desktop-runtime should require helper for linux-x64"
fi
grep -q "Missing agentctl linux/amd64 helper" "$ERR_FILE" || fail "verify-desktop-runtime did not explain missing helper"
pass "verify-desktop-runtime requires helper for linux-x64 runtime"

linux_output_dir="$TMP_DIR/linux-output"
"$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" \
  --bundle-dir "$runtime_dir" \
  --platform linux-x64 \
  --output-dir "$linux_output_dir" >"$OUT_FILE"
grep -q "prepared for linux-x64" "$OUT_FILE" || fail "prepare-desktop-runtime did not include platform output"
if [ ! -x "$linux_output_dir/bin/agentctl-linux-amd64" ]; then
  fail "prepare-desktop-runtime should copy executable helper for linux-x64"
fi
pass "prepare-desktop-runtime copies helper for linux-x64"

macos_output_dir="$TMP_DIR/macos-output"
"$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" \
  --bundle-dir "$runtime_dir" \
  --platform macos-arm64 \
  --output-dir "$macos_output_dir" >/dev/null
if [ ! -x "$macos_output_dir/bin/agentctl-linux-amd64" ]; then
  fail "prepare-desktop-runtime should copy executable helper for macos-arm64"
fi
pass "prepare-desktop-runtime copies helper for non-linux-x64"

if "$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" --bundle-dir "$runtime_dir" --output-dir / >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "prepare-desktop-runtime should reject root output directory"
fi
grep -q "Refusing dangerous desktop runtime output directory" "$ERR_FILE" || fail "prepare-desktop-runtime did not explain dangerous output directory"
pass "prepare-desktop-runtime rejects dangerous output directory"

safe_cwd="$TMP_DIR/safe-cwd"
mkdir -p "$safe_cwd"
if (cd "$safe_cwd" && "$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" --bundle-dir "$runtime_dir" --output-dir . >"$OUT_FILE" 2>"$ERR_FILE"); then
  fail "prepare-desktop-runtime should reject current-directory output"
fi
grep -q "Refusing dangerous desktop runtime output directory" "$ERR_FILE" || fail "prepare-desktop-runtime did not explain current-directory output"
pass "prepare-desktop-runtime rejects current-directory output"

assets_dir="$TMP_DIR/assets"
mkdir -p "$assets_dir"
artifact="$assets_dir/kandev-desktop-linux-x64-test.deb"
printf 'desktop artifact\n' > "$artifact"
(cd "$assets_dir" && shasum -a 256 "$(basename "$artifact")" > "$(basename "$artifact").sha256")
"$ROOT_DIR/scripts/release/verify-desktop-assets.sh" "$assets_dir" linux-x64 >/dev/null
pass "verify-desktop-assets accepts matching checksums"

printf 'tampered artifact\n' > "$artifact"
if "$ROOT_DIR/scripts/release/verify-desktop-assets.sh" "$assets_dir" linux-x64 >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "verify-desktop-assets should reject checksum mismatches"
fi
grep -q "Checksum verification failed" "$ERR_FILE" || fail "verify-desktop-assets did not explain checksum mismatch"
pass "verify-desktop-assets rejects checksum mismatches"
