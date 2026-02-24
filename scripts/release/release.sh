#!/usr/bin/env bash
# -----------------------------------------------------------------------------
# release.sh
#
# Unified release script for:
# - CLI npm release (apps/cli)
# - App runtime release (git tag that triggers GitHub Actions release workflow)
# - Both in one run
#
# Versioning model:
# - App tag format: vM.m
# - CLI npm version format: M.m.0
# - Default bump mode: minor (major bump is explicit)
#
# High-level flow:
# 1) Validate preconditions (tools, clean tree, main branch, fetch tags)
# 2) Select release target (cli | app | both)
# 3) Auto-detect current versions/tags and compute next versions
# 4) Show a release plan and require confirmation
# 5) Execute release actions:
#    - CLI: bump package version, install deps, build, commit/push, npm publish
#    - App: create/push release tag
#
# Safety:
# - Use --dry-run to preview every action without mutating repo or publishing.
# - Use --yes for non-interactive confirmation.
# -----------------------------------------------------------------------------
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLI_PACKAGE_JSON="$ROOT_DIR/apps/cli/package.json"

TARGET=""
DEFAULT_BUMP="minor"
AUTO_YES=0
DRY_RUN=0

CLI_SELECTED=0
APP_SELECTED=0

CURRENT_BRANCH=""
CURRENT_CLI_VERSION=""
CURRENT_CLI_MAJOR=0
CURRENT_CLI_MINOR=0
CURRENT_CLI_PATCH=0
CURRENT_APP_TAG=""
CURRENT_APP_MAJOR=0
CURRENT_APP_MINOR=0

CLI_BUMP=""
APP_BUMP=""
NEXT_CLI_VERSION=""
NEXT_APP_TAG=""

usage() {
  cat <<'EOF'
Usage: scripts/release/release.sh [options]

Options:
  --target <cli|app|both>  Release target. If omitted, prompt interactively.
  --bump <minor|major>     Bump mode. Default: minor.
  --yes                    Skip confirmation prompts.
  --dry-run                Print actions without making changes/publishes.
  --help, -h               Show help.

Versioning:
  - App tags: vM.m
  - CLI package version: M.m.0

Prerequisites:
  - Run from a clean working tree on branch main.
  - Required tools: git, node, npm, pnpm.

Examples:
  # Fully interactive release wizard
  scripts/release/release.sh

  # Non-interactive CLI minor release
  scripts/release/release.sh --target cli --bump minor --yes

  # Preview both releases with major bump
  scripts/release/release.sh --target both --bump major --dry-run
EOF
}

die() {
  echo "Error: $*" >&2
  exit 1
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

run_cmd() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] $*"
    return 0
  fi
  "$@"
}

confirm() {
  local prompt="$1"
  if [[ "$AUTO_YES" -eq 1 ]]; then
    return 0
  fi
  local answer
  read -r -p "$prompt [y/N]: " answer
  answer="$(echo "${answer:-}" | tr '[:upper:]' '[:lower:]')"
  [[ "$answer" == "y" || "$answer" == "yes" ]]
}

validate_bump() {
  case "$1" in
    minor|major) ;;
    *) die "Invalid bump '$1'. Use 'minor' or 'major'." ;;
  esac
}

parse_target() {
  case "$1" in
    cli)
      CLI_SELECTED=1
      APP_SELECTED=0
      ;;
    app)
      CLI_SELECTED=0
      APP_SELECTED=1
      ;;
    both)
      CLI_SELECTED=1
      APP_SELECTED=1
      ;;
    *)
      die "Invalid target '$1'. Use cli, app, or both."
      ;;
  esac
}

select_target_interactive() {
  echo "Select release target:"
  echo "  1) cli"
  echo "  2) app"
  echo "  3) both"
  local choice
  read -r -p "Choice [1/2/3]: " choice
  case "$choice" in
    1) parse_target "cli" ;;
    2) parse_target "app" ;;
    3) parse_target "both" ;;
    *) die "Invalid choice '$choice'." ;;
  esac
}

parse_cli_version() {
  local version="$1"
  if [[ ! "$version" =~ ^([0-9]+)\.([0-9]+)(\.([0-9]+))?$ ]]; then
    die "CLI version '$version' is not valid. Expected M.m.0 format."
  fi
  CURRENT_CLI_MAJOR="${BASH_REMATCH[1]}"
  CURRENT_CLI_MINOR="${BASH_REMATCH[2]}"
  CURRENT_CLI_PATCH="${BASH_REMATCH[4]:-0}"
}

detect_current_cli_version() {
  [[ -f "$CLI_PACKAGE_JSON" ]] || die "Missing $CLI_PACKAGE_JSON"
  CURRENT_CLI_VERSION="$(node -p "require('$CLI_PACKAGE_JSON').version")"
  parse_cli_version "$CURRENT_CLI_VERSION"
}

detect_current_app_version() {
  local tags
  tags="$(git -C "$ROOT_DIR" tag --list 'v*')"

  local found=0
  local best_major=0
  local best_minor=0
  local tag

  while IFS= read -r tag; do
    [[ -z "$tag" ]] && continue
    if [[ "$tag" =~ ^v([0-9]+)\.([0-9]+)(\.[0-9]+)?$ ]]; then
      local major="${BASH_REMATCH[1]}"
      local minor="${BASH_REMATCH[2]}"
      if [[ "$found" -eq 0 ]] || (( 10#$major > 10#$best_major )) || \
        (( 10#$major == 10#$best_major && 10#$minor > 10#$best_minor )); then
        found=1
        best_major="$major"
        best_minor="$minor"
      fi
    fi
  done <<<"$tags"

  CURRENT_APP_MAJOR="$best_major"
  CURRENT_APP_MINOR="$best_minor"
  CURRENT_APP_TAG="v${CURRENT_APP_MAJOR}.${CURRENT_APP_MINOR}"
}

next_minor_pair() {
  local major="$1"
  local minor="$2"
  echo "${major}.$((10#$minor + 1))"
}

next_major_pair() {
  local major="$1"
  echo "$((10#$major + 1)).0"
}

choose_bump() {
  local subject="$1"
  local current="$2"
  local next_minor="$3"
  local next_major="$4"

  if [[ -n "$DEFAULT_BUMP" ]]; then
    validate_bump "$DEFAULT_BUMP"
  fi

  if [[ "$AUTO_YES" -eq 1 && -n "$DEFAULT_BUMP" ]]; then
    echo "$DEFAULT_BUMP"
    return
  fi

  if [[ -n "$DEFAULT_BUMP" && "$TARGET" != "" ]]; then
    echo "$DEFAULT_BUMP"
    return
  fi

  echo "$subject version: $current"
  echo "  1) minor -> $next_minor (default)"
  echo "  2) major -> $next_major"
  local choice
  read -r -p "Choose bump [1/2]: " choice
  case "${choice:-1}" in
    1) echo "minor" ;;
    2) echo "major" ;;
    *) die "Invalid bump choice '$choice'." ;;
  esac
}

compute_next_versions() {
  if [[ "$CLI_SELECTED" -eq 1 ]]; then
    local cli_minor_pair cli_major_pair
    cli_minor_pair="$(next_minor_pair "$CURRENT_CLI_MAJOR" "$CURRENT_CLI_MINOR")"
    cli_major_pair="$(next_major_pair "$CURRENT_CLI_MAJOR")"

    CLI_BUMP="$(choose_bump "CLI" "${CURRENT_CLI_MAJOR}.${CURRENT_CLI_MINOR}.0" \
      "${cli_minor_pair}.0" "${cli_major_pair}.0")"
    validate_bump "$CLI_BUMP"

    if [[ "$CLI_BUMP" == "minor" ]]; then
      NEXT_CLI_VERSION="${cli_minor_pair}.0"
    else
      NEXT_CLI_VERSION="${cli_major_pair}.0"
    fi
  fi

  if [[ "$APP_SELECTED" -eq 1 ]]; then
    local app_minor_pair app_major_pair
    app_minor_pair="$(next_minor_pair "$CURRENT_APP_MAJOR" "$CURRENT_APP_MINOR")"
    app_major_pair="$(next_major_pair "$CURRENT_APP_MAJOR")"

    APP_BUMP="$(choose_bump "App" "$CURRENT_APP_TAG" "v${app_minor_pair}" "v${app_major_pair}")"
    validate_bump "$APP_BUMP"

    if [[ "$APP_BUMP" == "minor" ]]; then
      NEXT_APP_TAG="v${app_minor_pair}"
    else
      NEXT_APP_TAG="v${app_major_pair}"
    fi
  fi
}

ensure_prerequisites() {
  # Keep release operations deterministic and reproducible.
  command_exists git || die "git is required."
  command_exists node || die "node is required."
  command_exists npm || die "npm is required."
  command_exists pnpm || die "pnpm is required."

  CURRENT_BRANCH="$(git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD)"
  [[ "$CURRENT_BRANCH" == "main" ]] || die "Release must run from main branch (current: $CURRENT_BRANCH)."

  if [[ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]]; then
    die "Working tree is not clean. Commit or stash changes before releasing."
  fi

  run_cmd git -C "$ROOT_DIR" fetch --tags
}

ensure_app_tag_available() {
  [[ "$APP_SELECTED" -eq 1 ]] || return 0

  if git -C "$ROOT_DIR" rev-parse "$NEXT_APP_TAG" >/dev/null 2>&1; then
    die "Tag $NEXT_APP_TAG already exists locally."
  fi

  if git -C "$ROOT_DIR" ls-remote --tags origin "refs/tags/$NEXT_APP_TAG" | grep -q .; then
    die "Tag $NEXT_APP_TAG already exists on origin."
  fi
}

print_plan() {
  echo
  echo "Release plan:"
  echo "  branch: $CURRENT_BRANCH"
  if [[ "$CLI_SELECTED" -eq 1 ]]; then
    echo "  cli: $CURRENT_CLI_VERSION -> $NEXT_CLI_VERSION (${CLI_BUMP})"
  fi
  if [[ "$APP_SELECTED" -eq 1 ]]; then
    echo "  app tag: $CURRENT_APP_TAG -> $NEXT_APP_TAG (${APP_BUMP})"
  fi
  echo "  dry-run: $DRY_RUN"
  echo
}

commit_message() {
  if [[ "$CLI_SELECTED" -eq 1 && "$APP_SELECTED" -eq 1 ]]; then
    echo "release: app $NEXT_APP_TAG, cli $NEXT_CLI_VERSION"
  elif [[ "$CLI_SELECTED" -eq 1 ]]; then
    echo "release: cli $NEXT_CLI_VERSION"
  else
    echo ""
  fi
}

apply_cli_release() {
  [[ "$CLI_SELECTED" -eq 1 ]] || return 0

  # Update only the published CLI package version. Runtime app release is tag-based.
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] cd $ROOT_DIR/apps/cli && npm version --no-git-tag-version $NEXT_CLI_VERSION"
  else
    (cd "$ROOT_DIR/apps/cli" && npm version --no-git-tag-version "$NEXT_CLI_VERSION")
  fi

  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] pnpm -C $ROOT_DIR/apps install --frozen-lockfile"
    echo "[dry-run] pnpm -C $ROOT_DIR/apps/cli build"
  else
    pnpm -C "$ROOT_DIR/apps" install --frozen-lockfile
    pnpm -C "$ROOT_DIR/apps/cli" build
  fi
}

commit_and_push_if_needed() {
  local msg
  msg="$(commit_message)"
  [[ -n "$msg" ]] || return 0

  # App-only releases are tag-only and do not require a version commit.
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] git -C $ROOT_DIR add apps/cli/package.json"
    echo "[dry-run] git -C $ROOT_DIR commit -m \"$msg\""
    echo "[dry-run] git -C $ROOT_DIR push origin $CURRENT_BRANCH"
    return 0
  fi

  git -C "$ROOT_DIR" add apps/cli/package.json
  if git -C "$ROOT_DIR" diff --cached --quiet; then
    echo "No staged changes to commit."
    return 0
  fi

  git -C "$ROOT_DIR" commit -m "$msg"
  git -C "$ROOT_DIR" push origin "$CURRENT_BRANCH"
}

create_and_push_app_tag() {
  [[ "$APP_SELECTED" -eq 1 ]] || return 0
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] git -C $ROOT_DIR tag -a $NEXT_APP_TAG -m \"release: $NEXT_APP_TAG\""
    echo "[dry-run] git -C $ROOT_DIR push origin $NEXT_APP_TAG"
    return 0
  fi

  git -C "$ROOT_DIR" tag -a "$NEXT_APP_TAG" -m "release: $NEXT_APP_TAG"
  git -C "$ROOT_DIR" push origin "$NEXT_APP_TAG"
}

publish_cli() {
  [[ "$CLI_SELECTED" -eq 1 ]] || return 0
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] cd $ROOT_DIR/apps/cli && npm publish --access public"
    return 0
  fi
  (cd "$ROOT_DIR/apps/cli" && npm publish --access public)
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --target)
        [[ $# -ge 2 ]] || die "--target requires a value."
        TARGET="$2"
        shift 2
        ;;
      --bump)
        [[ $# -ge 2 ]] || die "--bump requires a value."
        DEFAULT_BUMP="$2"
        shift 2
        ;;
      --yes)
        AUTO_YES=1
        shift
        ;;
      --dry-run)
        DRY_RUN=1
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        die "Unknown argument '$1'."
        ;;
    esac
  done
}

main() {
  parse_args "$@"
  validate_bump "$DEFAULT_BUMP"
  ensure_prerequisites

  if [[ -n "$TARGET" ]]; then
    parse_target "$TARGET"
  else
    # Interactive mode should prompt for target and bump choices.
    DEFAULT_BUMP=""
    select_target_interactive
  fi

  detect_current_cli_version
  detect_current_app_version
  compute_next_versions
  ensure_app_tag_available
  print_plan

  if ! confirm "Proceed with release?"; then
    echo "Release cancelled."
    exit 0
  fi

  apply_cli_release
  commit_and_push_if_needed
  create_and_push_app_tag
  publish_cli

  echo
  echo "Release complete."
  if [[ "$CLI_SELECTED" -eq 1 ]]; then
    echo "  published CLI: $NEXT_CLI_VERSION"
  fi
  if [[ "$APP_SELECTED" -eq 1 ]]; then
    echo "  pushed app tag: $NEXT_APP_TAG"
  fi
}

main "$@"
