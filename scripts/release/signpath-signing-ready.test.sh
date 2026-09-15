#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/release/signpath-signing-ready.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

required=(
  SIGNPATH_API_TOKEN
  SIGNPATH_ORGANIZATION_ID
  SIGNPATH_PROJECT_SLUG
  SIGNPATH_SIGNING_POLICY_SLUG
)

run_with_complete_inputs() {
  env \
    SIGNPATH_API_TOKEN=test-token \
    SIGNPATH_ORGANIZATION_ID=test-org \
    SIGNPATH_PROJECT_SLUG=test-project \
    SIGNPATH_SIGNING_POLICY_SLUG=release-signing \
    "$@" bash "$SCRIPT"
}

run_with_complete_inputs >"$TMP_DIR/out" 2>"$TMP_DIR/err" ||
  fail "complete SignPath inputs were rejected"
grep -q "SignPath signing inputs complete." "$TMP_DIR/out" ||
  fail "complete SignPath inputs were not confirmed on stdout"

# Every input is load-bearing on its own: the signing action takes all four and
# fails the release when handed a blank one, so a partial configuration has to
# read as "not configured" rather than as "configured".
for name in "${required[@]}"; do
  if run_with_complete_inputs "$name=" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
    fail "an empty $name was accepted as complete SignPath configuration"
  fi
  grep -q "$name" "$TMP_DIR/err" ||
    fail "the incomplete-input error did not name $name"
  grep -q "SignPath signing inputs incomplete" "$TMP_DIR/err" ||
    fail "the incomplete-input error was not actionable for $name"
done

# A variable set to whitespace is the same misconfiguration as an unset one,
# and the action cannot tell them apart either.
for name in "${required[@]}"; do
  if run_with_complete_inputs "$name=   " >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
    fail "a whitespace-only $name was accepted as complete SignPath configuration"
  fi
  grep -q "$name" "$TMP_DIR/err" ||
    fail "the whitespace-input error did not name $name"
done

if env -u SIGNPATH_API_TOKEN -u SIGNPATH_ORGANIZATION_ID \
  -u SIGNPATH_PROJECT_SLUG -u SIGNPATH_SIGNING_POLICY_SLUG \
  bash "$SCRIPT" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "an entirely unconfigured environment was accepted"
fi
for name in "${required[@]}"; do
  grep -q "$name" "$TMP_DIR/err" ||
    fail "the unconfigured-environment error did not name $name"
done

# A test-signing policy signs with a certificate Windows does not trust, so it
# is only acceptable on a run that publishes nothing. On a publishing run the
# guard refuses it with its own exit code, so the workflow can warn rather
# than merely notice, and the bundle ships unsigned as it does today.
run_with_policy() {
  local purpose="$1" slug="$2"
  env \
    SIGNPATH_API_TOKEN=test-token \
    SIGNPATH_ORGANIZATION_ID=test-org \
    SIGNPATH_PROJECT_SLUG=test-project \
    SIGNPATH_SIGNING_POLICY_SLUG="$slug" \
    bash "$SCRIPT" $purpose
}

for slug in test-signing test test-2026; do
  status=0
  run_with_policy publish "$slug" >"$TMP_DIR/out" 2>"$TMP_DIR/err" || status=$?
  [ "$status" -ne 0 ] || fail "a publishing run accepted the test policy $slug"
  [ "$status" -eq 2 ] || fail "refusing $slug on a publishing run exited with $status, not 2"
  grep -q "$slug" "$TMP_DIR/err" || fail "the refusal did not name the policy $slug"
  grep -q "test-signing policy" "$TMP_DIR/err" || fail "the refusal did not say why $slug was rejected"
  grep -q "refusing" "$TMP_DIR/err" || fail "the refusal for $slug was not explicit"
done

# The default purpose is the safe one: omitting it must behave like publish.
if run_with_policy "" test-signing >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "omitting the purpose accepted a test policy"
fi

run_with_policy validate test-signing >"$TMP_DIR/out" 2>"$TMP_DIR/err" ||
  fail "a validation run rejected the test policy"
grep -q "SignPath signing inputs complete." "$TMP_DIR/out" ||
  fail "a validation run with the test policy was not confirmed"

for slug in release-signing latest-signing contest-signing; do
  run_with_policy publish "$slug" >"$TMP_DIR/out" 2>"$TMP_DIR/err" ||
    fail "a publishing run rejected the non-test policy $slug"
done

if run_with_policy deploy release-signing >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
  fail "an unknown purpose was accepted"
fi
grep -q "Unsupported run purpose: deploy" "$TMP_DIR/err" ||
  fail "the unknown-purpose error did not identify deploy"
grep -q "publish, validate, nightly" "$TMP_DIR/err" ||
  fail "the unknown-purpose error did not list the accepted values"

# Incomplete inputs stay a status-1 refusal on either purpose.
status=0
env -u SIGNPATH_API_TOKEN SIGNPATH_ORGANIZATION_ID=o SIGNPATH_PROJECT_SLUG=p \
  SIGNPATH_SIGNING_POLICY_SLUG=test-signing bash "$SCRIPT" validate \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err" || status=$?
[ "$status" -eq 1 ] || fail "incomplete inputs on a validation run exited with $status, not 1"

# Nightlies run unattended and the release policy needs a manual approval per
# request, so the nightly channel never submits a signing request at all. The
# guard says so with its own status before it even looks at the inputs.
status=0
run_with_policy nightly release-signing >"$TMP_DIR/out" 2>"$TMP_DIR/err" || status=$?
[ "$status" -eq 4 ] || fail "the nightly channel exited with $status, not 4"
grep -q "Nightly channel" "$TMP_DIR/err" || fail "the nightly skip did not name the channel"
grep -q "skipped" "$TMP_DIR/err" || fail "the nightly skip was not explicit"

status=0
env -u SIGNPATH_API_TOKEN SIGNPATH_ORGANIZATION_ID=o SIGNPATH_PROJECT_SLUG=p \
  SIGNPATH_SIGNING_POLICY_SLUG=release-signing bash "$SCRIPT" nightly \
  >"$TMP_DIR/out" 2>"$TMP_DIR/err" || status=$?
[ "$status" -eq 4 ] || fail "the nightly skip must not depend on the inputs (exited $status)"

echo "PASS: SignPath signing readiness requires all four inputs, confines test policies to validation runs and skips nightlies"
