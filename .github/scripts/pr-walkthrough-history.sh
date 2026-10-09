#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "$*" >&2
  exit 1
}

require_sha() {
  local name="$1"
  local value="$2"
  [[ "$value" =~ ^[0-9a-f]{40}$ ]] || fail "$name must be a full lowercase 40-character SHA"
}

require_sha TRUSTED_SHA "${TRUSTED_SHA:-}"
require_sha HEAD_SHA "${HEAD_SHA:-}"
[[ "${PR_NUMBER:-}" =~ ^[1-9][0-9]*$ ]] || fail "PR_NUMBER must be a positive integer"

test "$(git rev-parse HEAD)" = "$TRUSTED_SHA" || fail "checkout does not match TRUSTED_SHA"

trusted_ref=refs/kandev/pr-walkthrough/trusted
head_ref=refs/kandev/pr-walkthrough/head
fetch_args=(fetch --no-tags --filter=blob:none --negotiation-tip="$TRUSTED_SHA")
if [ "$(git rev-parse --is-shallow-repository)" = true ]; then
  fetch_args+=(--unshallow)
fi

if ! git "${fetch_args[@]}" origin \
  "+${TRUSTED_SHA}:${trusted_ref}" \
  "+refs/pull/${PR_NUMBER}/head:${head_ref}"; then
  fail "targeted walkthrough history fetch failed"
fi

fetched_trusted="$(git rev-parse "${trusted_ref}^{commit}")" || fail "trusted history is unavailable"
fetched_head="$(git rev-parse "${head_ref}^{commit}")" || fail "PR-head history is unavailable"
test "$fetched_trusted" = "$TRUSTED_SHA" || fail "fetched trusted commit does not match TRUSTED_SHA"
test "$fetched_head" = "$HEAD_SHA" || fail "fetched PR head does not match HEAD_SHA"
test "$(git rev-parse HEAD)" = "$TRUSTED_SHA" || fail "history fetch changed the trusted checkout"

if ! git merge-base "$TRUSTED_SHA" "$HEAD_SHA" >/dev/null; then
  fail "trusted and PR-head histories have no merge base"
fi
