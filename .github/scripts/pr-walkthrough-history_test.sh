#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HISTORY_HELPER="$REPO_ROOT/.github/scripts/pr-walkthrough-history.sh"
TEMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEMP_ROOT"' EXIT

export GIT_AUTHOR_NAME="Walkthrough Test"
export GIT_AUTHOR_EMAIL="walkthrough@example.invalid"
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME"
export GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"

fail() {
  echo "$*" >&2
  exit 1
}

REMOTE="$TEMP_ROOT/remote.git"
SOURCE="$TEMP_ROOT/source"
git init --bare --initial-branch=main "$REMOTE" >/dev/null
git --git-dir="$REMOTE" config uploadpack.allowFilter true
git init --initial-branch=main "$SOURCE" >/dev/null
git -C "$SOURCE" config user.name "$GIT_AUTHOR_NAME"
git -C "$SOURCE" config user.email "$GIT_AUTHOR_EMAIL"

printf 'base\n' > "$SOURCE/shared.txt"
git -C "$SOURCE" add shared.txt
git -C "$SOURCE" commit -m base >/dev/null
BASE_SHA="$(git -C "$SOURCE" rev-parse HEAD)"
git -C "$SOURCE" remote add origin "$REMOTE"

git -C "$SOURCE" switch -c feature "$BASE_SHA" >/dev/null
printf 'head\n' > "$SOURCE/pr.txt"
git -C "$SOURCE" add pr.txt
git -C "$SOURCE" commit -m head >/dev/null
PR_HEAD_SHA="$(git -C "$SOURCE" rev-parse HEAD)"
git -C "$SOURCE" push origin "$PR_HEAD_SHA:refs/pull/73/head" >/dev/null

git -C "$SOURCE" switch main >/dev/null
printf 'trusted\n' > "$SOURCE/trusted.txt"
git -C "$SOURCE" add trusted.txt
git -C "$SOURCE" commit -m trusted >/dev/null
TRUSTED_SHA="$(git -C "$SOURCE" rev-parse HEAD)"
git -C "$SOURCE" push origin main >/dev/null

git -C "$SOURCE" switch -c unrelated >/dev/null
printf 'unrelated\n' > "$SOURCE/unrelated.txt"
git -C "$SOURCE" add unrelated.txt
git -C "$SOURCE" commit -m unrelated >/dev/null
git -C "$SOURCE" tag unrelated-tag
git -C "$SOURCE" push origin unrelated >/dev/null
git -C "$SOURCE" push origin refs/tags/unrelated-tag >/dev/null

SHALLOW="$TEMP_ROOT/shallow"
git clone --depth=1 --branch main "file://$REMOTE" "$SHALLOW" >/dev/null 2>&1
if ! (cd "$SHALLOW" && TRUSTED_SHA="$TRUSTED_SHA" HEAD_SHA="$PR_HEAD_SHA" PR_NUMBER=73 \
  bash "$HISTORY_HELPER"); then
  fail "targeted history helper failed on a shallow trusted checkout"
fi

test "$(git -C "$SHALLOW" rev-parse HEAD)" = "$TRUSTED_SHA" || fail "helper changed trusted checkout"
test "$(git -C "$SHALLOW" rev-parse refs/kandev/pr-walkthrough/trusted)" = "$TRUSTED_SHA" || fail "trusted ref mismatch"
test "$(git -C "$SHALLOW" rev-parse refs/kandev/pr-walkthrough/head)" = "$PR_HEAD_SHA" || fail "PR head ref mismatch"
test "$(git -C "$SHALLOW" merge-base "$TRUSTED_SHA" "$PR_HEAD_SHA")" = "$BASE_SHA" || fail "merge base mismatch"
test "$(git -C "$SHALLOW" rev-parse --is-shallow-repository)" = false || fail "shallow boundaries remain"
test "$(git -C "$SHALLOW" show "$PR_HEAD_SHA:pr.txt")" = head || fail "PR-head blobs cannot be read"
git -C "$SHALLOW" diff --find-renames --find-copies "$TRUSTED_SHA...$PR_HEAD_SHA" >/dev/null || fail "trusted-to-head diff failed"
test -z "$(git -C "$SHALLOW" for-each-ref --format='%(refname)' refs/remotes/origin/unrelated)" || fail "unrelated branch was fetched"
test -z "$(git -C "$SHALLOW" for-each-ref --format='%(refname)' refs/tags/unrelated-tag)" || fail "unrelated tag was fetched"

COMPLETE="$TEMP_ROOT/complete"
git clone --branch main "file://$REMOTE" "$COMPLETE" >/dev/null 2>&1
if ! (cd "$COMPLETE" && TRUSTED_SHA="$TRUSTED_SHA" HEAD_SHA="$PR_HEAD_SHA" PR_NUMBER=73 \
  bash "$HISTORY_HELPER"); then
  fail "targeted history helper failed on a complete checkout"
fi
test "$(git -C "$COMPLETE" rev-parse HEAD)" = "$TRUSTED_SHA" || fail "complete checkout moved"
test "$(git -C "$COMPLETE" merge-base "$TRUSTED_SHA" "$PR_HEAD_SHA")" = "$BASE_SHA" || fail "complete checkout merge base mismatch"

if (cd "$SHALLOW" && TRUSTED_SHA="$TRUSTED_SHA" HEAD_SHA="$BASE_SHA" PR_NUMBER=73 bash "$HISTORY_HELPER") >/dev/null 2>&1; then
  fail "helper accepted an event SHA different from the PR-head ref"
fi

ORPHAN="$TEMP_ROOT/orphan"
git init --initial-branch=main "$ORPHAN" >/dev/null
git -C "$ORPHAN" config user.name "$GIT_AUTHOR_NAME"
git -C "$ORPHAN" config user.email "$GIT_AUTHOR_EMAIL"
printf 'orphan\n' > "$ORPHAN/orphan.txt"
git -C "$ORPHAN" add orphan.txt
git -C "$ORPHAN" commit -m orphan >/dev/null
ORPHAN_SHA="$(git -C "$ORPHAN" rev-parse HEAD)"
git -C "$ORPHAN" push --force "$REMOTE" "$ORPHAN_SHA:refs/pull/73/head" >/dev/null
if (cd "$SHALLOW" && TRUSTED_SHA="$TRUSTED_SHA" HEAD_SHA="$ORPHAN_SHA" PR_NUMBER=73 bash "$HISTORY_HELPER") >/dev/null 2>&1; then
  fail "helper accepted PR history with no merge base"
fi

echo "PR walkthrough history tests passed."
