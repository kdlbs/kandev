#!/usr/bin/env bash
# Update the kdlbs/homebrew-kandev tap with a new formula for the given release.
#
# Downloads .sha256 files from the GitHub release to populate the formula,
# generates Formula/kandev.rb, and pushes (or opens a PR).
#
# Usage:
#   update-homebrew-tap.sh <version> <tag>
#
# Arguments:
#   version  SemVer string (e.g. 0.17.0)
#   tag      Git tag (e.g. v0.17.0)
#
# Environment:
#   HOMEBREW_TAP_REPO         Override tap repo (default: kdlbs/homebrew-kandev).
#   HOMEBREW_TAP_DEPLOY_KEY   Private SSH key (PEM) for the tap repo. When set,
#                             clone via SSH and push directly to main. This is
#                             the default CI path. Deploy keys cannot open PRs.
#   HOMEBREW_TAP_PUSH         When no deploy key is present, set to "1" to push
#                             directly via `gh`/HTTPS instead of opening a PR.
set -euo pipefail

VERSION="${1:?Usage: $0 <version> <tag>}"
TAG="${2:?Usage: $0 <version> <tag>}"
TAP_REPO="${HOMEBREW_TAP_REPO:-kdlbs/homebrew-kandev}"
PUSH_DIRECT="${HOMEBREW_TAP_PUSH:-0}"

bold()  { printf '\033[1m%s\033[0m' "$*"; }
green() { printf '\033[32m%s\033[0m' "$*"; }
red()   { printf '\033[31m%s\033[0m' "$*"; }

log()    { echo "  >> $*"; }
log_ok() { echo "  $(green "ok") $*"; }

die() {
  echo "$(red "Error:") $*" >&2
  exit 1
}

# -- Determine clone strategy: deploy key (SSH) or gh (HTTPS) -----------------

CLONE_METHOD="gh"
SSH_KEY_FILE=""
SSH_WRAPPER=""

cleanup_ssh() {
  [[ -n "${SSH_KEY_FILE:-}" && -f "$SSH_KEY_FILE" ]] && rm -f "$SSH_KEY_FILE"
  [[ -n "${SSH_WRAPPER:-}" && -f "$SSH_WRAPPER" ]] && rm -f "$SSH_WRAPPER"
}
trap cleanup_ssh EXIT

if [[ -n "${HOMEBREW_TAP_DEPLOY_KEY:-}" ]]; then
  CLONE_METHOD="ssh"
  # Materialize the deploy key + a custom GIT_SSH_COMMAND so it doesn't pollute ~/.ssh
  SSH_KEY_FILE="$(mktemp)"
  chmod 600 "$SSH_KEY_FILE"
  printf '%s\n' "$HOMEBREW_TAP_DEPLOY_KEY" > "$SSH_KEY_FILE"
  # Ensure trailing newline (some secret stores strip it; openssh requires it)
  if [[ "$(tail -c1 "$SSH_KEY_FILE")" != "" ]]; then
    echo "" >> "$SSH_KEY_FILE"
  fi
  export GIT_SSH_COMMAND="ssh -i $SSH_KEY_FILE -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
  log "Using SSH deploy key for tap auth"
fi

# -- Fetch sha256 values from GitHub release ----------------------------------

fetch_sha256() {
  local platform="$1"
  local sha_file="kandev-${platform}.tar.gz.sha256"
  local content
  content="$(gh release download "$TAG" --pattern "$sha_file" --dir /tmp 2>/dev/null && cat "/tmp/$sha_file")" || {
    die "Failed to download $sha_file from release $TAG."
  }
  # sha256 files contain: <hash>  <filename> — extract just the hash
  echo "$content" | awk '{print $1}'
}

log "Fetching sha256 values from GitHub release $TAG..."
SHA_MACOS_ARM64="$(fetch_sha256 macos-arm64)"
SHA_MACOS_X64="$(fetch_sha256 macos-x64)"
SHA_LINUX_ARM64="$(fetch_sha256 linux-arm64)"
SHA_LINUX_X64="$(fetch_sha256 linux-x64)"
log_ok "sha256 values retrieved"

# -- Clone tap repo -----------------------------------------------------------

WORK_DIR="$(mktemp -d)"
TAP_DIR="$WORK_DIR/tap"

log "Cloning $TAP_REPO..."
if [[ "$CLONE_METHOD" == "ssh" ]]; then
  git clone "git@github.com:${TAP_REPO}.git" "$TAP_DIR"
else
  gh repo clone "$TAP_REPO" "$TAP_DIR"
fi
log_ok "Tap cloned to $TAP_DIR"

# -- Generate formula ---------------------------------------------------------

FORMULA_PATH="$TAP_DIR/Formula/kandev.rb"
mkdir -p "$(dirname "$FORMULA_PATH")"

GITHUB_BASE="https://github.com/kdlbs/kandev/releases/download/${TAG}"

cat > "$FORMULA_PATH" <<EOF
class Kandev < Formula
  desc "Manage tasks, orchestrate agents, review changes, and ship value"
  homepage "https://github.com/kdlbs/kandev"
  license "AGPL-3.0-only"
  version "$VERSION"

  # Node is required: the CLI launcher and Next.js standalone server both need it.
  depends_on "node"

  on_macos do
    if Hardware::CPU.arm?
      url "${GITHUB_BASE}/kandev-macos-arm64.tar.gz"
      sha256 "$SHA_MACOS_ARM64"
    else
      url "${GITHUB_BASE}/kandev-macos-x64.tar.gz"
      sha256 "$SHA_MACOS_X64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${GITHUB_BASE}/kandev-linux-arm64.tar.gz"
      sha256 "$SHA_LINUX_ARM64"
    else
      url "${GITHUB_BASE}/kandev-linux-x64.tar.gz"
      sha256 "$SHA_LINUX_X64"
    end
  end

  def install
    libexec.install Dir["*"]
    # cli/bin/cli.js has #!/usr/bin/env node shebang; write_env_script sets
    # KANDEV_BUNDLE_DIR so the CLI launcher finds bin/ and web/ in the Cellar.
    # KANDEV_VERSION is read by run.ts at startup so the version is shown in
    # logs (otherwise the launcher prints "release: (env)").
    bin.write_env_script libexec/"cli/bin/cli.js",
      KANDEV_BUNDLE_DIR: libexec.to_s,
      KANDEV_VERSION:    version.to_s
  end

  test do
    assert_match "kandev launcher", shell_output("#{bin}/kandev --help")
  end
end
EOF

log_ok "Formula written to Formula/kandev.rb"

# -- Commit and push (or open PR) ---------------------------------------------

cd "$TAP_DIR"
git config user.email "release-bot@kandev"
git config user.name "kandev release bot"

git add Formula/kandev.rb

if git diff --cached --quiet; then
  log "Formula unchanged — nothing to commit"
  rm -rf "$WORK_DIR"
  exit 0
fi

git commit -m "kandev $VERSION"

# Deploy keys can push but cannot open PRs (PR creation requires user/app auth).
# So when using SSH (deploy key path) or HOMEBREW_TAP_PUSH=1, push directly.
if [[ "$CLONE_METHOD" == "ssh" || "$PUSH_DIRECT" == "1" ]]; then
  git push origin HEAD:main
  log_ok "Pushed directly to main in $TAP_REPO"
else
  BRANCH="update-kandev-$VERSION"
  git branch -m "$BRANCH"
  git push -u origin "$BRANCH"
  gh pr create \
    --repo "$TAP_REPO" \
    --title "kandev $VERSION" \
    --body "Updates formula to [kandev $VERSION](https://github.com/kdlbs/kandev/releases/tag/$TAG)." \
    --head "$BRANCH" \
    --base main
  log_ok "PR opened in $TAP_REPO"
fi

rm -rf "$WORK_DIR"
echo "$(green "$(bold "Homebrew tap updated to $VERSION!")")"
