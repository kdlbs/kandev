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
  mkdir -p "$dir/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/kandev"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/agentctl"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/agentctl-linux-amd64"
}

runtime_dir="$TMP_DIR/runtime"
write_runtime "$runtime_dir"

if "$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" "$runtime_dir" >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "verify-desktop-runtime should reject non-executable binaries"
fi
grep -q "not executable" "$ERR_FILE" || fail "verify-desktop-runtime did not explain non-executable binaries"
pass "verify-desktop-runtime rejects non-executable binaries"

chmod +x "$runtime_dir/bin/kandev" "$runtime_dir/bin/agentctl" "$runtime_dir/bin/agentctl-linux-amd64"
"$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" "$runtime_dir" >/dev/null
pass "verify-desktop-runtime accepts executable runtime"

if "$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" --bundle-dir "$runtime_dir" --output-dir / >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "prepare-desktop-runtime should reject root output directory"
fi
grep -q "Refusing dangerous desktop runtime output directory" "$ERR_FILE" || fail "prepare-desktop-runtime did not explain dangerous output directory"
pass "prepare-desktop-runtime rejects dangerous output directory"
