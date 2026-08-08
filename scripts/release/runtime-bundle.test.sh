#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PACKAGE_SCRIPT="$ROOT_DIR/scripts/release/package-bundle.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

write_macho() {
  local path="$1"
  local signature="$2"
  node - "$path" "$signature" <<'NODE'
const fs = require("node:fs");

const path = process.argv[2];
const signature = process.argv[3];
const headerSize = 32;
const loadCommandSize = 16;
const codeDirectorySize = 83;
const superBlobSize = 20 + codeDirectorySize;
const signatureOffset = headerSize + loadCommandSize;
const data = Buffer.alloc(signatureOffset + (signature === "signed" ? superBlobSize : 0));
data.writeUInt32LE(0xfeedfacf, 0);
data.writeUInt32LE(0x0100000c, 4);
data.writeUInt32LE(1, 16);
data.writeUInt32LE(loadCommandSize, 20);
data.writeUInt32LE(signature === "unsigned" ? 0x1b : 0x1d, headerSize);
data.writeUInt32LE(loadCommandSize, headerSize + 4);
if (signature !== "unsigned") {
  data.writeUInt32LE(signatureOffset, headerSize + 8);
  data.writeUInt32LE(superBlobSize, headerSize + 12);
}
if (signature === "signed") {
  data.writeUInt32BE(0xfade0cc0, signatureOffset);
  data.writeUInt32BE(superBlobSize, signatureOffset + 4);
  data.writeUInt32BE(1, signatureOffset + 8);
  data.writeUInt32BE(0, signatureOffset + 12);
  data.writeUInt32BE(20, signatureOffset + 16);

  const codeDirectoryOffset = signatureOffset + 20;
  data.writeUInt32BE(0xfade0c02, codeDirectoryOffset);
  data.writeUInt32BE(codeDirectorySize, codeDirectoryOffset + 4);
  data.writeUInt32BE(0x20001, codeDirectoryOffset + 8);
  data.writeUInt32BE(0x2, codeDirectoryOffset + 12);
  data.writeUInt32BE(51, codeDirectoryOffset + 16);
  data.writeUInt32BE(44, codeDirectoryOffset + 20);
  data.writeUInt32BE(0, codeDirectoryOffset + 24);
  data.writeUInt32BE(1, codeDirectoryOffset + 28);
  data.writeUInt32BE(signatureOffset, codeDirectoryOffset + 32);
  data.writeUInt8(32, codeDirectoryOffset + 36);
  data.writeUInt8(2, codeDirectoryOffset + 37);
  data.writeUInt8(12, codeDirectoryOffset + 39);
  data.write("kandev\0", codeDirectoryOffset + 44, "utf8");
}
fs.writeFileSync(path, data, { mode: 0o755 });
NODE
}

make_complete_bundle() {
  local bundle_dir="$1"
  mkdir -p "$bundle_dir/bin"
  touch \
    "$bundle_dir/bin/kandev" \
    "$bundle_dir/bin/agentctl" \
    "$bundle_dir/bin/agentctl-linux-amd64" \
    "$bundle_dir/bin/agentctl-linux-arm64" \
    "$bundle_dir/bin/agentctl-darwin-arm64" \
    "$bundle_dir/bin/agentctl-darwin-amd64"
  chmod +x "$bundle_dir/bin/"*
  write_macho "$bundle_dir/bin/agentctl-darwin-arm64" signed
}

custom_bundle="$TMP_DIR/custom bundle"
make_complete_bundle "$custom_bundle"

if ! bash "$PACKAGE_SCRIPT" --bundle-dir "$custom_bundle" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator rejected a complete custom bundle: $(cat "$TMP_DIR/err")"
fi

grep -Fq "Bundle assembled at $custom_bundle" "$TMP_DIR/out" ||
  fail "validator did not report the custom bundle path"

extra_bundle="$TMP_DIR/extra artifact"
cp -R "$custom_bundle" "$extra_bundle"
touch "$extra_bundle/bin/debugger"
chmod +x "$extra_bundle/bin/debugger"
if bash "$PACKAGE_SCRIPT" --bundle-dir "$extra_bundle" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator accepted a seventh runtime artifact"
fi
grep -Fq "Unexpected runtime artifact debugger" "$TMP_DIR/err" ||
  fail "extra-artifact error was not actionable"

missing_bundle="$TMP_DIR/missing helper"
cp -R "$custom_bundle" "$missing_bundle"
rm "$missing_bundle/bin/agentctl-linux-arm64"
if bash "$PACKAGE_SCRIPT" --bundle-dir "$missing_bundle" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator accepted a bundle with a missing remote helper"
fi
grep -Fq "Missing remote agentctl helper agentctl-linux-arm64" "$TMP_DIR/err" ||
  fail "missing-helper error was not actionable"

non_executable_bundle="$TMP_DIR/non-executable helper"
cp -R "$custom_bundle" "$non_executable_bundle"
chmod -x "$non_executable_bundle/bin/agentctl-darwin-arm64"
if bash "$PACKAGE_SCRIPT" --bundle-dir "$non_executable_bundle" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator accepted a non-executable runtime binary"
fi
grep -Fq "Runtime binary agentctl-darwin-arm64 is not executable" "$TMP_DIR/err" ||
  fail "non-executable-binary error was not actionable"

unsigned_bundle="$TMP_DIR/unsigned darwin helper"
cp -R "$custom_bundle" "$unsigned_bundle"
write_macho "$unsigned_bundle/bin/agentctl-darwin-arm64" unsigned
if bash "$PACKAGE_SCRIPT" --bundle-dir "$unsigned_bundle" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator accepted an unsigned darwin/arm64 helper"
fi
grep -Fq "agentctl-darwin-arm64 is not code-signed" "$TMP_DIR/err" ||
  fail "unsigned-helper error was not actionable"

invalid_signature_bundle="$TMP_DIR/invalid darwin signature"
cp -R "$custom_bundle" "$invalid_signature_bundle"
write_macho "$invalid_signature_bundle/bin/agentctl-darwin-arm64" invalid-signature
if bash "$PACKAGE_SCRIPT" --bundle-dir "$invalid_signature_bundle" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator accepted a darwin/arm64 helper with missing signature data"
fi
grep -Fq "agentctl-darwin-arm64 does not contain a valid code signature" "$TMP_DIR/err" ||
  fail "invalid-signature error was not actionable"

unparsable_bundle="$TMP_DIR/unparsable darwin helper"
cp -R "$custom_bundle" "$unparsable_bundle"
printf 'not a Mach-O\n' > "$unparsable_bundle/bin/agentctl-darwin-arm64"
chmod +x "$unparsable_bundle/bin/agentctl-darwin-arm64"
if bash "$PACKAGE_SCRIPT" --bundle-dir "$unparsable_bundle" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator accepted an unparsable darwin/arm64 helper"
fi
grep -Fq "agentctl-darwin-arm64 is not a parsable thin darwin/arm64 Mach-O" "$TMP_DIR/err" ||
  fail "unparsable-helper error was not actionable"

if bash "$PACKAGE_SCRIPT" --bundle-dir / >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator accepted the filesystem root as a bundle"
fi
grep -Fq "bundle directory must not be /" "$TMP_DIR/err" ||
  fail "unsafe-root error was not actionable"

if bash "$PACKAGE_SCRIPT" --unknown >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator accepted an unknown argument"
fi
grep -Fq "usage:" "$TMP_DIR/err" || fail "invalid arguments did not print usage"

if ! make --dry-run -C "$ROOT_DIR/apps/backend" build-runtime \
  VERSION=1.2.3 GOFLAGS=-trimpath >"$TMP_DIR/backend-dry-run" 2>"$TMP_DIR/err"; then
  fail "backend runtime target is unavailable: $(cat "$TMP_DIR/err")"
fi

expected_runtime_binaries="$(printf '%s\n' \
  bin/agentctl \
  bin/agentctl-darwin-amd64 \
  bin/agentctl-darwin-arm64 \
  bin/agentctl-linux-amd64 \
  bin/agentctl-linux-arm64 \
  bin/kandev)"
actual_runtime_binaries="$(grep -oE 'bin/[[:alnum:]_.-]+' "$TMP_DIR/backend-dry-run" | sort -u)"
[ "$actual_runtime_binaries" = "$expected_runtime_binaries" ] ||
  fail "backend runtime target built an unexpected binary set: $actual_runtime_binaries"

grep -Fq -- "-tags fts5" "$TMP_DIR/backend-dry-run" ||
  fail "backend runtime target omitted the fts5 build tag"
grep -Fq -- "-X main.Version=1.2.3" "$TMP_DIR/backend-dry-run" ||
  fail "backend runtime target did not forward VERSION"

runtime_output="$TMP_DIR/runtime output"
if ! make --dry-run -C "$ROOT_DIR" runtime-bundle \
  PNPM="pnpm with current" \
  GOFLAGS=-trimpath \
  RUNTIME_VERSION=1.2.3 \
  RUNTIME_BUNDLE_DIR="$runtime_output" \
  >"$TMP_DIR/runtime-dry-run" 2>"$TMP_DIR/err"; then
  fail "root runtime target is unavailable: $(cat "$TMP_DIR/err")"
fi

grep -Fq "pnpm with current --filter @kandev/web build" "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not build the Vite web package"
grep -Fq "internal/webapp/embedded/generated" "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not sync embedded web assets"
grep -Fq "build-runtime" "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not invoke the minimal backend target"
grep -Fq "VERSION=\"1.2.3\"" "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not forward RUNTIME_VERSION"
grep -Fq "GOFLAGS=\"-trimpath\"" "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not forward GOFLAGS"
grep -Fq -- 'requested_bundle_dir='"\"$runtime_output\"" "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not retain the selected output directory"
grep -Fq -- '--bundle-dir "$staging_bundle_dir"' "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not validate the staged bundle"
grep -Fq 'resolved_bundle_dir=' "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not canonicalize the bundle directory before writing"
grep -Fq 'pwd -P' "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not resolve symlinked bundle directories"
grep -Fq 'mktemp -d "$resolved_bundle_dir/.runtime-bundle.XXXXXX"' "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not create a clean staging bundle"
if grep -Fq "rm -rf \"$runtime_output/bin\"" "$TMP_DIR/runtime-dry-run"; then
  fail "runtime target recursively deleted through an unresolved bundle path"
fi

for binary in \
  kandev \
  agentctl \
  agentctl-linux-amd64 \
  agentctl-linux-arm64 \
  agentctl-darwin-arm64 \
  agentctl-darwin-amd64; do
  grep -Fq "apps/backend/bin/$binary" "$TMP_DIR/runtime-dry-run" ||
    fail "runtime target did not package $binary"
done

if grep -Eq 'pnpm( with current)? .*install|playwright install' "$TMP_DIR/runtime-dry-run"; then
  fail "runtime target attempted to install dependencies or Playwright"
fi

runtime_backend="$TMP_DIR/runtime backend"
mkdir -p "$runtime_backend/bin"
for binary in \
  kandev \
  agentctl \
  agentctl-linux-amd64 \
  agentctl-linux-arm64 \
  agentctl-darwin-arm64 \
  agentctl-darwin-amd64; do
  printf 'new %s\n' "$binary" > "$runtime_backend/bin/$binary"
  chmod +x "$runtime_backend/bin/$binary"
done
write_macho "$runtime_backend/bin/agentctl-darwin-arm64" signed

failed_copy_bundle="$TMP_DIR/failed copy bundle"
make_complete_bundle "$failed_copy_bundle"
cp -R "$failed_copy_bundle/bin" "$TMP_DIR/failed copy snapshot"
rm "$runtime_backend/bin/agentctl-darwin-amd64"
if make -C "$ROOT_DIR" runtime-bundle \
  MAKE=true \
  BACKEND_DIR="$runtime_backend" \
  RUNTIME_VERSION=1.2.3 \
  RUNTIME_BUNDLE_DIR="$failed_copy_bundle" \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "runtime target ignored a failed binary copy"
fi
if ! diff -r "$TMP_DIR/failed copy snapshot" "$failed_copy_bundle/bin" >/dev/null; then
  fail "failed binary copy changed the previous runtime bundle"
fi

printf 'new agentctl-darwin-amd64\n' > "$runtime_backend/bin/agentctl-darwin-amd64"
chmod +x "$runtime_backend/bin/agentctl-darwin-amd64"
stale_bundle="$TMP_DIR/stale bundle"
make_complete_bundle "$stale_bundle"
touch "$stale_bundle/bin/debugger"
chmod +x "$stale_bundle/bin/debugger"
if ! make -C "$ROOT_DIR" runtime-bundle \
  MAKE=true \
  BACKEND_DIR="$runtime_backend" \
  RUNTIME_VERSION=1.2.3 \
  RUNTIME_BUNDLE_DIR="$stale_bundle" \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "repeat runtime bundle failed instead of replacing stale artifacts: $(cat "$TMP_DIR/err")"
fi
[ ! -e "$stale_bundle/bin/debugger" ] ||
  fail "repeat runtime bundle retained a stale artifact"

root_symlink="$TMP_DIR/root symlink"
ln -s / "$root_symlink"
if make -C "$ROOT_DIR" runtime-bundle \
  MAKE=true \
  RUNTIME_BUNDLE_DIR="$root_symlink" \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "runtime target accepted a bundle directory resolving to the filesystem root"
fi
grep -Fq "RUNTIME_BUNDLE_DIR must not resolve to /" "$TMP_DIR/out" ||
  fail "symlinked-root error was not actionable"

if ! make --dry-run -C "$ROOT_DIR" service-bundle \
  RUNTIME_VERSION=1.2.3 \
  SERVICE_BUNDLE_DIR="$TMP_DIR/service bundle" \
  >"$TMP_DIR/service-dry-run" 2>"$TMP_DIR/err"; then
  fail "service bundle dry run failed: $(cat "$TMP_DIR/err")"
fi
grep -Fq 'RUNTIME_VERSION="1.2.3"' "$TMP_DIR/service-dry-run" ||
  fail "service bundle replaced the release SemVer with another version"

echo "PASS: runtime bundle contract"
