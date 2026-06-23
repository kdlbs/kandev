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
"$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform macos-arm64 "$runtime_dir" >/dev/null
pass "verify-desktop-runtime accepts executable runtime"

linux_runtime_dir="$TMP_DIR/linux-runtime"
write_runtime "$linux_runtime_dir" without-helper
chmod_runtime "$linux_runtime_dir"
"$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform linux-x64 "$linux_runtime_dir" >/dev/null
pass "verify-desktop-runtime accepts linux-x64 runtime without helper"

if "$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform linux-arm64 "$linux_runtime_dir" >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "verify-desktop-runtime should require helper for linux-arm64"
fi
grep -q "Missing agentctl linux/amd64 helper" "$ERR_FILE" || fail "verify-desktop-runtime did not explain missing helper"
pass "verify-desktop-runtime requires helper for non-linux-x64 runtime"

linux_output_dir="$TMP_DIR/linux-output"
"$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" \
  --bundle-dir "$linux_runtime_dir" \
  --platform linux-x64 \
  --output-dir "$linux_output_dir" >/dev/null
if [ -e "$linux_output_dir/bin/agentctl-linux-amd64" ]; then
  fail "prepare-desktop-runtime should not copy helper for linux-x64"
fi
pass "prepare-desktop-runtime skips helper for linux-x64"

if "$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" --bundle-dir "$runtime_dir" --output-dir / >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "prepare-desktop-runtime should reject root output directory"
fi
grep -q "Refusing dangerous desktop runtime output directory" "$ERR_FILE" || fail "prepare-desktop-runtime did not explain dangerous output directory"
pass "prepare-desktop-runtime rejects dangerous output directory"
