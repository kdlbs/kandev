#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/run-quiet"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

pass() {
  printf 'ok - %s\n' "$1"
}

fail() {
  printf 'not ok - %s\n' "$1" >&2
  exit 1
}

cat >"$TMP_DIR/success-with-go-failure" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' '{"Time":"2026-07-15T19:00:00Z","Action":"fail","Package":"example.com/project/pkg","Test":"TestLeaky"}'
printf '%s\n' '--- FAIL: TestLeaky (0.00s)'
printf '%s\n' 'goleak: Errors on successful test run: found unexpected goroutines'
EOF
chmod +x "$TMP_DIR/success-with-go-failure"

summary_output="$(KANDEV_RUN_QUIET_DIR="$TMP_DIR" "$SCRIPT" e2e --summary -- bash -c 'printf "\033[32m  12 passed (2.0s)\033[0m\n  3 skipped\n  1 flaky\n"')"
grep -q '^exit=0 log=' <<<"$summary_output" || fail "E2E summary preserves exit and log path"
for counts in '12 passed' '3 skipped' '1 flaky'; do
  grep -q "$counts" <<<"$summary_output" || fail "E2E summary preserves $counts"
done
[[ "$(wc -l <<<"$summary_output" | tr -d ' ')" == 4 ]] || fail "E2E summary stays bounded"
pass "optional E2E summary reports counts including flaky results"

missing_summary="$(KANDEV_RUN_QUIET_DIR="$TMP_DIR" "$SCRIPT" playwright --summary -- bash -c 'printf "build complete\n"')"
grep -q '^summary=unavailable$' <<<"$missing_summary" || fail "missing E2E summary cannot imply zero failures"
pass "missing E2E summary is explicit"

if summary_failure="$(KANDEV_RUN_QUIET_DIR="$TMP_DIR" "$SCRIPT" e2e --summary -- bash -c 'printf "Error: assertion failed\n"; exit 7')"; then
  fail "E2E summary must preserve a failing exit code"
else
  summary_rc=$?
  [[ "$summary_rc" == 7 ]] || fail "E2E summary changed the underlying exit code"
  grep -q 'Error: assertion failed' <<<"$summary_failure" || fail "E2E summary hides failure context"
fi
pass "optional E2E summary preserves failure context and exit code"

default_e2e="$(KANDEV_RUN_QUIET_DIR="$TMP_DIR" "$SCRIPT" e2e -- bash -c 'printf "12 passed (2.0s)\n"')"
[[ "$(wc -l <<<"$default_e2e" | tr -d ' ')" == 1 ]] || fail "default E2E output must remain one line"
pass "E2E counts are opt-in"

if "$SCRIPT" ordinary --summary -- bash -c 'exit 0' >/dev/null 2>&1; then
  fail "E2E summary must reject unsupported tags"
fi
pass "E2E summaries reject unsupported tags"

for tag in gh-run ci gh-run-view; do
  gh_output="$("$SCRIPT" "$tag" -- "$TMP_DIR/success-with-go-failure")"
  if grep -q '"Action":"fail"' <<<"$gh_output" && grep -q -- '--- FAIL: TestLeaky' <<<"$gh_output" && grep -q 'goleak:' <<<"$gh_output"; then
    pass "$tag extracts Go failures from a successful command"
  else
    fail "$tag extracts Go failures from a successful command"
  fi
done

clean_output="$("$SCRIPT" gh-run -- bash -c 'printf "everything is fine\\n"')"
if [[ "$(wc -l <<<"$clean_output" | tr -d ' ')" == "1" ]] && grep -q '^exit=0 log=' <<<"$clean_output"; then
  pass "gh-run keeps clean successful output compact"
else
  fail "gh-run keeps clean successful output compact"
fi

if failing_output="$("$SCRIPT" gh-run -- bash -c 'printf "%s\\n" "--- FAIL: TestBroken"; exit 1')"; then
  fail "gh-run preserves a nonzero command exit"
elif grep -q -- '--- FAIL: TestBroken' <<<"$failing_output"; then
  pass "gh-run extracts failures from a nonzero command"
else
  fail "gh-run extracts failures from a nonzero command"
fi

normal_output="$("$SCRIPT" ordinary -- bash -c 'printf "ordinary success\\n"')"
if [[ "$(wc -l <<<"$normal_output" | tr -d ' ')" == "1" ]] && grep -q '^exit=0 log=' <<<"$normal_output"; then
  pass "ordinary successful tags remain compact"
else
  fail "ordinary successful tags remain compact"
fi

custom_tmp="$TMP_DIR/custom-tmp"
mkdir -p "$custom_tmp"
custom_tmp_output="$(env -u KANDEV_RUN_QUIET_DIR TMPDIR="$custom_tmp" "$SCRIPT" ordinary -- bash -c 'printf "custom tmp success\\n"')"
if grep -q "^exit=0 log=$custom_tmp/kandev-run\.ordinary\." <<<"$custom_tmp_output"; then
  pass "TMPDIR controls the log location"
else
  fail "TMPDIR controls the log location"
fi

quiet_dir="$TMP_DIR/nested/quiet-logs"
quiet_dir_output="$(KANDEV_RUN_QUIET_DIR="$quiet_dir" TMPDIR="$custom_tmp" "$SCRIPT" ordinary -- bash -c 'printf "quiet dir success\\n"')"
if grep -q "^exit=0 log=$quiet_dir/kandev-run\.ordinary\." <<<"$quiet_dir_output"; then
  pass "KANDEV_RUN_QUIET_DIR overrides TMPDIR and is created"
else
  fail "KANDEV_RUN_QUIET_DIR overrides TMPDIR and is created"
fi

if quiet_dir_failure="$(KANDEV_RUN_QUIET_DIR="$quiet_dir" "$SCRIPT" ordinary -- bash -c 'printf "Error: quiet dir failure\\n"; exit 3')"; then
  fail "KANDEV_RUN_QUIET_DIR preserves failure extraction"
elif grep -q "^exit=3 log=$quiet_dir/kandev-run\.ordinary\." <<<"$quiet_dir_failure" && grep -q 'Error: quiet dir failure' <<<"$quiet_dir_failure"; then
  pass "KANDEV_RUN_QUIET_DIR preserves failure extraction"
else
  fail "KANDEV_RUN_QUIET_DIR preserves failure extraction"
fi

if non_writable_output="$(KANDEV_RUN_QUIET_DIR=/proc/1 "$SCRIPT" ordinary -- bash -c 'printf "ok\\n"' 2>&1)"; then
  fail "non-writable log directory should fail"
elif grep -q 'cannot create log directory: /proc/1' <<<"$non_writable_output"; then
  pass "non-writable log directory is rejected"
else
  fail "non-writable log directory should produce a clear error"
fi
