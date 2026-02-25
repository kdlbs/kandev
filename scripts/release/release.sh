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

# -- Formatting helpers -------------------------------------------------------

bold() { printf '\033[1m%s\033[0m' "$*"; }
dim() { printf '\033[2m%s\033[0m' "$*"; }
green() { printf '\033[32m%s\033[0m' "$*"; }
yellow() { printf '\033[33m%s\033[0m' "$*"; }
red() { printf '\033[31m%s\033[0m' "$*"; }
cyan() { printf '\033[36m%s\033[0m' "$*"; }

log() { echo "  $(dim ">>") $*"; }
log_step() { echo; echo "$(bold "[$1]") $2"; }
log_ok() { echo "  $(green "ok") $*"; }
log_skip() { echo "  $(dim "skip") $*"; }

# -- Core helpers --------------------------------------------------------------

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
  echo "$(red "Error:") $*" >&2
  exit 1
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

run_cmd() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  $(yellow "[dry-run]") $*"
    return 0
  fi
  "$@"
}

confirm() {
  local prompt="$1"
  if [[ "$AUTO_YES" -eq 1 ]]; then
    return 0
  fi
  echo
  local answer
  read -r -p "$(bold "$prompt") [y/N]: " answer
  answer="$(echo "${answer:-}" | tr '[:upper:]' '[:lower:]')"
  [[ "$answer" == "y" || "$answer" == "yes" ]]
}

validate_bump() {
  case "$1" in
    minor|major) ;;
    *) die "Invalid bump '$1'. Use 'minor' or 'major'." ;;
  esac
}

# -- Target selection ----------------------------------------------------------

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
  echo
  echo "$(bold "What do you want to release?")"
  echo "  $(cyan "1)") cli   $(dim "- npm publish apps/cli")"
  echo "  $(cyan "2)") app   $(dim "- git tag for GitHub Actions release")"
  echo "  $(cyan "3)") both  $(dim "- cli + app together")"
  echo
  local choice
  read -r -p "$(bold "Choice") [1=cli / 2=app / 3=both]: " choice
  case "$choice" in
    1|cli)  parse_target "cli" ;;
    2|app)  parse_target "app" ;;
    3|both) parse_target "both" ;;
    *) die "Invalid choice '$choice'. Enter 1, 2, 3, cli, app, or both." ;;
  esac

  local selected=""
  [[ "$CLI_SELECTED" -eq 1 ]] && selected+="cli "
  [[ "$APP_SELECTED" -eq 1 ]] && selected+="app "
  log "Selected target: $(bold "$selected")"
}

# -- Version parsing -----------------------------------------------------------

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
  log "Current CLI version: $(bold "$CURRENT_CLI_VERSION") $(dim "(from apps/cli/package.json)")"
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

  if [[ "$found" -eq 1 ]]; then
    log "Current app tag:     $(bold "$CURRENT_APP_TAG") $(dim "(latest git tag matching vM.m)")"
  else
    log "Current app tag:     $(dim "none found") $(dim "(will start at v0.1)")"
  fi
}

# -- Bump selection ------------------------------------------------------------

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

  echo >&2
  echo "$(bold "$subject version bump") $(dim "(current: $current)")" >&2
  echo "  $(cyan "1)") minor -> $(bold "$next_minor") $(dim "(default)")" >&2
  echo "  $(cyan "2)") major -> $(bold "$next_major")" >&2
  local choice
  read -r -p "$(bold "Bump") [1=minor / 2=major]: " choice
  case "${choice:-1}" in
    1|minor) echo "minor" ;;
    2|major) echo "major" ;;
    *) die "Invalid bump choice '$choice'. Enter 1, 2, minor, or major." ;;
  esac
}

# -- Version computation -------------------------------------------------------

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
    log "CLI bump: $CLI_BUMP -> $(bold "$NEXT_CLI_VERSION")"
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
    log "App bump: $APP_BUMP -> $(bold "$NEXT_APP_TAG")"
  fi
}

# -- Preconditions -------------------------------------------------------------

ensure_prerequisites() {
  log_step "1/5" "Checking prerequisites"

  for tool in git node npm pnpm; do
    if command_exists "$tool"; then
      log_ok "$tool"
    else
      die "$tool is required but not found in PATH."
    fi
  done

  CURRENT_BRANCH="$(git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD)"
  if [[ "$CURRENT_BRANCH" == "main" ]]; then
    log_ok "On branch $(bold "main")"
  else
    die "Release must run from main branch (current: $CURRENT_BRANCH)."
  fi

  if [[ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]]; then
    die "Working tree is not clean. Commit or stash changes before releasing."
  fi
  log_ok "Working tree is clean"

  log "Fetching tags from origin..."
  run_cmd git -C "$ROOT_DIR" fetch --tags
  log_ok "Tags fetched"
}

ensure_npm_auth() {
  [[ "$CLI_SELECTED" -eq 1 ]] || return 0
  [[ "$DRY_RUN" -eq 1 ]] && return 0

  log "Checking npm authentication..."
  local npm_user
  if npm_user="$(npm whoami 2>/dev/null)"; then
    log_ok "Logged in to npm as $(bold "$npm_user")"
  else
    echo
    echo "  $(yellow "npm auth required") — you are not logged in to npm."
    echo "  $(dim "This is needed to publish the CLI package.")"
    echo
    if [[ "$AUTO_YES" -eq 1 ]]; then
      die "npm login required but --yes was specified. Run 'npm login' first."
    fi
    local answer
    read -r -p "$(bold "Run npm login now?") [Y/n]: " answer
    answer="$(echo "${answer:-y}" | tr '[:upper:]' '[:lower:]')"
    if [[ "$answer" == "y" || "$answer" == "yes" || -z "$answer" ]]; then
      npm login
      # Verify login succeeded
      if npm_user="$(npm whoami 2>/dev/null)"; then
        log_ok "Logged in to npm as $(bold "$npm_user")"
      else
        die "npm login failed. Cannot publish CLI."
      fi
    else
      die "npm login required to publish CLI. Run 'npm login' and try again."
    fi
  fi
}

ensure_app_tag_available() {
  [[ "$APP_SELECTED" -eq 1 ]] || return 0

  log "Checking tag $(bold "$NEXT_APP_TAG") is available..."
  if git -C "$ROOT_DIR" rev-parse "$NEXT_APP_TAG" >/dev/null 2>&1; then
    die "Tag $NEXT_APP_TAG already exists locally."
  fi

  if git -C "$ROOT_DIR" ls-remote --tags origin "refs/tags/$NEXT_APP_TAG" | grep -q .; then
    die "Tag $NEXT_APP_TAG already exists on origin."
  fi
  log_ok "Tag $(bold "$NEXT_APP_TAG") is available"
}

# -- Release plan --------------------------------------------------------------

print_plan() {
  echo
  echo "$(bold "=== Release Plan ===")"
  echo "  branch:   $(bold "$CURRENT_BRANCH")"
  if [[ "$CLI_SELECTED" -eq 1 ]]; then
    echo "  cli:      $(bold "$CURRENT_CLI_VERSION") -> $(green "$NEXT_CLI_VERSION") ($CLI_BUMP bump)"
  fi
  if [[ "$APP_SELECTED" -eq 1 ]]; then
    echo "  app tag:  $(bold "$CURRENT_APP_TAG") -> $(green "$NEXT_APP_TAG") ($APP_BUMP bump)"
  fi
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  mode:     $(yellow "DRY RUN") $(dim "(no changes will be made)")"
  fi
  echo

  if [[ "$CLI_SELECTED" -eq 1 ]]; then
    echo "  $(dim "Steps for CLI:")"
    echo "    $(dim "1.") Bump version in apps/cli/package.json"
    echo "    $(dim "2.") Install dependencies (pnpm install)"
    echo "    $(dim "3.") Build CLI (pnpm build)"
    echo "    $(dim "4.") Commit and push to main"
    echo "    $(dim "5.") Publish to npm"
  fi
  if [[ "$APP_SELECTED" -eq 1 ]]; then
    echo "  $(dim "Steps for App:")"
    echo "    $(dim "1.") Create annotated git tag $NEXT_APP_TAG"
    echo "    $(dim "2.") Push tag to origin (triggers GitHub Actions)"
  fi
  echo
}

# -- Release actions -----------------------------------------------------------

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

  log_step "3/5" "Building CLI release"

  log "Bumping CLI version to $(bold "$NEXT_CLI_VERSION")..."
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  $(yellow "[dry-run]") npm version --no-git-tag-version $NEXT_CLI_VERSION"
  else
    (cd "$ROOT_DIR/apps/cli" && npm version --no-git-tag-version "$NEXT_CLI_VERSION")
  fi
  log_ok "Version bumped in package.json"

  log "Installing dependencies..."
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  $(yellow "[dry-run]") pnpm install --frozen-lockfile"
  else
    pnpm -C "$ROOT_DIR/apps" install --frozen-lockfile
  fi
  log_ok "Dependencies installed"

  log "Building CLI..."
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  $(yellow "[dry-run]") pnpm -C apps/cli build"
  else
    pnpm -C "$ROOT_DIR/apps/cli" build
  fi
  log_ok "CLI built"
}

commit_and_push_if_needed() {
  local msg
  msg="$(commit_message)"
  [[ -n "$msg" ]] || return 0

  log_step "4/5" "Committing and pushing"

  # App-only releases are tag-only and do not require a version commit.
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  $(yellow "[dry-run]") git add apps/cli/package.json"
    echo "  $(yellow "[dry-run]") git commit -m \"$msg\""
    echo "  $(yellow "[dry-run]") git push origin $CURRENT_BRANCH"
    return 0
  fi

  log "Staging apps/cli/package.json..."
  git -C "$ROOT_DIR" add apps/cli/package.json
  if git -C "$ROOT_DIR" diff --cached --quiet; then
    log_skip "No staged changes to commit"
    return 0
  fi

  log "Committing: $(dim "$msg")"
  git -C "$ROOT_DIR" commit -m "$msg"
  log_ok "Committed"

  log "Pushing to origin/$CURRENT_BRANCH..."
  git -C "$ROOT_DIR" push origin "$CURRENT_BRANCH"
  log_ok "Pushed"
}

create_and_push_app_tag() {
  [[ "$APP_SELECTED" -eq 1 ]] || return 0

  if [[ "$CLI_SELECTED" -eq 0 ]]; then
    log_step "3/5" "Creating app tag"
  else
    log_step "4/5" "Creating app tag"
  fi

  log "Creating annotated tag $(bold "$NEXT_APP_TAG")..."
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  $(yellow "[dry-run]") git tag -a $NEXT_APP_TAG -m \"release: $NEXT_APP_TAG\""
    echo "  $(yellow "[dry-run]") git push origin $NEXT_APP_TAG"
    return 0
  fi

  git -C "$ROOT_DIR" tag -a "$NEXT_APP_TAG" -m "release: $NEXT_APP_TAG"
  log_ok "Tag created"

  log "Pushing tag to origin..."
  git -C "$ROOT_DIR" push origin "$NEXT_APP_TAG"
  log_ok "Tag pushed (GitHub Actions release workflow will trigger)"
}

publish_cli() {
  [[ "$CLI_SELECTED" -eq 1 ]] || return 0

  log_step "5/5" "Publishing CLI to npm"

  log "Running npm publish --access public..."
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "  $(yellow "[dry-run]") npm publish --access public"
    return 0
  fi
  (cd "$ROOT_DIR/apps/cli" && npm publish --access public)
  log_ok "Published to npm"
}

# -- Argument parsing ----------------------------------------------------------

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

# -- Main ----------------------------------------------------------------------

main() {
  parse_args "$@"
  validate_bump "$DEFAULT_BUMP"

  echo
  echo "$(bold "Kandev Release")"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "$(yellow "  DRY RUN MODE - no changes will be made")"
  fi

  ensure_prerequisites

  log_step "2/5" "Selecting target and version"

  if [[ -n "$TARGET" ]]; then
    parse_target "$TARGET"
    local selected=""
    [[ "$CLI_SELECTED" -eq 1 ]] && selected+="cli "
    [[ "$APP_SELECTED" -eq 1 ]] && selected+="app "
    log "Target: $(bold "$selected") $(dim "(from --target flag)")"
  else
    # Interactive mode should prompt for target and bump choices.
    DEFAULT_BUMP=""
    select_target_interactive
  fi

  ensure_npm_auth
  detect_current_cli_version
  detect_current_app_version
  compute_next_versions
  ensure_app_tag_available
  print_plan

  if ! confirm "Proceed with release?"; then
    echo
    echo "Release cancelled."
    exit 0
  fi

  apply_cli_release
  commit_and_push_if_needed
  create_and_push_app_tag
  publish_cli

  echo
  echo "$(green "$(bold "Release complete!")")"
  if [[ "$CLI_SELECTED" -eq 1 ]]; then
    echo "  $(green "cli:") published $NEXT_CLI_VERSION to npm"
  fi
  if [[ "$APP_SELECTED" -eq 1 ]]; then
    echo "  $(green "app:") pushed tag $NEXT_APP_TAG to origin"
  fi
  echo
}

main "$@"
