#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/release/updater-manifest.mjs"
SIGNING_READY_SCRIPT="$ROOT_DIR/scripts/release/updater-signing-ready.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

TEST_PUBLIC_KEY="$(node - <<'NODE'
const value = `untrusted comment: minisign public key E7620F1842B4E81F
RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO3
`;
process.stdout.write(Buffer.from(value).toString("base64"));
NODE
)"
TEST_SIGNATURE="$(node - <<'NODE'
const value = `untrusted comment: signature from minisign secret key
RUQf6LRCGA9i559r3g7V1qNyJDApGip8MfqcadIgT9CuhV3EMhHoN1mGTkUidF/z7SrlQgXdy8ofjb7bNJJylDOocrCo8KLzZwo=
trusted comment: timestamp:1556193335\tfile:test
y/rUw2y8/hOUYjZU71eHp/Wo1KZ40fGy2VJEDl34XMJM+TX48Ss/17u3IvIfbVR1FkZZSNCisQbuQY+bHwhEBg==`;
process.stdout.write(Buffer.from(value).toString("base64"));
NODE
)"
WRONG_PUBLIC_KEY="$(node - <<'NODE'
const value = `untrusted comment: wrong updater public key
RWQf6LRCGA9i53mlYecO4IzT51TGPpvWucNSCh1CBM0QTaLn73Y7GFO4
`;
process.stdout.write(Buffer.from(value).toString("base64"));
NODE
)"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

pass() {
  echo "PASS: $*"
}

write_artifact() {
  local assets_dir="$1"
  local filename="$2"
  printf 'test' > "$assets_dir/$filename"
  printf '%s\n' "$TEST_SIGNATURE" > "$assets_dir/$filename.sig"
}

write_complete_assets() {
  local assets_dir="$1"
  mkdir -p "$assets_dir"
  write_artifact "$assets_dir" "kandev-desktop-macos-arm64-Kandev.app.tar.gz"
  write_artifact "$assets_dir" "kandev-desktop-macos-x64-Kandev.app.tar.gz"
  write_artifact "$assets_dir" "kandev-desktop-linux-arm64-Kandev.AppImage.tar.gz"
  write_artifact "$assets_dir" "kandev-desktop-linux-x64-Kandev.AppImage.tar.gz"
  write_artifact "$assets_dir" "kandev-desktop-windows-x64-Kandev-setup.nsis.zip"
}

generate_manifest() {
  local assets_dir="$1"
  shift
  node "$SCRIPT" generate \
    --assets-dir "$assets_dir" \
    --output "$assets_dir/latest.json" \
    --version "1.2.3" \
    --tag "v1.2.3" \
    --repository "kdlbs/kandev" \
    --notes-file "$assets_dir/notes.md" \
    --pub-date "2026-07-15T12:00:00Z" \
    --public-key "${UPDATER_TEST_PUBLIC_KEY:-$TEST_PUBLIC_KEY}" \
    "$@"
}

complete_assets="$TMP_DIR/complete"
write_complete_assets "$complete_assets"
printf 'Release notes\n' > "$complete_assets/notes.md"
for file in "$complete_assets"/kandev-desktop-*; do
  "$ROOT_DIR/scripts/release/write-sha256.sh" "$file" "$file.sha256"
done
for platform in macos-arm64 macos-x64 linux-arm64 linux-x64 windows-x64; do
  "$ROOT_DIR/scripts/release/verify-desktop-assets.sh" "$complete_assets" "$platform" >/dev/null
done
pass "desktop asset verification retains updater bundles and signatures"

generate_manifest "$complete_assets"
node "$SCRIPT" verify \
  --assets-dir "$complete_assets" \
  --manifest "$complete_assets/latest.json" \
  --version "1.2.3" \
  --tag "v1.2.3" \
  --repository "kdlbs/kandev" \
  --public-key "$TEST_PUBLIC_KEY"
node - "$complete_assets/latest.json" <<'NODE'
const fs = require("node:fs");
const manifest = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const expected = [
  "darwin-aarch64",
  "darwin-x86_64",
  "linux-aarch64",
  "linux-x86_64",
  "windows-x86_64",
];
if (JSON.stringify(Object.keys(manifest.platforms).sort()) !== JSON.stringify(expected)) {
  throw new Error(`unexpected updater targets: ${Object.keys(manifest.platforms).join(", ")}`);
}
if (manifest.notes !== "Release notes\n" || manifest.pub_date !== "2026-07-15T12:00:00Z") {
  throw new Error("manifest metadata was not preserved");
}
NODE
pass "updater manifest covers every desktop target"

missing_signature="$TMP_DIR/missing-signature"
cp -R "$complete_assets" "$missing_signature"
rm "$missing_signature/kandev-desktop-linux-arm64-Kandev.AppImage.tar.gz.sig" \
  "$missing_signature/latest.json"
if "$ROOT_DIR/scripts/release/verify-desktop-assets.sh" "$missing_signature" linux-arm64 \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "desktop asset verification should reject a missing updater signature"
fi
grep -q "Missing updater signature" "$TMP_DIR/err" || fail "asset verifier did not explain the missing updater signature"
if generate_manifest "$missing_signature" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "generator should reject a missing signature"
fi
grep -q "Missing updater signature" "$TMP_DIR/err" || fail "missing signature error was not actionable"
pass "updater manifest rejects missing signatures"

invalid_signature="$TMP_DIR/invalid-signature"
cp -R "$complete_assets" "$invalid_signature"
printf 'not-a-tauri-signature\n' > "$invalid_signature/kandev-desktop-linux-x64-Kandev.AppImage.tar.gz.sig"
rm "$invalid_signature/latest.json"
if generate_manifest "$invalid_signature" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "generator should reject an invalid signature"
fi
grep -q "Invalid updater signature" "$TMP_DIR/err" || fail "invalid signature error was not actionable"
pass "updater manifest rejects invalid signatures"

wrong_key="$TMP_DIR/wrong-key"
cp -R "$complete_assets" "$wrong_key"
rm "$wrong_key/latest.json"
if UPDATER_TEST_PUBLIC_KEY="$WRONG_PUBLIC_KEY" generate_manifest "$wrong_key" \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "generator should reject signatures made by a different updater key"
fi
grep -Eq "different key|does not match embedded public key|invalid updater public key" \
  "$TMP_DIR/err" || fail "wrong updater key error was not actionable"
pass "updater manifest rejects signatures from a different key"

wrong_url="$TMP_DIR/wrong-url"
cp -R "$complete_assets" "$wrong_url"
WRONG_URL_MANIFEST="$wrong_url/latest.json" node <<'NODE'
const fs = require("node:fs");
const path = process.env.WRONG_URL_MANIFEST;
const manifest = JSON.parse(fs.readFileSync(path, "utf8"));
manifest.platforms["darwin-aarch64"].url = "https://example.test/not-the-release-asset.tar.gz";
fs.writeFileSync(path, `${JSON.stringify(manifest, null, 2)}\n`);
NODE
if node "$SCRIPT" verify \
  --assets-dir "$wrong_url" \
  --manifest "$wrong_url/latest.json" \
  --version "1.2.3" \
  --tag "v1.2.3" \
  --repository "kdlbs/kandev" \
  --public-key "$TEST_PUBLIC_KEY" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "verifier should reject a wrong release URL"
fi
grep -q "Unexpected updater URL" "$TMP_DIR/err" || fail "wrong URL error was not actionable"
pass "updater manifest rejects wrong URLs"

if node "$SCRIPT" generate \
  --assets-dir "$complete_assets" \
  --output "$TMP_DIR/prerelease.json" \
  --version "1.2.3-beta.1" \
  --tag "v1.2.3-beta.1" \
  --repository "kdlbs/kandev" \
  --notes-file "$complete_assets/notes.md" \
  --pub-date "2026-07-15T12:00:00Z" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "generator should reject prerelease metadata"
fi
grep -q "stable SemVer" "$TMP_DIR/err" || fail "prerelease error was not actionable"
pass "updater manifest rejects prerelease versions"

unsigned_assets="$TMP_DIR/unsigned"
mkdir -p "$unsigned_assets"
printf 'Release notes\n' > "$unsigned_assets/notes.md"
printf 'unsigned installer\n' > "$unsigned_assets/kandev-desktop-linux-x64-Kandev.AppImage"
generate_manifest "$unsigned_assets" --allow-unsigned
if [ -e "$unsigned_assets/latest.json" ]; then
  fail "unsigned fallback should not emit latest.json"
fi
pass "unsigned installers are excluded from the updater feed"

partial_assets="$TMP_DIR/partial"
mkdir -p "$partial_assets"
printf 'Release notes\n' > "$partial_assets/notes.md"
write_artifact "$partial_assets" "kandev-desktop-linux-x64-Kandev.AppImage.tar.gz"
if generate_manifest "$partial_assets" --allow-unsigned >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "partial signed updater sets should fail"
fi
grep -q "Incomplete updater artifact set" "$TMP_DIR/err" || fail "partial set error was not actionable"
pass "partial signed updater feeds fail closed"

TAURI_SIGNING_PRIVATE_KEY=test-key MACOS_SIGNING_ENABLED=false \
  bash "$SIGNING_READY_SCRIPT" macos-arm64 >/dev/null
pass "macOS updater artifacts require only the updater key"

TAURI_SIGNING_PRIVATE_KEY=test-key WINDOWS_SIGNING_ENABLED=false \
  bash "$SIGNING_READY_SCRIPT" windows-x64 >/dev/null
pass "Windows updater artifacts require only the updater key"

TAURI_SIGNING_PRIVATE_KEY=test-key MACOS_SIGNING_ENABLED=false WINDOWS_SIGNING_ENABLED=false \
  bash "$SIGNING_READY_SCRIPT" linux-arm64 >/dev/null
pass "Linux updater artifacts require only the updater key"

if env -u TAURI_SIGNING_PRIVATE_KEY bash "$SIGNING_READY_SCRIPT" macos-arm64 \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "updater artifacts should require the Tauri updater key on every platform"
fi
grep -q "require TAURI_SIGNING_PRIVATE_KEY" "$TMP_DIR/err" || \
  fail "missing updater key failure was not actionable"
pass "every updater artifact requires the Tauri updater key"
