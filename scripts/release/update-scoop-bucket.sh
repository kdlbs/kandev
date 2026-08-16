#!/usr/bin/env bash
# Update the kdlbs/scoop-kandev bucket with a Kandev Stable release.
#
# Usage:
#   update-scoop-bucket.sh <version> <tag>
#
# Environment:
#   SCOOP_SOURCE_REPO       Override the source repository (default:
#                           kdlbs/kandev).
#   SCOOP_BUCKET_REPO       Override the bucket repository (default:
#                           kdlbs/scoop-kandev).
#   SCOOP_BUCKET_DEPLOY_KEY Private SSH key for the bucket repository. When set,
#                           clone and push with pinned GitHub host keys.
#
# Without SCOOP_BUCKET_DEPLOY_KEY, the script uses the authenticated `gh` CLI
# to clone the bucket and pushes through the clone's configured remote. This is
# intended for local maintenance and black-box tests; CI preflights the deploy
# key before invoking this script.
set -euo pipefail

VERSION="${1:?Usage: $0 <version> <tag>}"
TAG="${2:?Usage: $0 <version> <tag>}"
SOURCE_REPO="${SCOOP_SOURCE_REPO:-kdlbs/kandev}"
BUCKET_REPO="${SCOOP_BUCKET_REPO:-kdlbs/scoop-kandev}"
ARCHIVE_NAME="kandev-windows-x64.tar.gz"
CHECKSUM_NAME="${ARCHIVE_NAME}.sha256"
WORK_DIR=""
BUCKET_DIR=""
SSH_KEY_FILE=""
KNOWN_HOSTS_FILE=""

log() {
  printf '  >> %s\n' "$*"
}

log_ok() {
  printf '  ok %s\n' "$*"
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  die "version must be a three-part SemVer value: $VERSION"
fi
if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  die "tag must be a v-prefixed three-part SemVer value: $TAG"
fi
if [[ "$TAG" != "v$VERSION" ]]; then
  die "tag must match version: expected v$VERSION, got $TAG"
fi

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/kandev-scoop.XXXXXX")"
BUCKET_DIR="$WORK_DIR/bucket"

cleanup() {
  local exit_code=$?
  trap - EXIT
  if [[ -n "$SSH_KEY_FILE" ]]; then
    rm -f -- "$SSH_KEY_FILE" || true
  fi
  if [[ -n "$KNOWN_HOSTS_FILE" ]]; then
    rm -f -- "$KNOWN_HOSTS_FILE" || true
  fi
  if [[ -n "$WORK_DIR" ]]; then
    rm -rf -- "$WORK_DIR" || true
  fi
  exit "$exit_code"
}
trap cleanup EXIT

# -- Configure the deploy-key clone path ------------------------------------

if [[ -n "${SCOOP_BUCKET_DEPLOY_KEY:-}" ]]; then
  SSH_KEY_FILE="$WORK_DIR/deploy-key"
  umask 077
  printf '%s\n' "$SCOOP_BUCKET_DEPLOY_KEY" > "$SSH_KEY_FILE"
  chmod 600 "$SSH_KEY_FILE"

  KNOWN_HOSTS_FILE="$WORK_DIR/known_hosts"
  cat > "$KNOWN_HOSTS_FILE" <<'EOF'
github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl
github.com ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEmKSENjQEezOmxkZMy7opKgwFB9nkt5YRrYMjNuG5N87uRgg6CLrbo5wAdT/y6v0mKV0U2w0WZ2YB/++Tpockg=
github.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCj7ndNxQowgcQnjshcLrqPEiiphnt+VTTvDP6mHBL9j1aNUkY4Ue1gvwnGLVlOhGeYrnZaMgRK6+PKCUXaDbC7qtbW8gIkhL7aGCsOr/C56SJMy/BCZfxd1nWzAOxSDPgVsmerOBYfNqltV9/hWCqBywINIR+5dIg6JTJ72pcEpEjcYgXkE2YEFXV1JHnsKgbLWNlhScqb2UmyRkQyytRLtL+38TGxkxCflmO+5Z8CSSNY7GidjMIZ7Q4zMjA2n1nGrlTDkzwDCsw+wqFPGQA179cnfGWOWRVruj16z6XyvxvjJwbz0wQZ75XK5tKSb7FNyeIEs4TT4jk+S4dhPeAUC5y+bDYirYgM4GC7uEnztnZyaVWQ7B381AK4Qdrwt51ZqExKbQpTUNn+EjqoTwvqNj4kqx5QUCI0ThS/YkOxJCXmPUWZbhjpCg56i+2aB6CmK2JGhn57K5mj0MNdBXA4/WnwH6XoPWJzK5Nyu2zB3nAZp+S5hpQs+p1vN1/wsjk=
EOF
  chmod 600 "$KNOWN_HOSTS_FILE"
  export GIT_SSH_COMMAND="ssh -i $SSH_KEY_FILE -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=$KNOWN_HOSTS_FILE -o LogLevel=ERROR"
  log "Using the Scoop bucket deploy key with pinned github.com host keys"
fi

# -- Fetch the checksum from the exact GitHub release ------------------------

log "Fetching $CHECKSUM_NAME from release $TAG..."
if ! gh release download "$TAG" \
  --repo "$SOURCE_REPO" \
  --pattern "$CHECKSUM_NAME" \
  --dir "$WORK_DIR"; then
  die "failed to download $CHECKSUM_NAME from release $TAG"
fi

CHECKSUM_PATH="$WORK_DIR/$CHECKSUM_NAME"
if [[ ! -f "$CHECKSUM_PATH" ]]; then
  die "release $TAG did not contain checksum file $CHECKSUM_NAME"
fi

CHECKSUM_CONTENT="$(<"$CHECKSUM_PATH")"
CHECKSUM_PATTERN='^[0-9a-f]{64}[[:space:]]+\*?kandev-windows-x64\.tar\.gz[[:space:]]*$'
if [[ ! "$CHECKSUM_CONTENT" =~ $CHECKSUM_PATTERN ]]; then
  die "invalid checksum for $ARCHIVE_NAME in release $TAG"
fi
SHA256="${CHECKSUM_CONTENT%%[[:space:]]*}"
log_ok "Checksum retrieved and validated"

# -- Clone the bucket --------------------------------------------------------

log "Cloning $BUCKET_REPO..."
if [[ -n "${SCOOP_BUCKET_DEPLOY_KEY:-}" ]]; then
  git clone "git@github.com:${BUCKET_REPO}.git" "$BUCKET_DIR"
else
  gh repo clone "$BUCKET_REPO" "$BUCKET_DIR"
fi
if [[ ! -d "$BUCKET_DIR/.git" ]]; then
  die "bucket clone did not produce a Git repository"
fi
git -C "$BUCKET_DIR" checkout main >/dev/null 2>&1 || die "bucket repository has no main branch"
log_ok "Bucket cloned"

# -- Update only the version-dependent manifest fields -----------------------

MANIFEST_PATH="$BUCKET_DIR/bucket/kandev.json"
if [[ ! -f "$MANIFEST_PATH" ]]; then
  die "bucket manifest is missing: bucket/kandev.json"
fi
if ! jq -e '
  (.version | type == "string") and
  (.architecture["64bit"].url | type == "string") and
  (.architecture["64bit"].hash | type == "string")
' "$MANIFEST_PATH" >/dev/null; then
  die "bucket manifest does not contain the expected 64-bit fields"
fi

RELEASE_URL="https://github.com/${SOURCE_REPO}/releases/download/${TAG}/${ARCHIVE_NAME}"
CURRENT_VERSION="$(jq -r '.version' "$MANIFEST_PATH")"
CURRENT_URL="$(jq -r '.architecture["64bit"].url' "$MANIFEST_PATH")"
CURRENT_HASH="$(jq -r '.architecture["64bit"].hash' "$MANIFEST_PATH")"

if [[ "$CURRENT_VERSION" == "$VERSION" && "$CURRENT_URL" == "$RELEASE_URL" && "$CURRENT_HASH" == "$SHA256" ]]; then
  log "Scoop manifest is already up to date; nothing to commit"
  exit 0
fi

UPDATED_MANIFEST="$WORK_DIR/kandev.json"
jq --indent 4 \
  --arg version "$VERSION" \
  --arg url "$RELEASE_URL" \
  --arg hash "$SHA256" \
  '.version = $version | .architecture["64bit"].url = $url | .architecture["64bit"].hash = $hash' \
  "$MANIFEST_PATH" > "$UPDATED_MANIFEST"
mv "$UPDATED_MANIFEST" "$MANIFEST_PATH"

cd "$BUCKET_DIR"
git config user.email "release-bot@kandev"
git config user.name "kandev release bot"
git add -- bucket/kandev.json

if git diff --cached --quiet; then
  log "Scoop manifest is already up to date; nothing to commit"
  exit 0
fi

git commit -m "kandev $VERSION"
git push origin HEAD:main
log_ok "Scoop bucket updated to $VERSION"
