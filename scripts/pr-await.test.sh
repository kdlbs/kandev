#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/pr-await"

pass() { printf 'ok - %s\n' "$1"; }
fail() {
  printf 'not ok - %s\n' "$1" >&2
  [[ $# -lt 2 ]] || printf '%s\n' "$2" >&2
  exit 1
}

TMP_DIRS=()
cleanup() { [[ "${#TMP_DIRS[@]}" -eq 0 ]] || rm -rf "${TMP_DIRS[@]}"; }
trap cleanup EXIT

make_tmp_dir() {
  local d
  d="$(mktemp -d)"
  TMP_DIRS+=("$d")
  printf '%s' "$d"
}

# A snapshot generator. Each call to the fake pr-state pops the next file from
# a numbered sequence, so a test can describe CI as it evolves over time.
# The last snapshot repeats once the sequence is exhausted.
setup_fake() {
  local dir=$1
  mkdir -p "$dir/seq"
  cat >"$dir/pr-state" <<'FAKE'
#!/usr/bin/env bash
n=$(cat "$FAKE_DIR/counter" 2>/dev/null || echo 0)
n=$((n + 1))
printf '%s' "$n" > "$FAKE_DIR/counter"
last=$(ls "$FAKE_DIR/seq" | wc -l | tr -d ' ')
[[ $n -le $last ]] || n=$last
if [[ -f "$FAKE_DIR/seq/$n.fail" ]]; then exit 1; fi
cat "$FAKE_DIR/seq/$n.json"
FAKE
  cat >"$dir/gh" <<'FAKEGH'
#!/usr/bin/env bash
cat "$FAKE_DIR/gh.json" 2>/dev/null || printf '{"state":"OPEN","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefOid":"aaaaaaaaaaaa"}'
FAKEGH
  cat >"$dir/sleep" <<'FAKESLEEP'
#!/usr/bin/env bash
printf 'slept %s\n' "$1" >> "$FAKE_DIR/sleeps"
FAKESLEEP
  chmod +x "$dir/pr-state" "$dir/gh" "$dir/sleep"
}

snapshot() {
  # snapshot <dir> <index> <passed> <failed> <pending> [complete] [head] [extra-json]
  local dir=$1 idx=$2 passed=$3 failed=$4 pending=$5
  local complete=${6:-true} head=${7:-aaaaaaaaaaaa} extra=${8:-'{}'}
  local failed_arr='[]' pending_arr='[]'
  [[ "$failed" -eq 0 ]] || failed_arr="$(jq -nc --argjson n "$failed" '[range($n) | {name: "Failing \(.)", workflow: "w", status: "completed", conclusion: "failure", run_id: 100, job_id: 200}]')"
  [[ "$pending" -eq 0 ]] || pending_arr="$(jq -nc --argjson n "$pending" '[range($n) | {name: "Pending \(.)", workflow: "w", status: "in_progress", run_id: 101, job_id: 201}]')"
  jq -n --argjson f "$failed_arr" --argjson p "$pending_arr" --argjson passed "$passed" \
        --argjson complete "$complete" --arg head "$head" --argjson extra "$extra" \
    '{pr: {number: 1, head_ref_name: "feat", base_ref_name: "main", head_ref_oid: $head},
      failed_checks: $f, pending_checks: $p, passed_check_count: $passed,
      checks_head_sha: $head, checks_snapshot_complete: $complete,
      approval_required_runs: [], unresolved_review_thread_count: 0,
      unresolved_threads: [], hidden_unresolved_threads: [],
      actionable_issue_comment_count: 0} * $extra' > "$dir/seq/$idx.json"
}

run_await() {
  local dir=$1; shift
  FAKE_DIR="$dir" PR_AWAIT_PR_STATE="$dir/pr-state" PR_AWAIT_GH="$dir/gh" \
    PR_AWAIT_SLEEP="$dir/sleep" "$SCRIPT" "$@"
}

# --- argument validation ---------------------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
run_await "$d" >/dev/null 2>&1 && fail "missing PR should exit non-zero" || pass "missing PR argument is rejected"
run_await "$d" 12 --mode bogus >/dev/null 2>&1 && fail "bad --mode accepted" || pass "invalid --mode is rejected"
run_await "$d" notanumber >/dev/null 2>&1 && fail "non-numeric PR accepted" || pass "non-numeric PR is rejected"
run_await "$d" 12 --interval-sec 0 >/dev/null 2>&1 && fail "zero interval accepted" || pass "zero --interval-sec is rejected"
"$SCRIPT" --help >/dev/null 2>&1 && pass "--help exits 0" || fail "--help should exit 0"

# --- the documented race: a failure while the workflow is still running -----
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 10 1 5          # a job has failed, but 5 checks still pending
snapshot "$d" 2 12 1 3
snapshot "$d" 3 14 2 0          # terminal: a SECOND failure only appeared at the end
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "all-terminal with findings should exit 1, got $rc" "$out"
grep -q '2 failed' <<<"$out" || fail "should report BOTH failures, not just the first" "$out"
grep -q '0 pending' <<<"$out" || fail "should only return when nothing is pending" "$out"
pass "all-terminal waits past an early failure and reports every failure at once"

# --- first-failure returns early (and therefore reports less) --------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 10 1 5
snapshot "$d" 2 14 2 0
out="$(run_await "$d" 12 --mode first-failure --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "first-failure with a failure should exit 1, got $rc" "$out"
grep -q '1 failed' <<<"$out" || fail "first-failure should return on the first failure" "$out"
grep -q '5 pending' <<<"$out" || fail "first-failure returns with checks still pending" "$out"
pass "first-failure returns on the first failing check"

# --- clean terminal state ---------------------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "clean state should exit 0, got $rc" "$out"
grep -q 'clean at this head' <<<"$out" || fail "clean state should say so" "$out"
pass "clean terminal state exits 0"

# --- an incomplete snapshot is not terminal --------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0 false
snapshot "$d" 2 20 0 0 true
out="$(run_await "$d" 12 --interval-sec 1 2>"$d/err")" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "expected 0 after snapshot completed, got $rc" "$out"
grep -q '2 poll' <<<"$out" || fail "should have polled twice, not stopped on the incomplete snapshot" "$out"
pass "checks_snapshot_complete=false keeps waiting"

# --- deadline ---------------------------------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 10 0 4
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 2 ]] || fail "deadline should exit 2, got $rc" "$out"
grep -q 'STILL PENDING' <<<"$out" || fail "deadline should name pending checks" "$out"
grep -q 'Pending 0' <<<"$out" || fail "deadline should list check names" "$out"
pass "deadline exits 2 and names the pending checks"

# --- approval-required is blocked, not green --------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 0 0 0 true aaaaaaaaaaaa \
  '{"approval_required_runs":[{"run_id":9,"name":"E2E","conclusion":"action_required"}]}'
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "approval_required should exit 3, got $rc" "$out"
grep -q 'approval required' <<<"$out" || fail "should explain the block" "$out"
pass "approval_required_runs exits 3 instead of reading as clean"

# --- merged/closed PR -------------------------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 10 0 2
printf '{"state":"MERGED","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefOid":"aaaaaaaaaaaa"}' > "$d/gh.json"
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "merged PR should exit 3, got $rc" "$out"
grep -q 'MERGED' <<<"$out" || fail "should report the merged state" "$out"
pass "a merged PR exits 3 rather than waiting forever"

# --- merge conflict ---------------------------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
printf '{"state":"OPEN","mergeable":"CONFLICTING","mergeStateStatus":"DIRTY","headRefOid":"aaaaaaaaaaaa"}' > "$d/gh.json"
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "conflict should exit 1, got $rc" "$out"
grep -q 'resolve the merge conflict' <<<"$out" || fail "conflict should lead the NEXT line" "$out"
pass "a merge conflict is surfaced even when every check passed"

# --- transient pr-state failures recover; sustained ones block --------------
d="$(make_tmp_dir)"; setup_fake "$d"
: > "$d/seq/1.fail"; snapshot "$d" 1 0 0 0
snapshot "$d" 2 20 0 0
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "one transient failure should be retried, got $rc" "$out"
pass "a transient pr-state failure is retried"

d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do : > "$d/seq/$i.fail"; snapshot "$d" "$i" 0 0 0; done
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "three consecutive failures should exit 3, got $rc" "$out"
grep -q 'unknown' <<<"$out" || fail "should say CI state is unknown" "$out"
pass "three consecutive pr-state failures exit 3 as unknown, never as clean"

# --- a push during the wait restarts the gate ------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 10 0 3 true aaaaaaaaaaaa
snapshot "$d" 2 2 0 8 true bbbbbbbbbbbb
snapshot "$d" 3 20 0 0 true bbbbbbbbbbbb
cat > "$d/gh" <<'GH2'
#!/usr/bin/env bash
n=$(cat "$FAKE_DIR/counter" 2>/dev/null || echo 1)
if [[ $n -le 1 ]]; then oid=aaaaaaaaaaaa; else oid=bbbbbbbbbbbb; fi
printf '{"state":"OPEN","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefOid":"%s"}' "$oid"
GH2
chmod +x "$d/gh"
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "expected clean after the new head settled, got $rc" "$out"
grep -q 'head moved 1 time' <<<"$out" || fail "should report the head change" "$out"
grep -q 'bbbbbbbbb' <<<"$out" || fail "should report the NEW head, not the stale one" "$out"
pass "a push during the wait restarts the gate and reports the new head"

# --- stdout carries only the report ----------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 10 0 2
snapshot "$d" 2 12 0 0
out="$(run_await "$d" 12 --interval-sec 1 2>/dev/null)"
grep -q '^poll ' <<<"$out" && fail "progress leaked into stdout" "$out"
err="$(run_await "$d" 12 --interval-sec 1 2>&1 >/dev/null)"
grep -q '^poll ' <<<"$err" || fail "progress should go to stderr" "$err"
pass "progress goes to stderr; stdout carries only the final report"

# --- --quiet silences progress ---------------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 10 0 0
err="$(run_await "$d" 12 --interval-sec 1 --quiet 2>&1 >/dev/null)"
[[ -z "$err" ]] || fail "--quiet should emit nothing on stderr" "$err"
pass "--quiet silences per-poll progress"

# --- json format -----------------------------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 14 2 0
out="$(run_await "$d" 12 --interval-sec 1 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "json mode should keep the exit code, got $rc" "$out"
jq -e . >/dev/null <<<"$out" || fail "json mode must emit valid JSON" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'terminal' ]] || fail "outcome missing" "$out"
[[ "$(jq -r '.summary.failed_checks | length' <<<"$out")" == '2' ]] || fail "summary should be embedded whole" "$out"
pass "--format json embeds the full pr-state summary and keeps the exit code"

# --- the cost claim: one invocation, not one per poll ----------------------
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3 4 5 6 7; do snapshot "$d" "$i" "$i" 0 $((8 - i)); done
snapshot "$d" 8 20 0 0
run_await "$d" 12 --interval-sec 1 --quiet >/dev/null 2>&1 || true
[[ "$(wc -l < "$d/sleeps" | tr -d ' ')" -eq 7 ]] || fail "expected 7 internal sleeps" "$(cat "$d/sleeps")"
pass "eight polls happen inside a single invocation (7 internal sleeps, 0 caller round-trips)"

printf '\nall tests passed\n'
