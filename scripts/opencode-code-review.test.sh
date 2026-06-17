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
  output="$(rg --fixed-strings --count-matches "$pattern" "$WORKFLOW" || true)"
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

model_reference_count="$(count_occurrences 'OpenCode model: `{model}`')"
if [[ "$model_reference_count" != "2" ]]; then
  fail "OpenCode no-suggestions comments include the model in both workflow paths"
fi
pass "OpenCode no-suggestions comments include the model in both workflow paths"

paginated_comment_lookup_count="$(count_occurrences '"gh", "api", "--paginate",')"
if [[ "$paginated_comment_lookup_count" != "2" ]]; then
  fail "OpenCode no-suggestions comment lookup paginates in both workflow paths"
fi
pass "OpenCode no-suggestions comment lookup paginates in both workflow paths"
