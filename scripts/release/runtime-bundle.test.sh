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
}

custom_bundle="$TMP_DIR/custom bundle"
make_complete_bundle "$custom_bundle"

if ! bash "$PACKAGE_SCRIPT" --bundle-dir "$custom_bundle" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "validator rejected a complete custom bundle: $(cat "$TMP_DIR/err")"
fi

grep -Fq "Bundle assembled at $custom_bundle" "$TMP_DIR/out" ||
  fail "validator did not report the custom bundle path"

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
grep -Fq -- "--bundle-dir \"$runtime_output\"" "$TMP_DIR/runtime-dry-run" ||
  fail "runtime target did not validate the selected output directory"

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

if ! make --dry-run -C "$ROOT_DIR" service-bundle \
  RUNTIME_VERSION=1.2.3 \
  SERVICE_BUNDLE_DIR="$TMP_DIR/service bundle" \
  >"$TMP_DIR/service-dry-run" 2>"$TMP_DIR/err"; then
  fail "service bundle dry run failed: $(cat "$TMP_DIR/err")"
fi
grep -Fq 'RUNTIME_VERSION="1.2.3"' "$TMP_DIR/service-dry-run" ||
  fail "service bundle replaced the release SemVer with another version"

echo "PASS: runtime bundle contract"
