#!/usr/bin/env bash
# Publish the main kandev npm package + all @kdlbs/runtime-* optional packages.
#
# Authentication: Trusted Publishers (OIDC). Each of the 6 packages must have
# this workflow configured as its trusted publisher on npmjs.com. The npm CLI
# auto-detects OIDC credentials from GitHub Actions and exchanges them for a
# short-lived publish token. No NPM_TOKEN secret is needed.
#
# Prerequisites:
#   - Five runtime archives from either a GitHub release or a local directory.
#   - Running inside GitHub Actions with `id-token: write` permission set on
#     the publish-npm job. (npm publish from a local shell will fall back to
#     classic auth — but tokens are not the recommended path going forward.)
#
# Usage:
#   publish-npm.sh --version <semver> --dist-tag <latest|nightly> \
#     (--release-tag <git-tag> | --assets-dir <path>)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/release/npm-packages.sh
source "$ROOT_DIR/scripts/release/npm-packages.sh"

bold()  { printf '\033[1m%s\033[0m' "$*"; }
green() { printf '\033[32m%s\033[0m' "$*"; }
red()   { printf '\033[31m%s\033[0m' "$*"; }
yellow(){ printf '\033[33m%s\033[0m' "$*"; }

log()    { echo "  >> $*"; }
log_ok() { echo "  $(green "ok") $*"; }

usage() {
  echo "Usage: $0 --version <semver> --dist-tag <latest|nightly> (--release-tag <git-tag> | --assets-dir <path>)" >&2
}

die() {
  echo "$(red "Error:") $*" >&2
  exit 1
}

npm_view_version() {
  bash "$ROOT_DIR/scripts/release/npm-view-version.sh" "$1"
}

VERSION=""
DIST_TAG=""
RELEASE_TAG=""
SOURCE_ASSETS_DIR=""

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --version)
      [[ "$#" -ge 2 ]] || die "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --dist-tag)
      [[ "$#" -ge 2 ]] || die "--dist-tag requires a value"
      DIST_TAG="$2"
      shift 2
      ;;
    --release-tag)
      [[ "$#" -ge 2 ]] || die "--release-tag requires a value"
      RELEASE_TAG="$2"
      shift 2
      ;;
    --assets-dir)
      [[ "$#" -ge 2 ]] || die "--assets-dir requires a value"
      SOURCE_ASSETS_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      die "unknown argument: $1"
      ;;
  esac
done

[[ -n "$VERSION" ]] || die "--version is required"
[[ -n "$DIST_TAG" ]] || die "--dist-tag is required"
case "$DIST_TAG" in
  latest)
    [[ "$VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || \
      die "--version must be stable X.Y.Z for --dist-tag latest"
    ;;
  nightly)
    [[ "$VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-nightly\.sha[0-9a-f]{12}$ ]] || \
      die "--version must be X.Y.Z-nightly.sha<12-hex> for --dist-tag nightly"
    ;;
  *)
    die "--dist-tag must be latest or nightly"
    ;;
esac

if [[ -n "$RELEASE_TAG" && -n "$SOURCE_ASSETS_DIR" ]]; then
  die "provide exactly one asset source: --release-tag or --assets-dir"
elif [[ -z "$RELEASE_TAG" && -z "$SOURCE_ASSETS_DIR" ]]; then
  die "provide exactly one asset source: --release-tag or --assets-dir"
elif [[ "$DIST_TAG" == "latest" && -z "$RELEASE_TAG" ]]; then
  die "--dist-tag latest requires --release-tag"
elif [[ "$DIST_TAG" == "nightly" && -z "$SOURCE_ASSETS_DIR" ]]; then
  die "--dist-tag nightly requires --assets-dir"
fi

package_already_published() {
  local pkg="$1"
  local published_version
  if ! published_version="$(npm_view_version "${pkg}@${VERSION}")"; then
    die "could not verify whether $pkg@$VERSION is already published"
  fi
  [[ -n "$published_version" ]]
}

record_already_published() {
  local pkg="$1"
  if [[ "$DIST_TAG" == "nightly" ]]; then
    local tagged_version
    if ! tagged_version="$(npm_view_version "${pkg}@${DIST_TAG}")"; then
      die "could not verify $pkg@$DIST_TAG for idempotent publication"
    fi
    if [[ "$tagged_version" != "$VERSION" ]]; then
      die "$pkg@$VERSION exists, but $pkg@$DIST_TAG resolves to '${tagged_version:-nothing}'; refusing idempotent success"
    fi
  fi
  echo "  $(yellow "skip") $pkg@$VERSION already published (treated as idempotent success)" >&2
  ALREADY_PUBLISHED+=("$pkg")
}

WORK_DIR="$(mktemp -d)"
CLI_PACKAGE_JSON="$ROOT_DIR/apps/cli/package.json"
CLI_PACKAGE_BACKUP="$WORK_DIR/cli-package.json"
cp "$CLI_PACKAGE_JSON" "$CLI_PACKAGE_BACKUP"
cleanup() {
  if [[ -f "$CLI_PACKAGE_BACKUP" ]]; then
    cp "$CLI_PACKAGE_BACKUP" "$CLI_PACKAGE_JSON"
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

# -- Resolve and verify release assets ----------------------------------------

if [[ -n "$RELEASE_TAG" ]]; then
  ASSETS_DIR="$WORK_DIR/assets"
  mkdir -p "$ASSETS_DIR"
  log "Downloading release assets for $RELEASE_TAG..."
  for platform in "${RUNTIME_PLATFORMS[@]}"; do
    asset="kandev-${platform}.tar.gz"
    log "  downloading $asset..."
    gh release download "$RELEASE_TAG" --pattern "$asset" --dir "$ASSETS_DIR" || \
      die "GitHub release asset missing: $asset in release $RELEASE_TAG"
  done
else
  ASSETS_DIR="$SOURCE_ASSETS_DIR"
  [[ -d "$ASSETS_DIR" ]] || die "asset directory does not exist: $ASSETS_DIR"
  log "Using release assets from $ASSETS_DIR..."
fi

for platform in "${RUNTIME_PLATFORMS[@]}"; do
  asset="kandev-${platform}.tar.gz"
  [[ -f "$ASSETS_DIR/$asset" ]] || die "release asset missing: $ASSETS_DIR/$asset"
done
log_ok "All 5 platform assets present"

# -- Generate npm runtime packages --------------------------------------------

NPM_PKG_DIR="$WORK_DIR/npm-packages"
bash "$ROOT_DIR/scripts/release/package-npm-runtime.sh" "$VERSION" "$ASSETS_DIR" "$NPM_PKG_DIR"

# -- Publish @kdlbs/runtime-* packages first ---------------------------------

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

  if package_already_published "$pkg"; then
    record_already_published "$pkg"
    continue
  fi

  log "Publishing $pkg@$VERSION..."
  # Capture full npm output so we can show the real error on failure rather
  # than just a generic warning. Distinguish "already published" (idempotent
  # case — fine) from real failures (must abort).
  if output="$(cd "$pkg_dir" && npm publish --access public --provenance --tag "$DIST_TAG" 2>&1)"; then
    log_ok "$pkg@$VERSION published"
  elif echo "$output" | grep -qE "EPUBLISHCONFLICT|cannot publish over the previously published versions|You cannot publish over"; then
    record_already_published "$pkg"
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
log "Setting the launcher version and optionalDependencies to $VERSION..."
node - "$CLI_PACKAGE_JSON" "$VERSION" <<'NODE'
  const fs = require("fs");
  const [path, version] = process.argv.slice(2);
  const pkg = JSON.parse(fs.readFileSync(path, "utf8"));
  pkg.version = version;
  if (pkg.optionalDependencies) {
    for (const k of Object.keys(pkg.optionalDependencies)) {
      pkg.optionalDependencies[k] = version;
    }
  }
  fs.writeFileSync(path, JSON.stringify(pkg, null, 2) + "\n");
NODE
log_ok "Launcher metadata pinned to $VERSION"

# -- Publish main kandev package ----------------------------------------------

echo
echo "$(bold "Publishing kandev@$VERSION...")"
# Same idempotency handling as the runtime packages: capture output, treat
# "already published" as success so partial-failure re-runs converge.
# `prepublishOnly` (in package.json) runs `pnpm build` automatically.
if package_already_published "kandev"; then
  record_already_published "kandev"
elif main_output="$(cd "$ROOT_DIR/apps/cli" && npm publish --access public --provenance --tag "$DIST_TAG" 2>&1)"; then
  log_ok "kandev@$VERSION published"
elif echo "$main_output" | grep -qE "EPUBLISHCONFLICT|cannot publish over the previously published versions|You cannot publish over"; then
  record_already_published "kandev"
else
  echo "  $(red "FAIL") Failed to publish kandev@$VERSION:" >&2
  echo "$main_output" | sed 's/^/      /' >&2
  exit 1
fi

# -- Report -------------------------------------------------------------------
# (WORK_DIR cleanup happens via the EXIT trap above.)

echo
echo "$(green "$(bold "All npm packages published successfully!")")"
if [[ "${#ALREADY_PUBLISHED[@]}" -gt 0 ]]; then
  echo "  $(yellow "note") The following npm packages were already published at $VERSION:"
  for pkg in "${ALREADY_PUBLISHED[@]}"; do
    echo "    - $pkg"
  done
fi
