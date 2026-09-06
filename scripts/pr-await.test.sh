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
last=$(ls "$FAKE_DIR"/seq/*.json 2>/dev/null | wc -l | tr -d ' ')
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
        --argjson failed "$failed" --argjson pending "$pending" \
        --argjson complete "$complete" --arg head "$head" --argjson extra "$extra" \
    '{pr: {number: 1, head_ref_name: "feat", base_ref_name: "main", head_ref_oid: $head},
      failed_checks: $f, pending_checks: $p, passed_check_count: $passed,
      check_count: ($passed + $failed + $pending),
      checks_head_sha: $head, checks_snapshot_complete: $complete,
      terminal_checks: [],
      required_status_checks: [], required_status_checks_known: true,
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

assert_json_envelope() {
  local name=$1 json=$2
  jq -e '
    (keys | sort) == [
      "deadline_sec", "evidence_complete", "exit_code", "head_changes",
      "interval_sec", "merge_state_status", "mergeable", "message", "mode",
      "outcome", "polls", "pr", "summary", "toolchain", "waited_sec"
    ]
    and (.pr | type) == "number"
    and (.message == null or (.message | type) == "string")
    and (.summary == null or (.summary | type) == "object")
  ' <<<"$json" >/dev/null || fail "$name" "$json"
}

# --- argument validation ---------------------------------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
run_await "$d" >/dev/null 2>&1 && fail "missing PR should exit non-zero" || pass "missing PR argument is rejected"
run_await "$d" 12 --mode bogus >/dev/null 2>&1 && fail "bad --mode accepted" || pass "invalid --mode is rejected"
run_await "$d" notanumber >/dev/null 2>&1 && fail "non-numeric PR accepted" || pass "non-numeric PR is rejected"
run_await "$d" 0 >/dev/null 2>&1 && fail "PR 0 accepted" || pass "PR #0 is rejected as a usage error"
run_await "$d" 00 >/dev/null 2>&1 && fail "PR 00 accepted" || pass "PR #00 is rejected as a usage error"
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

# --- first-failure requires complete evidence before returning --------------
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do
  snapshot "$d" "$i" 10 1 2 false aaaaaaaaaaaa \
    '{"errors":[{"source":"review_threads","message":"graphql failed"}]}'
done
out="$(run_await "$d" 12 --mode first-failure --interval-sec 1 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "incomplete first-failure evidence should block, got $rc" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'blocked-evidence' ]] \
  || fail "incomplete first-failure evidence should report blocked-evidence" "$out"
assert_json_envelope "blocked-evidence must use the JSON envelope" "$out"
pass "first-failure waits for complete evidence before returning a failure"

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
grep -q '3 poll' <<<"$out" || fail "should confirm the terminal rollup after the incomplete snapshot" "$out"
pass "checks_snapshot_complete=false and the first terminal snapshot keep waiting"

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

# --- clean checks can still require human approval --------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
printf '{"state":"OPEN","mergeable":"MERGEABLE","mergeStateStatus":"BLOCKED","reviewDecision":"REVIEW_REQUIRED","headRefOid":"aaaaaaaaaaaa"}' > "$d/gh.json"
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "approval-gated clean checks should exit 0, got $rc" "$out"
grep -q 'review: REVIEW_REQUIRED' <<<"$out" || fail "wait output should expose the review decision" "$out"
grep -qi 'human approval is required' <<<"$out" || fail "wait output should distinguish the approval gate" "$out"
pass "clean checks report the human approval gate without a code failure"

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
assert_json_envelope "terminal JSON must use the shared envelope" "$out"
[[ "$(jq -r '.pr' <<<"$out")" == '12' ]] || fail "terminal JSON must include the PR number" "$out"
[[ "$(jq -r '.message' <<<"$out")" == 'null' ]] || fail "terminal JSON message must be null" "$out"
pass "--format json embeds the full pr-state summary and keeps the exit code"

# --- the cost claim: one invocation, not one per poll ----------------------
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3 4 5 6 7; do snapshot "$d" "$i" "$i" 0 $((8 - i)); done
snapshot "$d" 8 20 0 0
run_await "$d" 12 --interval-sec 1 --quiet >/dev/null 2>&1 || true
[[ "$(wc -l < "$d/sleeps" | tr -d ' ')" -eq 8 ]] || fail "expected 8 internal sleeps" "$(cat "$d/sleeps")"
pass "nine polls happen inside a single invocation (8 internal sleeps, 0 caller round-trips)"

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
# An old toolchain is recorded, not refused. pr-state works on jq 1.6 and bash
# 3.2 now, so a version gate would only turn away working installations; the
# snapshot-quality guards below cover a degraded pr-state whatever its cause.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(PROBE_JQ="$d/jq-bad" run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "an old jq must not block a healthy run, got $rc" "$out"
grep -q 'jq-1.6-fake' <<<"$out" || fail "the old jq should still be recorded" "$out"
pass "an old jq is recorded in the report rather than refused"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(PROBE_BASH="$d/bash-bad" run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "an old bash must not block a healthy run, got $rc" "$out"
grep -q '3.2.57-fake' <<<"$out" || fail "the old bash should still be recorded" "$out"
pass "an old bash is recorded in the report rather than refused"

# But a reader whose pr-state produced nothing usable needs to know the
# toolchain is a known cause of exactly that.
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do : > "$d/seq/$i.fail"; snapshot "$d" "$i" 0 0 0; done
out="$(PROBE_JQ="$d/jq-bad" PROBE_BASH="$d/bash-bad" run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "expected blocked, got $rc" "$out"
grep -qi 'Toolchain note' <<<"$out" || fail "a blocked report should name the toolchain risk" "$out"
grep -qi 'empty-matching gsub' <<<"$out" || fail "should explain the jq risk" "$out"
grep -qi 'empty array under set -u' <<<"$out" || fail "should explain the bash risk" "$out"
pass "a blocked report explains a known-bad toolchain as a possible cause"

# A current toolchain gets no note, so the hint stays signal.
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do : > "$d/seq/$i.fail"; snapshot "$d" "$i" 0 0 0; done
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "expected blocked, got $rc" "$out"
grep -qi 'Toolchain note' <<<"$out" && fail "a current toolchain must not emit a note" "$out"
pass "a current toolchain produces no toolchain note"

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
: > "$d/sleeps.seen"; [[ -f "$d/sleeps" ]] || : > "$d/sleeps"
[[ "$rc" -eq 3 ]] || fail "expected blocked, got $rc" "$out"
sleeps=$(wc -l < "$d/sleeps" 2>/dev/null | tr -d ' ' || echo 0)
[[ "${sleeps:-0}" -le 1 ]] || fail "a zero deadline must not burn 3 retry intervals, slept $sleeps times"
pass "a retry path stops at the deadline instead of sleeping out its full strike count"

# --- the jq probe works without timeout(1) on PATH --------------------------
# Stock macOS ships no /usr/bin/timeout, so a probe that depends on it would
# silently skip on the exact hosts that carry the broken jq.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
cat > "$d/pr-state" <<'HANGNT'
#!/usr/bin/env bash
exec sleep 120
HANGNT
chmod +x "$d/pr-state"
start=$SECONDS
out="$(PR_AWAIT_NO_TIMEOUT=1 PR_AWAIT_READ_FLOOR=2 run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -ne 0 ]] || fail "a stalled read must be bounded without timeout(1), got $rc" "$out"
[[ $((SECONDS - start)) -lt 12 ]] || fail "the shell fallback did not bound the read"
pass "a stalled read is bounded without timeout(1) available"

# The fallback must also complete a normal run, not merely catch a hang.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(PR_AWAIT_NO_TIMEOUT=1 run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "the shell fallback must handle a healthy read, got $rc" "$out"
pass "the shell fallback completes a normal run without timeout(1)"

# --- the suite is wired into make test-scripts ------------------------------
grep -q 'bash scripts/pr-await.test.sh' "$ROOT_DIR/Makefile" \
  || fail "pr-await.test.sh must be registered in make test-scripts"
pass "the suite runs under make test-scripts"


# --- the deadline is absolute, not restarted by every push -----------------
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3 4 5 6; do snapshot "$d" "$i" 2 0 5 true "head$i"; done
cat > "$d/gh" <<'GHMOVE'
#!/usr/bin/env bash
n=$(cat "$FAKE_DIR/counter" 2>/dev/null || echo 1)
printf '{"state":"OPEN","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefOid":"head%s"}' "$n"
GHMOVE
chmod +x "$d/gh"
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 2 ]] || fail "a repeatedly-moving head must still hit the deadline, got $rc" "$out"
pass "a repeatedly-moving head still reaches the deadline"

# The behavioural test above cannot separate "clock reset" from "clock kept"
# without waiting out a real deadline, so assert the invariant directly: the
# deadline is absolute for the invocation, and a push discards stale evidence,
# never the caller's time limit.
[[ "$(grep -c 'START_SEC=\$SECONDS' "$ROOT_DIR/scripts/pr-await")" -eq 1 ]] \
  || fail "the deadline clock must be assigned once, not reset on head change"
pass "the deadline clock is assigned exactly once (a push cannot defer the hard stop)"

# --- blocked outcomes still honor --format json ----------------------------
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do : > "$d/seq/$i.fail"; snapshot "$d" "$i" 0 0 0; done
out="$(run_await "$d" 12 --interval-sec 1 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "expected 3, got $rc" "$out"
jq -e . >/dev/null <<<"$out" || fail "blocked-access must still emit JSON" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'blocked-access' ]] || fail "outcome missing" "$out"
[[ "$(jq -r '.exit_code' <<<"$out")" == '3' ]] || fail "exit_code missing" "$out"
assert_json_envelope "blocked-access JSON must use the shared envelope" "$out"
pass "a blocked-access result is still valid JSON under --format json"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
out="$(PROBE_JQ="$d/jq-bad" run_await "$d" 12 --interval-sec 1 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "an old jq must not block the JSON path, got $rc" "$out"
jq -e . >/dev/null <<<"$out" || fail "must emit valid JSON" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'terminal' ]] || fail "outcome should be terminal" "$out"
grep -q 'jq-1.6-fake' <<<"$(jq -r '.toolchain' <<<"$out")" || fail "toolchain should record the jq" "$out"
pass "an old jq is recorded in the JSON toolchain field, not treated as blocked"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
cat > "$d/gh" <<'GHF'
#!/usr/bin/env bash
exit 1
GHF
chmod +x "$d/gh"
out="$(run_await "$d" 12 --interval-sec 1 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
jq -e . >/dev/null <<<"$out" || fail "merge-state failure must emit JSON" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'blocked-merge-state' ]] || fail "outcome missing" "$out"
assert_json_envelope "blocked-merge-state JSON must use the shared envelope" "$out"
pass "a blocked merge-state result is still valid JSON under --format json"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0 true aaaaaaaaaaaa \
  '{"approval_required_runs":[{"run_id":9,"name":"E2E","conclusion":"action_required"}]}'
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "blocked-approval JSON should exit 3, got $rc" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'blocked-approval' ]] || fail "blocked-approval outcome missing" "$out"
assert_json_envelope "blocked-approval JSON must use the shared envelope" "$out"
pass "blocked-approval produces the shared JSON envelope"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
printf '{"state":"MERGED","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefOid":"aaaaaaaaaaaa"}' > "$d/gh.json"
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "blocked-pr-state JSON should exit 3, got $rc" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'blocked-pr-state' ]] || fail "blocked-pr-state outcome missing" "$out"
assert_json_envelope "blocked-pr-state JSON must use the shared envelope" "$out"
pass "blocked-pr-state produces the shared JSON envelope"

d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do snapshot "$d" "$i" 20 0 0; done
printf '{"state":"OPEN","mergeable":"UNKNOWN","mergeStateStatus":"UNKNOWN","headRefOid":"aaaaaaaaaaaa"}' > "$d/gh.json"
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "blocked-merge-unknown JSON should exit 3, got $rc" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'blocked-merge-unknown' ]] || fail "blocked-merge-unknown outcome missing" "$out"
assert_json_envelope "blocked-merge-unknown JSON must use the shared envelope" "$out"
pass "blocked-merge-unknown produces the shared JSON envelope"


# --- hidden threads are a subset of the total, not an addition -------------
# pr-state:922 counts every unresolved thread; hidden_unresolved_threads is the
# subset filtered out of unresolved_threads. Summing both double-counts.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0 true aaaaaaaaaaaa \
  '{"unresolved_review_thread_count":4,"unresolved_threads":[],
    "hidden_unresolved_threads":[{"thread_id":"t1","comment_id":1,"author":"bot","path":"a.go"},
                                 {"thread_id":"t2","comment_id":2,"author":"bot","path":"b.go"},
                                 {"thread_id":"t3","comment_id":3,"author":"bot","path":"c.go"},
                                 {"thread_id":"t4","comment_id":4,"author":"bot","path":"d.go"}]}'
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "expected findings, got $rc" "$out"
grep -q 'fix 0 failed check(s) and 4 thread(s)' <<<"$out" \
  || fail "4 unresolved threads must count as 4, not 8" "$out"
grep -q '4 total: 0 visible, 4 hidden' <<<"$out" \
  || fail "the header must not label the total as visible" "$out"
pass "hidden threads are counted once, and the total is not labelled visible"

# --- an empty check rollup is not terminal ---------------------------------
# Right after a push GitHub can expose the new head before Actions registers
# any checks. Zero of everything is "not yet", not "clean".
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do snapshot "$d" "$i" 0 0 0; done
snapshot "$d" 4 20 0 0
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "expected clean once checks appeared, got $rc" "$out"
grep -q '5 poll' <<<"$out" || fail "must wait for checks to appear and stabilize, not stop at the empty rollup" "$out"
pass "an empty check rollup keeps waiting until the terminal rollup stabilizes"

d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3 4 5; do snapshot "$d" "$i" 0 0 0; done
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -ne 0 ]] || fail "a permanently empty rollup must never exit 0" "$out"
pass "a rollup that never populates does not become clean at the deadline"
grep -qi 'no checks registered yet' <<<"$out" \
  || fail "a zero-check deadline must say no checks registered, not '0 check(s) pending'" "$out"
grep -q '0 check(s) pending' <<<"$out" \
  && fail "a zero-check deadline must not be phrased as pending checks" "$out"
pass "a zero-check deadline is distinguished from a deadline with pending checks"

# Skipped and neutral checks are terminal evidence even though pr-state does
# not include them in passed_check_count. The separate check_count prevents the
# await loop from treating such a rollup as if Actions had not registered it.
for conclusion in skipped neutral; do
  d="$(make_tmp_dir)"; setup_fake "$d"
  snapshot "$d" 1 0 0 0 true aaaaaaaaaaaa "{\"check_count\":1,\"terminal_conclusions\":[\"$conclusion\"]}"
  out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 1 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
  [[ "$rc" -eq 0 ]] || fail "$conclusion-only check rollup should be terminal, got $rc" "$out"
  [[ "$(jq -r '.outcome' <<<"$out")" == 'terminal' ]] || fail "$conclusion-only rollup should emit terminal" "$out"
done
pass "skipped and neutral terminal checks do not look like an empty rollup"

# --- a sparse non-empty rollup is provisional -------------------------------
# Immediately after a push, GitHub can report a couple of terminal checks
# before the rest of the required workflows materialize. A non-zero check count
# must not be treated as proof that the rollup is complete.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 2 0 0 true aaaaaaaaaaaa
snapshot "$d" 2 45 0 0 true aaaaaaaaaaaa
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "sparse rollup should settle to clean, got $rc" "$out"
grep -q '45 passed' <<<"$out" || fail "must report the materialized full rollup, not the sparse one" "$out"
grep -q '3 poll' <<<"$out" || fail "must confirm the expanded rollup before returning" "$out"
pass "a sparse non-empty rollup stays provisional until the full set stabilizes"

# The set of checks is logical state, not an API-order-sensitive sequence. A
# reordered response must not reset the terminal confirmation streak.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 2 0 0 true aaaaaaaaaaaa \
  '{"successful_checks":[{"name":"check-b","workflow":"ci","status":"completed","conclusion":"success"},{"name":"check-a","workflow":"ci","status":"completed","conclusion":"success"}]}'
snapshot "$d" 2 2 0 0 true aaaaaaaaaaaa \
  '{"successful_checks":[{"name":"check-a","workflow":"ci","status":"completed","conclusion":"success"},{"name":"check-b","workflow":"ci","status":"completed","conclusion":"success"}]}'
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "reordered terminal checks should settle clean, got $rc" "$out"
grep -q '2 poll' <<<"$out" || fail "reordered terminal checks should not reset the confirmation streak" "$out"
pass "terminal rollup fingerprint ignores API ordering"

# --- required contexts prevent an optional-only rollup from becoming clean ---
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 1 0 0 true aaaaaaaaaaaa \
  '{"required_status_checks":["E2E Tests Passed"],"required_status_checks_known":true,
    "successful_checks":[{"name":"CodeRabbit","workflow":"review","status":"completed","conclusion":"success"}]}'
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 2 ]] || fail "an optional-only rollup must hit the deadline, got $rc" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'deadline' ]] || fail "optional-only rollup outcome missing" "$out"
grep -q 'E2E Tests Passed' <<<"$(jq -r '.summary.pending_checks[].name' <<<"$out")" \
  || fail "missing required context should remain pending" "$out"
pass "an optional-only rollup does not become clean while required contexts are absent"

d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do
  snapshot "$d" "$i" 1 0 0 true aaaaaaaaaaaa \
    '{"required_status_checks_known":false}'
done
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 1 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "an unavailable required-status policy should block, got $rc" "$out"
[[ "$(jq -r '.outcome' <<<"$out")" == 'blocked-required-statuses' ]] \
  || fail "an unavailable required-status policy should report its blocked outcome" "$out"
pass "an unavailable required-status policy never becomes clean"

d="$(make_tmp_dir)"; setup_fake "$d"
terminal_required='{"check_count":1,"required_status_checks":["Skipped required"],"required_status_checks_known":true,"terminal_checks":[{"name":"Skipped required","workflow":"ci","status":"completed","conclusion":"skipped"}]}'
snapshot "$d" 1 0 0 0 true aaaaaaaaaaaa "$terminal_required"
snapshot "$d" 2 0 0 0 true aaaaaaaaaaaa "$terminal_required"
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "a skipped required context should be terminal, got $rc" "$out"
grep -q '0 pending' <<<"$out" || fail "a skipped required context must not be synthesized as pending" "$out"
pass "skipped required contexts remain terminal instead of pending"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 1 0 0 true aaaaaaaaaaaa \
  '{"required_status_checks":["E2E Tests Passed"],"required_status_checks_known":true,
    "successful_checks":[{"name":"CodeRabbit","workflow":"review","status":"completed","conclusion":"success"}]}'
snapshot "$d" 2 2 0 0 true aaaaaaaaaaaa \
  '{"required_status_checks":["E2E Tests Passed"],"required_status_checks_known":true,
    "successful_checks":[{"name":"CodeRabbit","workflow":"review","status":"completed","conclusion":"success"},
                         {"name":"E2E Tests Passed","workflow":"e2e","status":"completed","conclusion":"success"}]}'
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "a materialized required context should allow clean, got $rc" "$out"
grep -q '3 poll' <<<"$out" || fail "required context should settle after it materializes" "$out"
pass "a required context that materializes allows the terminal verdict"

# --- a base that advanced leaves the merge result untested -----------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0 true aaaaaaaaaaaa '{"pr":{"base_advanced_since_head":true}}'
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "base_advanced_since_head must prevent a clean verdict, got $rc" "$out"
grep -qi 'base advanced' <<<"$out" || fail "should report the advanced base" "$out"
pass "an advanced base is reported and prevents a clean verdict"

# --- approval-required must belong to the current head ---------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 10 0 2 true aaaaaaaaaaaa \
  '{"approval_required_runs":[{"run_id":9,"name":"E2E","conclusion":"action_required"}]}'
snapshot "$d" 2 20 0 0 true bbbbbbbbbbbb
cat > "$d/gh" <<'GHAP'
#!/usr/bin/env bash
printf '{"state":"OPEN","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefOid":"bbbbbbbbbbbb"}'
GHAP
chmod +x "$d/gh"
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 0 ]] || fail "approval for a superseded head must be discarded, got $rc" "$out"
pass "an approval-required run from a superseded head is not actionable"

# --- a leading-zero PR number survives the JSON path -----------------------
d="$(make_tmp_dir)"; setup_fake "$d"
for i in 1 2 3; do : > "$d/seq/$i.fail"; snapshot "$d" "$i" 0 0 0; done
out="$(run_await "$d" 0012 --interval-sec 1 --quiet --format json 2>/dev/null)" && rc=0 || rc=$?
jq -e . >/dev/null <<<"$out" || fail "leading-zero PR must not break the JSON envelope" "$out"
[[ "$(jq -r '.pr' <<<"$out")" == '12' ]] || fail "PR should normalize to 12" "$out"
pass "a leading-zero PR number normalizes and keeps the JSON envelope valid"


# The bounded reads are always captured with $(...). A watchdog that keeps that
# pipe open blocks the substitution long after the command itself has finished,
# so a fast run must stay fast when captured, not only when redirected.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
start=$SECONDS
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 30 --quiet 2>/dev/null)" && rc=0 || rc=$?
elapsed=$((SECONDS - start))
[[ "$rc" -eq 0 ]] || fail "expected clean, got $rc" "$out"
[[ "$elapsed" -lt 10 ]] || fail "a captured fast run must not wait on the watchdog, took ${elapsed}s"
pass "a bounded read does not hold the command substitution open after it finishes"

# --- a stalled GitHub read cannot outlive the deadline ---------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
cat > "$d/pr-state" <<'HANG'
#!/usr/bin/env bash
exec sleep 120
HANG
chmod +x "$d/pr-state"
start=$SECONDS
out="$(PR_AWAIT_READ_FLOOR=2 run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet 2>/dev/null)" && rc=0 || rc=$?
elapsed=$((SECONDS - start))
[[ "$rc" -ne 0 ]] || fail "a stalled pr-state must never exit 0, got $rc" "$out"
[[ "$elapsed" -lt 12 ]] || fail "a zero deadline must not wait out a 120s read, took ${elapsed}s"
pass "a stalled pr-state read cannot exceed the deadline"

d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
cat > "$d/gh" <<'HANGGH'
#!/usr/bin/env bash
exec sleep 120
HANGGH
chmod +x "$d/gh"
start=$SECONDS
out="$(PR_AWAIT_READ_FLOOR=2 run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet 2>/dev/null)" && rc=0 || rc=$?
elapsed=$((SECONDS - start))
[[ "$rc" -ne 0 ]] || fail "a stalled gh pr view must never exit 0, got $rc" "$out"
[[ "$elapsed" -lt 12 ]] || fail "a zero deadline must not wait out a 120s gh read, took ${elapsed}s"
pass "a stalled gh pr view read cannot exceed the deadline"
grep -qi 'exceeded its read bound' <<<"$out" \
  || fail "a timed-out gh pr view must say so, not report a bare failure count" "$out"
grep -q 'failed 0 times' <<<"$out" \
  && fail "a pure gh pr view timeout must not be reported as 'failed 0 times in a row'" "$out"
pass "a timed-out gh pr view is reported as a timeout, not a miscounted failure"

# Structural guard: both external reads must go through the bounded helper, or
# the deadline is only enforced between reads and not during them.
grep -q 'run_bounded_capture "\$budget" "\$READ_TMP" "\$PR_STATE"' "$ROOT_DIR/scripts/pr-await" \
  || fail "the pr-state read must run through run_bounded_capture"
grep -A2 'run_bounded_capture "\$budget" "\$READ_TMP"' "$ROOT_DIR/scripts/pr-await" \
  | grep -q '"\$GH" pr view' \
  || fail "the gh pr view read must run through run_bounded_capture"
pass "every external GitHub read is bounded by the remaining deadline"

# Structural guard: a failed read must be validated in a local before it can
# replace SUMMARY_JSON/MERGE_JSON. Otherwise a transient failure right before
# the deadline destroys evidence a working poll already produced, and the
# deadline-clock check ("if -n SUMMARY_JSON: OUTCOME=deadline") wrongly trusts
# garbage instead of falling back to the last good snapshot.
grep -q 'summary_body="\$(cat "\$READ_TMP")"' "$ROOT_DIR/scripts/pr-await" \
  || fail "the pr-state read must stage into a local before committing to SUMMARY_JSON"
grep -q 'merge_body="\$(cat "\$READ_TMP")"' "$ROOT_DIR/scripts/pr-await" \
  || fail "the gh pr view read must stage into a local before committing to MERGE_JSON"
pass "a failed GitHub read is staged before it can clobber the last good snapshot"



# --- a head change voids the snapshot in hand ------------------------------
# Otherwise a deadline reached before the next read renders the old head's
# checks under the new SHA.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 3 2 7 true aaaaaaaaaaaa
snapshot "$d" 2 3 2 7 true aaaaaaaaaaaa
cat > "$d/gh" <<'GHMOVED'
#!/usr/bin/env bash
printf '{"state":"OPEN","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","headRefOid":"bbbbbbbbbbbb"}'
GHMOVED
chmod +x "$d/gh"
out="$(run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 3 ]] || fail "a deadline with only stale evidence must block, got $rc" "$out"
grep -q 'Failing' <<<"$out" && fail "must not report the old head's checks under the new SHA" "$out"
pass "a head change voids the snapshot, so a deadline cannot report stale checks"

# --- a timed-out read takes its children with it ---------------------------
# On timeout-less hosts, killing only pr-state's shell leaves its gh child
# running and still issuing requests.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
cat > "$d/pr-state" <<'SPAWN'
#!/usr/bin/env bash
sleep 300 &
printf '%s' "$!" > "$FAKE_DIR/childpid"
wait
SPAWN
chmod +x "$d/pr-state"
PR_AWAIT_NO_TIMEOUT=1 PR_AWAIT_READ_FLOOR=2 run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet >/dev/null 2>&1 || true
child="$(cat "$d/childpid" 2>/dev/null || echo '')"
[[ -n "$child" ]] || fail "the fake did not record its child pid"
sleep 1
if kill -0 "$child" 2>/dev/null; then
  kill -9 "$child" 2>/dev/null || true
  fail "a timed-out read left its child process running (pid $child)"
fi
pass "a timed-out read terminates the whole process group, not just the shell"

# --- cancellation stops an active bounded read -----------------------------
# A caller that cancels the waiter must not leave a child GitHub read alive.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
cat > "$d/pr-state" <<'CANCEL'
#!/usr/bin/env bash
sleep 120 &
printf '%s' "$!" > "$FAKE_DIR/childpid"
wait "$!"
CANCEL
chmod +x "$d/pr-state"
FAKE_DIR="$d" PR_AWAIT_PR_STATE="$d/pr-state" PR_AWAIT_GH="$d/gh" \
  PR_AWAIT_SLEEP="$d/sleep" PR_AWAIT_JQ="$d/jq-ok" PR_AWAIT_PROBE_BASH="$d/bash-ok" \
  PR_AWAIT_NO_TIMEOUT=1 PR_AWAIT_READ_FLOOR=30 "$SCRIPT" 12 --interval-sec 1 --deadline-min 30 --quiet >"$d/out" 2>"$d/err" &
await_pid=$!
for _ in 1 2 3 4 5; do
  [[ -s "$d/childpid" ]] && break
  /bin/sleep 1
done
[[ -s "$d/childpid" ]] || { kill -KILL "$await_pid" 2>/dev/null || true; fail "the fake did not start a bounded child read"; }
child="$(<"$d/childpid")"
kill -TERM "$await_pid"
for _ in 1 2 3 4 5; do
  kill -0 "$await_pid" 2>/dev/null || break
  /bin/sleep 1
done
if kill -0 "$await_pid" 2>/dev/null; then
  kill -KILL "$await_pid" 2>/dev/null || true
  kill -KILL "$child" 2>/dev/null || true
  wait "$await_pid" 2>/dev/null || true
  fail "SIGTERM must stop an active bounded read"
fi
wait "$await_pid" && rc=0 || rc=$?
[[ "$rc" -eq 143 ]] || fail "SIGTERM should return 143, got $rc" "$(<"$d/err")"
if kill -0 "$child" 2>/dev/null; then
  kill -KILL "$child" 2>/dev/null || true
  fail "SIGTERM left the bounded child read running (pid $child)"
fi
pass "SIGTERM stops the waiter and its active bounded read"


# --- a spent deadline grants one read floor, not one per query -------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
# pr-state SUCCEEDS but eats most of the floor; gh then stalls. A shared budget
# leaves gh the remainder; a fresh grant hands it a whole second floor.
cat > "$d/pr-state" <<'SLOWPS'
#!/usr/bin/env bash
sleep 3
cat "$FAKE_DIR/seq/1.json"
SLOWPS
cat > "$d/gh" <<'SLOWGH'
#!/usr/bin/env bash
exec sleep 120
SLOWGH
chmod +x "$d/pr-state" "$d/gh"
start=$SECONDS
PR_AWAIT_READ_FLOOR=4 run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet >/dev/null 2>&1 || true
elapsed=$((SECONDS - start))
[[ "$elapsed" -lt 6 ]] || fail "a zero deadline granted a second read floor, took ${elapsed}s (budget was 4)"
pass "a spent deadline grants a single read floor shared across both queries"

# --- every --format json exit path emits parseable JSON --------------------
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0
cat > "$d/gh" <<'GARBAGE'
#!/usr/bin/env bash
printf 'not json at all'
GARBAGE
chmod +x "$d/gh"
out="$(PR_AWAIT_READ_FLOOR=2 run_await "$d" 12 --interval-sec 1 --deadline-min 0 --quiet --format json 2>/dev/null)" || true
jq -e . >/dev/null 2>&1 <<<"$out" || fail "a garbage merge-state read must still yield parseable JSON" "$out"
pass "a garbage merge-state read still yields parseable JSON"

# --- a comment-only finding gets an accurate NEXT message -------------------
# Zero failed checks and zero unresolved threads must not be reported as
# "fix 0 failed check(s) and 0 thread(s)" when an actionable issue comment
# (or a blocking review, conflict, or advanced base) is the actual finding.
d="$(make_tmp_dir)"; setup_fake "$d"
snapshot "$d" 1 20 0 0 true aaaaaaaaaaaa '{"actionable_issue_comment_count":1}'
out="$(run_await "$d" 12 --interval-sec 1 --quiet 2>/dev/null)" && rc=0 || rc=$?
[[ "$rc" -eq 1 ]] || fail "an actionable issue comment with clean checks should exit 1, got $rc" "$out"
grep -q 'fix 0 failed check(s) and 0 thread(s)' <<<"$out" \
  && fail "must not claim nothing to fix when a comment is the only finding" "$out"
grep -qi 'review the findings reported above' <<<"$out" \
  || fail "should point the caller at the reported findings instead" "$out"
pass "a comment-only finding is not reported as '0 failed check(s) and 0 thread(s)'"

printf '\nall tests passed\n'
