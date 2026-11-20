#!/usr/bin/env bash
set -euo pipefail

# Only trusted workflow files occupy the execution worktree.
case "${REVIEW_DIR:-}" in
  .opencode-review|.claude-review) ;;
  *) echo 'Invalid review output directory' >&2; exit 1 ;;
esac
[[ "${BASE_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || { echo 'Invalid BASE_SHA' >&2; exit 1; }
bash .github/scripts/pr-walkthrough-history.sh
git fetch --no-tags origin "$BASE_SHA"
test "$(git rev-parse HEAD)" = "$TRUSTED_SHA"

rm -rf "$REVIEW_DIR"
mkdir -p "$REVIEW_DIR"
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context \
  --repo "$GITHUB_WORKSPACE" \
  --base-sha "$BASE_SHA" \
  --head-sha "$HEAD_SHA" \
  --output-dir "$REVIEW_DIR/head-context"

RANGE="$BASE_SHA...$HEAD_SHA"
git diff --no-ext-diff --no-textconv --find-renames --find-copies --unified=80 "$RANGE" > "$REVIEW_DIR/review.patch"
git diff --no-ext-diff --no-textconv --name-only "$RANGE" > "$REVIEW_DIR/files.txt"
{
  printf 'Review mode: full-pr\nDiff range: %s\n' "$RANGE"
  printf 'Workflow definition: %s\n' "${GITHUB_WORKFLOW_REF:-}"
  printf 'Workflow SHA: %s\nCheckout SHA: %s\n' "$TRUSTED_SHA" "$(git rev-parse HEAD)"
  printf 'Prepared head context: %s/head-context/manifest.json\n\n' "$REVIEW_DIR"
  printf 'Changed files:\n'
  sed 's/^/- /' "$REVIEW_DIR/files.txt"
  printf '\nTrusted repository guidelines:\n'
  sed -n '1,220p' AGENTS.md
  printf '\nTrusted code review skill:\n'
  sed -n '1,220p' .agents/skills/code-review/SKILL.md
} > "$REVIEW_DIR/guidelines.md"
printf 'mode=full-pr\nrange=%s\n' "$RANGE" >> "$GITHUB_OUTPUT"
