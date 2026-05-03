#!/usr/bin/env bash
# Publish the main kandev npm package + all @kdlbs/runtime-* optional packages.
#
# Authentication: Trusted Publishers (OIDC). Each of the 6 packages must have
# this workflow configured as its trusted publisher on npmjs.com. The npm CLI
# auto-detects OIDC credentials from GitHub Actions and exchanges them for a
# short-lived publish token. No NPM_TOKEN secret is needed.
#
# Prerequisites:
#   - GitHub release assets for <tag> must already exist (verified before publishing).
#   - Running inside GitHub Actions with `id-token: write` permission set on
#     the publish-npm job. (npm publish from a local shell will fall back to
#     classic auth — but tokens are not the recommended path going forward.)
#
# Usage:
#   publish-npm.sh <version> <tag>
#
# Arguments:
#   version  SemVer string (e.g. 0.17.0)
#   tag      Git tag (e.g. v0.17.0) — used to verify GitHub release assets exist
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

VERSION="${1:?Usage: $0 <version> <tag>}"
TAG="${2:?Usage: $0 <version> <tag>}"

bold()  { printf '\033[1m%s\033[0m' "$*"; }
green() { printf '\033[32m%s\033[0m' "$*"; }
red()   { printf '\033[31m%s\033[0m' "$*"; }
yellow(){ printf '\033[33m%s\033[0m' "$*"; }

log()    { echo "  >> $*"; }
log_ok() { echo "  $(green "ok") $*"; }

die() {
  echo "$(red "Error:") $*" >&2
  exit 1
}

# -- Verify GitHub release assets exist before publishing ---------------------

REQUIRED_PLATFORMS=(linux-x64 linux-arm64 macos-x64 macos-arm64 windows-x64)

log "Verifying GitHub release assets exist for $TAG..."
for platform in "${REQUIRED_PLATFORMS[@]}"; do
  asset="kandev-${platform}.tar.gz"
  if ! gh release view "$TAG" --json assets --jq ".assets[].name" 2>/dev/null | grep -q "^${asset}$"; then
    die "GitHub release asset missing: $asset in release $TAG. Run release workflow first."
  fi
done
log_ok "All 5 platform assets present in GitHub release $TAG"

# -- Download release assets for packaging ------------------------------------

WORK_DIR="$(mktemp -d)"
ASSETS_DIR="$WORK_DIR/assets"
mkdir -p "$ASSETS_DIR"

log "Downloading release assets for $TAG..."
for platform in "${REQUIRED_PLATFORMS[@]}"; do
  asset="kandev-${platform}.tar.gz"
  log "  downloading $asset..."
  gh release download "$TAG" --pattern "$asset" --dir "$ASSETS_DIR"
done
log_ok "Assets downloaded to $ASSETS_DIR"

# -- Generate npm runtime packages --------------------------------------------

NPM_PKG_DIR="$WORK_DIR/npm-packages"
bash "$ROOT_DIR/scripts/release/package-npm-runtime.sh" "$VERSION" "$ASSETS_DIR" "$NPM_PKG_DIR"

# -- Publish @kdlbs/runtime-* packages first ---------------------------------

RUNTIME_PACKAGES=(
  "@kdlbs/runtime-linux-x64"
  "@kdlbs/runtime-linux-arm64"
  "@kdlbs/runtime-darwin-x64"
  "@kdlbs/runtime-darwin-arm64"
  "@kdlbs/runtime-win32-x64"
)

echo
echo "$(bold "Publishing @kdlbs/runtime-* packages...")"
FAILED_PACKAGES=()
ALREADY_PUBLISHED=()

for pkg in "${RUNTIME_PACKAGES[@]}"; do
  scope="${pkg%%/*}"   # @kdlbs
  name="${pkg##*/}"    # runtime-linux-x64
  pkg_dir="$NPM_PKG_DIR/${scope}/${name}"

  if [[ ! -d "$pkg_dir" ]]; then
    echo "  $(red "missing") $pkg_dir (package directory was not generated)" >&2
    FAILED_PACKAGES+=("$pkg")
    continue
  fi

  log "Publishing $pkg@$VERSION..."
  # Capture full npm output so we can show the real error on failure rather
  # than just a generic warning. Distinguish "already published" (idempotent
  # case — fine) from real failures (must abort).
  if output="$(cd "$pkg_dir" && npm publish --access public --provenance 2>&1)"; then
    log_ok "$pkg@$VERSION published"
  elif echo "$output" | grep -qE "EPUBLISHCONFLICT|cannot publish over the previously published versions|You cannot publish over"; then
    echo "  $(yellow "skip") $pkg@$VERSION already published (treated as idempotent success)" >&2
    ALREADY_PUBLISHED+=("$pkg")
  else
    echo "  $(red "FAIL") Failed to publish $pkg@$VERSION:" >&2
    echo "$output" | sed 's/^/      /' >&2
    FAILED_PACKAGES+=("$pkg")
  fi
done

# Abort before publishing main kandev if any runtime publish failed.
# Otherwise users on those platforms would get "No Kandev runtime found"
# after install, with kandev pointing at @kdlbs/runtime-*@VERSION that
# doesn't exist on npm.
if [[ "${#FAILED_PACKAGES[@]}" -gt 0 ]]; then
  echo
  echo "$(red "Error:") The following runtime packages failed to publish:" >&2
  for pkg in "${FAILED_PACKAGES[@]}"; do
    echo "  - $pkg" >&2
  done
  echo >&2
  echo "Refusing to publish main kandev@$VERSION. Fix the runtime publish failures" >&2
  echo "and re-run this script (already-published runtime packages will be skipped)." >&2
  exit 1
fi

# -- Pin optionalDependencies before publishing main kandev ------------------
#
# In committed source, optionalDependencies reference 0.0.0-bootstrap so the
# lockfile resolves during normal development. For the published kandev@VERSION
# package, we want optionalDependencies to point at @kdlbs/runtime-*@VERSION
# so users get matching runtime bundles. The runtime packages were just
# published above, so this version exists on npm now.
log "Pinning optionalDependencies to $VERSION before publishing main package..."
node -e "
  const fs = require('fs');
  const path = '$ROOT_DIR/apps/cli/package.json';
  const pkg = JSON.parse(fs.readFileSync(path, 'utf8'));
  if (pkg.optionalDependencies) {
    for (const k of Object.keys(pkg.optionalDependencies)) {
      pkg.optionalDependencies[k] = '$VERSION';
    }
  }
  fs.writeFileSync(path, JSON.stringify(pkg, null, 2) + '\n');
"
log_ok "optionalDependencies pinned to $VERSION"

# -- Publish main kandev package ----------------------------------------------

echo
echo "$(bold "Publishing kandev@$VERSION...")"
(
  cd "$ROOT_DIR/apps/cli"
  # Build before publishing
  pnpm build
  npm publish --access public --provenance
)
log_ok "kandev@$VERSION published"

# -- Cleanup ------------------------------------------------------------------

rm -rf "$WORK_DIR"

# -- Report -------------------------------------------------------------------

echo
echo "$(green "$(bold "All npm packages published successfully!")")"
if [[ "${#ALREADY_PUBLISHED[@]}" -gt 0 ]]; then
  echo "  $(yellow "note") The following runtime packages were already published at $VERSION:"
  for pkg in "${ALREADY_PUBLISHED[@]}"; do
    echo "    - $pkg"
  done
fi
