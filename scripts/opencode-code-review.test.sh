#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKFLOW="$ROOT_DIR/.github/workflows/opencode-code-review.yml"

pass() {
  printf 'ok - %s\n' "$1"
}

fail() {
  printf 'not ok - %s\n' "$1" >&2
  exit 1
}

count_occurrences() {
  local pattern=$1
  local output
  output="$(rg --fixed-strings --count-matches -- "$pattern" "$WORKFLOW" || true)"
  if [[ -z "$output" ]]; then
    printf '0\n'
    return
  fi
  if [[ "$output" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "$output"
    return
  fi
  awk -F: '{ print $2 }' <<<"$output"
}

count_regex_occurrences() {
  local pattern=$1
  local output
  output="$(rg --count-matches -- "$pattern" "$WORKFLOW" || true)"
  if [[ -z "$output" ]]; then
    printf '0\n'
    return
  fi
  if [[ "$output" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "$output"
    return
  fi
  awk -F: '{ print $2 }' <<<"$output"
}

tmp_review_reference_count="$(count_occurrences '/tmp/opencode-review')"
if [[ "$tmp_review_reference_count" != "0" ]]; then
  fail "OpenCode review artifacts are not written under /tmp"
fi
pass "OpenCode review artifacts are not written under /tmp"

relative_guidelines_count="$(count_occurrences '--file "$REVIEW_DIR/guidelines.md"')"
if [[ "$relative_guidelines_count" != "2" ]]; then
  fail "OpenCode guidelines are passed as relative workspace files in both workflow paths"
fi
pass "OpenCode guidelines are passed as relative workspace files in both workflow paths"

relative_patch_count="$(count_occurrences '--file "$REVIEW_DIR/review.patch"')"
if [[ "$relative_patch_count" != "2" ]]; then
  fail "OpenCode patches are passed as relative workspace files in both workflow paths"
fi
pass "OpenCode patches are passed as relative workspace files in both workflow paths"

fork_base_fetch_count="$(count_occurrences 'git fetch --no-tags --prune origin "$BASE_SHA"')"
if [[ "$fork_base_fetch_count" != "1" ]]; then
  fail "Fork review fetches the trusted base commit before building the diff"
fi
pass "Fork review fetches the trusted base commit before building the diff"

shallow_base_fetch_count="$(count_occurrences 'git fetch --no-tags --prune --depth=1 origin "$BASE_SHA"')"
if [[ "$shallow_base_fetch_count" != "0" ]]; then
  fail "Fork review does not shallow-fetch the base commit before a three-dot diff"
fi
pass "Fork review does not shallow-fetch the base commit before a three-dot diff"

recreate_config_count="$(count_occurrences 'rm -rf .opencode')"
if [[ "$recreate_config_count" != "2" ]]; then
  fail "OpenCode project config is recreated before writing the review agent"
fi
pass "OpenCode project config is recreated before writing the review agent"

allowed_tools_prompt_count="$(count_occurrences 'Use only read/search tools made available by OpenCode, such as glob, grep, and read.')"
if [[ "$allowed_tools_prompt_count" != "2" ]]; then
  fail "OpenCode review prompts explicitly limit the model to read/search tools"
fi
pass "OpenCode review prompts explicitly limit the model to read/search tools"

deny_bash_prompt_count="$(count_occurrences 'Never call bash, shell, edit, patch, task, todo, fetch, or web tools.')"
if [[ "$deny_bash_prompt_count" != "2" ]]; then
  fail "OpenCode review prompts explicitly forbid bash and mutation tools"
fi
pass "OpenCode review prompts explicitly forbid bash and mutation tools"

explicit_model_env_count="$(count_occurrences 'OPENCODE_MODEL: ${{ env.OPENCODE_MODEL }}')"
if [[ "$explicit_model_env_count" != "2" ]]; then
  fail "OpenCode posting steps explicitly pass OPENCODE_MODEL"
fi
pass "OpenCode posting steps explicitly pass OPENCODE_MODEL"

patch_capability_count="$(count_occurrences 'post-findings --help | grep -q -- "--patch"')"
if [[ "$patch_capability_count" != "2" ]]; then
  fail "OpenCode posting steps check trusted parser support before passing review.patch"
fi
pass "OpenCode posting steps check trusted parser support before passing review.patch"

explicit_patch_arg_count="$(count_occurrences 'post_args+=(--patch .opencode-review/review.patch)')"
if [[ "$explicit_patch_arg_count" != "2" ]]; then
  fail "OpenCode posting steps explicitly pass review.patch for inline filtering"
fi
pass "OpenCode posting steps explicitly pass review.patch for inline filtering"

trusted_script_count="$(count_occurrences 'git show "$BASE_SHA:scripts/opencode-code-review" > .opencode-review/opencode-code-review')"
if [[ "$trusted_script_count" != "2" ]]; then
  fail "OpenCode review executes the parser script from the trusted base commit in both workflow paths"
fi
pass "OpenCode review executes the parser script from the trusted base commit in both workflow paths"

artifact_upload_count="$(count_regex_occurrences 'uses: actions/upload-artifact@(v4|[a-f0-9]{40} # v4)')"
if [[ "$artifact_upload_count" != "2" ]]; then
  fail "OpenCode review artifacts are uploaded from both workflow paths"
fi
pass "OpenCode review artifacts are uploaded from both workflow paths"

invalid_artifact_upload_refs="$(
  rg --line-number 'uses: actions/upload-artifact@' "$WORKFLOW" |
    rg --invert-match 'uses: actions/upload-artifact@[0-9a-f]{40} # v[0-9]+(?:\.[0-9]+\.[0-9]+)?$' || true
)"
if [[ -n "$invalid_artifact_upload_refs" ]]; then
  fail "OpenCode review artifact uploads use immutable action refs"
fi
pass "OpenCode review artifact uploads use immutable action refs"
