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
  cat >"$dir/jq-ok" <<'JQOK'
#!/usr/bin/env bash
[ "$1" = "--version" ] && { echo "jq-1.7.1-fake"; exit 0; }
exit 0
JQOK
  cat >"$dir/jq-bad" <<'JQBAD'
#!/usr/bin/env bash
[ "$1" = "--version" ] && { echo "jq-1.6-fake"; exit 0; }
exec sleep 30
JQBAD
  cat >"$dir/bash-ok" <<'BASHOK'
#!/usr/bin/env bash
case "$2" in
  *BASH_VERSION*) printf '5.9.9-fake' ;;
  *) exit 0 ;;
esac
BASHOK
  cat >"$dir/bash-bad" <<'BASHBAD'
#!/usr/bin/env bash
case "$2" in
  *BASH_VERSION*) printf '3.2.57-fake' ;;
  *) exit 1 ;;
esac
BASHBAD
  chmod +x "$dir/jq-ok" "$dir/jq-bad" "$dir/bash-ok" "$dir/bash-bad"
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
      actionable_issue_comment_count: 0, errors: [],
      review_evidence: {active_changes_requested_count: 0,
                        blocked_exact_current_head_review_count: 0}} * $extra' > "$dir/seq/$idx.json"
}

run_await() {
  local dir=$1; shift
  FAKE_DIR="$dir" PR_AWAIT_PR_STATE="$dir/pr-state" PR_AWAIT_GH="$dir/gh" \
    PR_AWAIT_SLEEP="$dir/sleep" \
    PR_AWAIT_JQ="${PROBE_JQ:-$dir/jq-ok}" PR_AWAIT_PROBE_BASH="${PROBE_BASH:-$dir/bash-ok}" \
    "$SCRIPT" "$@"
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

# --- P1: a snapshot carrying errors is never clean ---------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3 4; do
  snapshot "$d" "$i" 20 0 0 true aaaaaaaaaaaa \
    '{"errors":[{"source":"review_threads","message":"graphql failed"}]}'
done
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "errors in snapshot must not exit 0, got $rc" "$out"
grep -q 'review_threads' <<<"$out" || fail "should name the failing source" "$out"
pass "a pr-state snapshot with non-empty errors exits 3, never clean"

# --- P1: a transient error that clears is fine ------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0 true aaaaaaaaaaaa '{"errors":[{"source":"x","message":"blip"}]}'
snapshot "$d" 2 20 0 0
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "a cleared transient error should exit 0, got $rc" "$out"
pass "an error that clears on a later poll does not block"

# --- P1: an unknown thread count is unknown, not zero -----------------------
# This is the observed bash-3.2 pr-state failure: pagination dies non-fatally
# and the count comes back null while GitHub has unresolved threads.
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3 4; do
  snapshot "$d" "$i" 20 0 0 true aaaaaaaaaaaa '{"unresolved_review_thread_count":null}'
done
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "null thread count must not read as 0/clean, got $rc" "$out"
pass "a null unresolved_review_thread_count blocks instead of counting as zero"

# --- P1: blocking reviews count as findings ---------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0 true aaaaaaaaaaaa \
  '{"review_evidence":{"active_changes_requested_count":1,"blocked_exact_current_head_review_count":0}}'
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "CHANGES_REQUESTED with no threads should exit 1, got $rc" "$out"
grep -qi 'changes requested' <<<"$out" || fail "should report the blocking review" "$out"
pass "an active CHANGES_REQUESTED review is a finding even with zero threads"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0 true aaaaaaaaaaaa \
  '{"review_evidence":{"active_changes_requested_count":0,"blocked_exact_current_head_review_count":2}}'
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "blocked exact-head reviews should exit 1, got $rc" "$out"
pass "blocked exact-current-head reviews count as findings"

# --- P1: a failed merge-state query is unknown, not clean -------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
cat > "$d/gh" <<'GHFAIL'
#!/usr/bin/env bash
exit 1
GHFAIL
chmod +x "$d/gh"
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "failed gh pr view must not exit 0, got $rc" "$out"
grep -qi 'merge state' <<<"$out" || fail "should say merge state is unknown" "$out"
pass "a failed merge-state query exits 3 instead of reporting clean"

# --- P2: first-failure must not act on a stale head -------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
# snapshot 1 describes the OLD head; gh already reports the NEW one.
snapshot "$d" 1 10 1 5 true aaaaaaaaaaaa
snapshot "$d" 2 20 0 0 true bbbbbbbbbbbb
cat > "$d/gh" <<'GH3'
#!/usr/bin/env bash
printf '{"state":"OPEN","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefOid":"bbbbbbbbbbbb"}'
GH3
chmod +x "$d/gh"
out="$(run_await "$d" 12 --mode first-failure --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "stale first-failure snapshot must be discarded, got $rc" "$out"
grep -q '0 failed' <<<"$out" || fail "should report the NEW head, not the stale failure" "$out"
pass "first-failure discards a snapshot whose checks belong to a superseded head"

# --- P2: leading-zero arguments are decimal, not octal ---------------------
# Assert the parsed values rather than timing the wait: "010" as octal is 8,
# which a duration test would only catch by waiting out the difference.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 09 --quiet --format json 2>"$d/err")" || true
grep -q 'value too great for base' "$d/err" && fail "09 must not be read as octal" "$(cat "$d/err")"
[[ "$(jq -r '.deadline_sec' <<<"$out")" == '540' ]] \
  || fail "--deadline-min 09 should be 540s, got $(jq -r '.deadline_sec' <<<"$out")" "$out"
pass "--deadline-min 09 parses as 9 minutes, not an octal error"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(run_await "$d" 12 --interval-sec 010 --deadline-min 010 --quiet --format json 2>"$d/err")" || true
[[ "$(jq -r '.interval_sec' <<<"$out")" == '10' ]] \
  || fail "--interval-sec 010 should be 10, got $(jq -r '.interval_sec' <<<"$out")" "$out"
[[ "$(jq -r '.deadline_sec' <<<"$out")" == '600' ]] \
  || fail "--deadline-min 010 should be 600s (octal would be 480), got $(jq -r '.deadline_sec' <<<"$out")" "$out"
pass "--interval-sec 010 and --deadline-min 010 parse as decimal 10, not octal 8"


# --- toolchain preflight: pr-state degrades silently, so refuse to run ------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(PROBE_JQ="$d/jq-bad" run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "jq that loops on empty-match gsub must block, got $rc" "$out"
grep -q 'jq-1.6-fake' <<<"$out" || fail "should name the jq version" "$out"
grep -qi 'cannot be read' <<<"$out" || fail "should explain the impact" "$out"
pass "a jq whose gsub loops blocks before any polling"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(PROBE_BASH="$d/bash-bad" run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "bash that fails empty-array expansion must block, got $rc" "$out"
grep -q '3.2.57-fake' <<<"$out" || fail "should name the bash version" "$out"
grep -qi 'unresolved review threads' <<<"$out" || fail "should explain the silent thread loss" "$out"
pass "a bash that aborts empty-array expansion blocks before any polling"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "a good toolchain should not block, got $rc" "$out"
grep -q 'toolchain: jq-1.7.1-fake, bash 5.9.9-fake' <<<"$out" \
  || fail "the report must record the verified toolchain" "$out"
pass "a verified toolchain is recorded in every report"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(run_await "$d" 12 --interval-sec 1 --quiet --format json 2>/dev/null)"
[[ "$(jq -r '.toolchain' <<<"$out")" == 'jq-1.7.1-fake, bash 5.9.9-fake' ]] \
  || fail "json report must carry the toolchain" "$out"
[[ "$(jq -r '.evidence_complete' <<<"$out")" == 'true' ]] || fail "evidence_complete missing" "$out"
pass "json report carries toolchain and evidence_complete"


# --- mergeable UNKNOWN is inconclusive, not clean ---------------------------
# GitHub returns UNKNOWN while it computes mergeability; merge-conflicts.md
# says query again rather than deciding.
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3 4; do snapshot "$d" "$i" 20 0 0; done
printf '{"state":"OPEN","mergeable":"UNKNOWN","mergeStateStatus":"UNKNOWN","headRefOid":"aaaaaaaaaaaa"}' > "$d/gh.json"
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "persistent mergeable=UNKNOWN must not exit 0, got $rc" "$out"
grep -qi 'mergeability' <<<"$out" || fail "should explain the unknown mergeability" "$out"
pass "a persistently UNKNOWN mergeability exits 3 instead of clean"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0; snapshot "$d" 2 20 0 0
cat > "$d/gh" <<'GHU'
#!/usr/bin/env bash
n=$(cat "$FAKE_DIR/counter" 2>/dev/null || echo 1)
if [[ $n -le 1 ]]; then m=UNKNOWN; else m=MERGEABLE; fi
printf '{"state":"OPEN","mergeable":"%s","mergeStateStatus":"CLEAN","headRefOid":"aaaaaaaaaaaa"}' "$m"
GHU
chmod +x "$d/gh"
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "UNKNOWN that resolves should exit 0, got $rc" "$out"
pass "an UNKNOWN mergeability that resolves does not block"

# --- the deadline is honored while retrying, not only while polling ---------
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3 4 5; do : > "$d/seq/$i.fail"; snapshot "$d" "$i" 0 0 0; done
out="$(run_await "$d" 12 --interval-sec 30 --deadline-min 0 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "expected blocked, got $rc" "$out"
sleeps=$(wc -l < "$d/sleeps" 2>/dev/null | tr -d ' ' || echo 0)
[[ "${sleeps:-0}" -le 1 ]] || fail "a zero deadline must not burn 3 retry intervals, slept $sleeps times"
pass "a retry path stops at the deadline instead of sleeping out its full strike count"

# --- the jq probe works without timeout(1) on PATH --------------------------
# Stock macOS ships no /usr/bin/timeout, so a probe that depends on it would
# silently skip on the exact hosts that carry the broken jq.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
mkdir -p "$d/minbin"
for cmd in jq bash sleep kill wc cat printf; do
  src="$(command -v "$cmd" 2>/dev/null)"; [ -n "$src" ] && ln -sf "$src" "$d/minbin/$cmd" 2>/dev/null || true
done
command -v timeout >/dev/null 2>&1 && ! [ -e "$d/minbin/timeout" ] || true
out="$(PATH="$d/minbin" PROBE_JQ="$d/jq-bad" run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "hanging jq must be caught without timeout(1), got $rc" "$out"
grep -q 'jq-1.6-fake' <<<"$out" || fail "should still name the bad jq" "$out"
pass "the jq probe catches a hanging jq with no timeout(1) available"

# --- the suite is wired into make test-scripts ------------------------------
grep -q 'bash scripts/pr-await.test.sh' "$ROOT_DIR/Makefile" \
  || fail "pr-await.test.sh must be registered in make test-scripts"
pass "the suite runs under make test-scripts"


printf '\nall tests passed\n'
