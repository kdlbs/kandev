#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
OUT_FILE="$TMP_DIR/out"
ERR_FILE="$TMP_DIR/err"
SIGNING_SECRET_ENV=(
  -u APPLE_CERTIFICATE
  -u APPLE_CERTIFICATE_PASSWORD
  -u KEYCHAIN_PASSWORD
  -u APPLE_ID
  -u APPLE_PASSWORD
  -u APPLE_TEAM_ID
  -u APPLE_API_KEY
  -u APPLE_API_ISSUER
  -u APPLE_API_KEY_P8
  -u WINDOWS_CERTIFICATE
  -u WINDOWS_CERTIFICATE_PASSWORD
)
REMOTE_AGENTCTL_HELPERS=(
  agentctl-linux-amd64
  agentctl-linux-arm64
  agentctl-darwin-arm64
  agentctl-darwin-amd64
)

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

pass() {
  echo "PASS: $*"
}

signing_secret_env() {
  env "${SIGNING_SECRET_ENV[@]}" "$@"
}

write_runtime() {
  local dir="$1"
  local helper="${2:-with-helper}"
  mkdir -p "$dir/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/kandev"
  printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/agentctl"
  if [ "$helper" = "with-helper" ]; then
    for remote_helper in "${REMOTE_AGENTCTL_HELPERS[@]}"; do
      printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/bin/$remote_helper"
    done
  fi
}

chmod_runtime() {
  local dir="$1"
  chmod +x "$dir/bin/kandev" "$dir/bin/agentctl"
  for remote_helper in "${REMOTE_AGENTCTL_HELPERS[@]}"; do
    if [ -f "$dir/bin/$remote_helper" ]; then
      chmod +x "$dir/bin/$remote_helper"
    fi
  done
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

missing_darwin_helper_runtime_dir="$TMP_DIR/missing-darwin-helper-runtime"
write_runtime "$missing_darwin_helper_runtime_dir"
rm "$missing_darwin_helper_runtime_dir/bin/agentctl-darwin-arm64"
chmod_runtime "$missing_darwin_helper_runtime_dir"
if "$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform macos-arm64 "$missing_darwin_helper_runtime_dir" >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "verify-desktop-runtime should require darwin helper"
fi
grep -q "Missing agentctl darwin/arm64 helper" "$ERR_FILE" || fail "verify-desktop-runtime did not explain missing darwin helper"
pass "verify-desktop-runtime requires darwin helper"

nonexec_helper_runtime_dir="$TMP_DIR/nonexec-helper-runtime"
write_runtime "$nonexec_helper_runtime_dir"
chmod +x "$nonexec_helper_runtime_dir/bin/kandev" "$nonexec_helper_runtime_dir/bin/agentctl"
if env -u OS "$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform linux-x64 "$nonexec_helper_runtime_dir" >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "verify-desktop-runtime should require executable helper on POSIX hosts"
fi
grep -q "agentctl linux/amd64 helper is not executable" "$ERR_FILE" || fail "verify-desktop-runtime did not explain non-executable helper"
pass "verify-desktop-runtime requires executable helper on POSIX hosts"

windows_runtime_dir="$TMP_DIR/windows-runtime"
mkdir -p "$windows_runtime_dir/bin"
printf 'stub\n' > "$windows_runtime_dir/bin/kandev.exe"
printf 'stub\n' > "$windows_runtime_dir/bin/agentctl.exe"
for remote_helper in "${REMOTE_AGENTCTL_HELPERS[@]}"; do
  printf 'stub\n' > "$windows_runtime_dir/bin/$remote_helper"
done
OS=Windows_NT "$ROOT_DIR/scripts/release/verify-desktop-runtime.sh" --platform windows-x64 "$windows_runtime_dir" >"$OUT_FILE"
grep -q "verified for windows-x64" "$OUT_FILE" || fail "verify-desktop-runtime did not accept Windows-host runtime"
pass "verify-desktop-runtime accepts Windows-host helper without POSIX mode bits"

linux_output_dir="$TMP_DIR/linux-output"
"$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" \
  --bundle-dir "$runtime_dir" \
  --platform linux-x64 \
  --output-dir "$linux_output_dir" >"$OUT_FILE"
grep -q "prepared for linux-x64" "$OUT_FILE" || fail "prepare-desktop-runtime did not include platform output"
for remote_helper in "${REMOTE_AGENTCTL_HELPERS[@]}"; do
  if [ ! -x "$linux_output_dir/bin/$remote_helper" ]; then
    fail "prepare-desktop-runtime should copy executable $remote_helper for linux-x64"
  fi
done
pass "prepare-desktop-runtime copies helpers for linux-x64"

macos_output_dir="$TMP_DIR/macos-output"
"$ROOT_DIR/scripts/release/prepare-desktop-runtime.sh" \
  --bundle-dir "$runtime_dir" \
  --platform macos-arm64 \
  --output-dir "$macos_output_dir" >/dev/null
for remote_helper in "${REMOTE_AGENTCTL_HELPERS[@]}"; do
  if [ ! -x "$macos_output_dir/bin/$remote_helper" ]; then
    fail "prepare-desktop-runtime should copy executable $remote_helper for macos-arm64"
  fi
done
pass "prepare-desktop-runtime copies helpers for non-linux-x64"

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
"$ROOT_DIR/scripts/release/write-sha256.sh" "$artifact" "$artifact.sha256"
"$ROOT_DIR/scripts/release/verify-desktop-assets.sh" "$assets_dir" linux-x64 >/dev/null
pass "verify-desktop-assets accepts matching checksums"

macos_assets_dir="$TMP_DIR/macos-assets"
mkdir -p "$macos_assets_dir"
macos_artifact="$macos_assets_dir/kandev-desktop-macos-arm64-Kandev.dmg"
printf 'unsigned macOS artifact\n' > "$macos_artifact"
"$ROOT_DIR/scripts/release/write-sha256.sh" \
  "$macos_artifact" "$macos_artifact.sha256"
"$ROOT_DIR/scripts/release/verify-desktop-assets.sh" \
  "$macos_assets_dir" macos-arm64 >/dev/null
pass "verify-desktop-assets accepts macOS artifacts without optional updater bundles"

if grep -Eq 'updater_(bundles|signatures)=\(' \
  "$ROOT_DIR/scripts/release/verify-desktop-assets.sh"; then
  fail "verify-desktop-assets uses optional arrays that fail under Bash 3.2 nounset"
fi
pass "verify-desktop-assets avoids Bash 3.2 empty-array nounset failures"

if signing_secret_env "$ROOT_DIR/scripts/release/desktop-signing-ready.sh" macos >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "desktop-signing-ready should report macOS signing not ready without secrets"
fi
grep -q "APPLE_CERTIFICATE" "$ERR_FILE" || fail "desktop-signing-ready did not report missing Apple certificate"
grep -q "APPLE_ID/APPLE_PASSWORD/APPLE_TEAM_ID" "$ERR_FILE" || fail "desktop-signing-ready did not report missing notarization inputs"
pass "desktop-signing-ready reports incomplete macOS inputs"

signing_secret_env \
  APPLE_CERTIFICATE=cert \
  APPLE_CERTIFICATE_PASSWORD=cert-password \
  KEYCHAIN_PASSWORD=keychain-password \
  APPLE_ID=apple@example.test \
  APPLE_PASSWORD=apple-password \
  APPLE_TEAM_ID=TEAMID123 \
  "$ROOT_DIR/scripts/release/desktop-signing-ready.sh" macos >"$OUT_FILE"
grep -q "complete for macos" "$OUT_FILE" || fail "desktop-signing-ready rejected Apple ID notarization inputs"
pass "desktop-signing-ready accepts Apple ID notarization inputs"

signing_secret_env \
  APPLE_CERTIFICATE=cert \
  APPLE_CERTIFICATE_PASSWORD=cert-password \
  KEYCHAIN_PASSWORD=keychain-password \
  APPLE_API_KEY=api-key \
  APPLE_API_ISSUER=api-issuer \
  APPLE_API_KEY_P8=api-key-p8 \
  "$ROOT_DIR/scripts/release/desktop-signing-ready.sh" macos >"$OUT_FILE"
grep -q "complete for macos" "$OUT_FILE" || fail "desktop-signing-ready rejected App Store Connect API notarization inputs"
pass "desktop-signing-ready accepts API key notarization inputs"

if signing_secret_env "$ROOT_DIR/scripts/release/desktop-signing-ready.sh" windows >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "desktop-signing-ready should report Windows signing not ready without secrets"
fi
grep -q "WINDOWS_CERTIFICATE" "$ERR_FILE" || fail "desktop-signing-ready did not report missing Windows certificate"
pass "desktop-signing-ready reports incomplete Windows inputs"

signing_secret_env \
  WINDOWS_CERTIFICATE=windows-cert \
  WINDOWS_CERTIFICATE_PASSWORD=windows-password \
  "$ROOT_DIR/scripts/release/desktop-signing-ready.sh" windows >"$OUT_FILE"
grep -q "complete for windows" "$OUT_FILE" || fail "desktop-signing-ready rejected Windows signing inputs"
pass "desktop-signing-ready accepts Windows signing inputs"

fallback_tools_dir="$TMP_DIR/sha256sum-tools"
fallback_assets_dir="$TMP_DIR/sha256sum-assets"
mkdir -p "$fallback_tools_dir" "$fallback_assets_dir"
for tool in bash basename dirname mkdir mv rm sha256sum; do
  ln -s "$(command -v "$tool")" "$fallback_tools_dir/$tool"
done
fallback_artifact="$fallback_assets_dir/kandev-desktop-linux-x64-fallback.deb"
printf 'desktop artifact\n' > "$fallback_artifact"
PATH="$fallback_tools_dir" bash "$ROOT_DIR/scripts/release/write-sha256.sh" \
  "$fallback_artifact" "$fallback_artifact.sha256"
PATH="$fallback_tools_dir" bash "$ROOT_DIR/scripts/release/verify-desktop-assets.sh" \
  "$fallback_assets_dir" linux-x64 >/dev/null
pass "desktop asset checksums work without shasum"

printf 'tampered artifact\n' > "$artifact"
if "$ROOT_DIR/scripts/release/verify-desktop-assets.sh" "$assets_dir" linux-x64 >"$OUT_FILE" 2>"$ERR_FILE"; then
  fail "verify-desktop-assets should reject checksum mismatches"
fi
grep -q "Checksum verification failed" "$ERR_FILE" || fail "verify-desktop-assets did not explain checksum mismatch"
pass "verify-desktop-assets rejects checksum mismatches"

bash "$ROOT_DIR/scripts/release/updater-signing-ready.test.sh"
bash "$ROOT_DIR/scripts/release/updater-manifest.test.sh"
