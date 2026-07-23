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

count_matches() {
  local mode=$1
  local pattern=$2
  local output
  local rg_args=(--count-matches)
  if [[ "$mode" == "fixed" ]]; then
    rg_args=(--fixed-strings --count-matches)
  elif [[ "$mode" != "regex" ]]; then
    fail "Unsupported match mode: $mode"
  fi
  output="$(rg "${rg_args[@]}" -- "$pattern" "$WORKFLOW" || true)"
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

count_occurrences() {
  local pattern=$1
  count_matches fixed "$pattern"
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

run_id_env_count="$(count_occurrences 'GITHUB_RUN_ID: ${{ github.run_id }}')"
run_attempt_env_count="$(count_occurrences 'GITHUB_RUN_ATTEMPT: ${{ github.run_attempt }}')"
if [[ "$run_id_env_count" -ne 2 || "$run_attempt_env_count" -ne 2 ]]; then
  fail "Dedicated terminal posting passes immutable workflow run identity"
fi
pass "Dedicated terminal posting passes immutable workflow run identity"

trusted_environment_count="$(count_occurrences 'environment: opencode-review-trusted')"
if [[ "$trusted_environment_count" -ne 1 ]]; then
  fail "Same-repository trusted review uses the protected environment"
fi
if grep -q 'OPENCODE_REVIEW_APP_PRIVATE_KEY' "$WORKFLOW" || ! grep -q 'OPENCODE_REVIEW_ENV_APP_PRIVATE_KEY' "$WORKFLOW"; then
  fail "Trusted review uses only the environment-scoped App private key"
fi
pass "Same-repository trusted review isolates App credentials in the protected environment"

for harness_file in \
  .agents/agents/pr-poller.md \
  .codex/agents/pr-poller.toml \
  .cursor/agents/pr-poller.md \
  .opencode/agents/pr-poller.md \
  .agents/skills/pr-fixup/SKILL.md \
  .agents/skills/planner-orchestration/SKILL.md; do
  if ! grep -q 'OpenCode App.*trusted_producer=true\|trusted_producer=true.*OpenCode App\|OpenCode producer.*trusted_producer=true' "$ROOT_DIR/$harness_file"; then
    fail "Harness requires trusted producer only for the dedicated OpenCode App"
  fi
done
pass "Harness preserves generic selected-reviewer qualification without App provenance"

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

artifact_upload_count="$(count_matches regex 'uses: actions/upload-artifact@(v4([[:space:]]|$)|[a-f0-9]{40}[[:space:]]+# v4([[:space:]]|$))')"
if [[ "$artifact_upload_count" != "2" ]]; then
  fail "OpenCode review upload-artifact is pinned to v4 in both workflow paths"
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

if rg -q '^  pull_request:' "$WORKFLOW"; then
  fail "OpenCode review runs only from pull_request_target base workflow"
fi
pass "OpenCode review runs only from pull_request_target base workflow"

app_token_action='actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3.2.0'
if [[ "$(count_occurrences "$app_token_action")" != "1" ]]; then
  fail "Only same-repository trusted review mints a pinned dedicated App token"
fi
pass "Only same-repository trusted review mints a pinned dedicated App token"

if [[ "$(count_occurrences 'permission-pull-requests: write')" != "1" ]]; then
  fail "Dedicated App tokens explicitly request pull-request write permission"
fi
pass "Dedicated App tokens explicitly request pull-request write permission"

if [[ "$(count_occurrences 'GH_TOKEN: ${{ steps.app-token.outputs.token }}')" != "1" ]]; then
  fail "Only dedicated App tokens are supplied to terminal posting"
fi
pass "Only dedicated App tokens are supplied to terminal posting"

if [[ "$(count_occurrences 'GH_TOKEN: ${{ github.token }}')" != "1" ]]; then
  fail "Only advisory fork posting uses github.token"
fi
pass "Only advisory fork posting uses github.token"

if [[ "$(count_occurrences 'ref: ${{ github.event.pull_request.head.sha }}')" != "2" ]] || [[ "$(count_occurrences 'git rev-parse HEAD')" != "2" ]]; then
  fail "Both review paths use and assert immutable PR head SHA checkouts"
fi
pass "Both review paths use and assert immutable PR head SHA checkouts"

if [[ "$(count_occurrences 'RANGE="$BASE_SHA...$HEAD_SHA"')" != "2" ]] || rg -q 'incremental|BEFORE|AFTER' "$WORKFLOW"; then
  fail "Both review paths always review the full base-to-head PR range"
fi
pass "Both review paths always review the full base-to-head PR range"

if [[ "$(count_matches regex '^[[:space:]]+pull-requests: read$')" != "1" ]] || [[ "$(count_matches regex '^[[:space:]]+pull-requests: write$')" != "1" ]]; then
  fail "Same-repository token is read-only and advisory fork publishing is the only write job"
fi
pass "Same-repository token is read-only and advisory fork publishing is the only write job"

model_steps="$(awk '/- name: Run OpenCode review/{in_run=1} /- name: Create dedicated OpenCode review App token/{in_run=0} in_run' "$WORKFLOW")"
if rg -q '(OPENCODE_REVIEW_APP_PRIVATE_KEY|steps\.app-token\.outputs\.token)' <<<"$model_steps"; then
  fail "OpenCode model step must not receive dedicated App credentials"
fi
pass "OpenCode model step does not receive dedicated App credentials"

if ! rg -A 10 '^  strip-safe-to-review:' "$WORKFLOW" | rg -q 'issues: write'; then
  fail "Fork safe-to-review cleanup has issues write permission"
fi
pass "Fork safe-to-review cleanup has issues write permission"

if [[ "$(count_occurrences "$app_token_action")" != "1" ]]; then
  fail "Only same-repository review mints the dedicated App token"
fi
pass "Only same-repository review mints the dedicated App token"
