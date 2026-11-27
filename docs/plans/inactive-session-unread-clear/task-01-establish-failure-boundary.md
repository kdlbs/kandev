---
id: "01-establish-failure-boundary"
title: "Establish the inactive-session failure boundary"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
acceptance_criteria:
  - AC-TASKS-TASK-UNREAD-DOT-001.2
  - AC-TASKS-TASK-UNREAD-DOT-001.4
  - AC-TASKS-TASK-UNREAD-DOT-001.6
  - AC-TASKS-TASK-UNREAD-DOT-001.9
  - AC-TASKS-TASK-UNREAD-DOT-001.12
  - AC-TASKS-TASK-UNREAD-DOT-001.17
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
---

# Task 01: Establish the inactive-session failure boundary

## Summary

Reproduce reading a cursorless failed session without a live execution, using
the existing desktop and phone session selectors. The prior DB evidence proves
an unread sibling, not a failure after that sibling was viewed. Identify the
first boundary that violates the active read contract before production edits.

## In scope

- After an explicit implementation request, mark this order `in_progress`.
  Read `/debug`, `/tdd`, `/e2e`, `/playwright-cli`, `/mobile-parity`, and scoped
  web guidance before executing the scenario.
- Use an isolated E2E backend and disposable data. Never open the user's live
  failed session as an experiment: viewing it can mutate its read cursor.
- Extend the existing unread-dot fixtures/specs with a read primary and a
  cursorless FAILED sibling with no execution. Seed agent boot/status/error
  rows representative of the evidence; record the actual newest acknowledgeable
  message ID rather than assuming the last rendered row is the target.
- Proposed regression title in both existing specs:
  `clears a cursorless failed sibling only after its transcript is read`.
  First show the primary and prove its failed sibling remains unread; then
  explicitly select/reopen the failed session and observe the complete
  acknowledgement path. Do not issue mark-read from the fixture.
- Inspect the real selected session identity, loaded raw tail, page/panel
  visibility, route hydration, initial history readiness, mark-read request and
  response, persisted cursor, and authoritative unread list. Reuse existing
  `messages:read-tracking` diagnostics; do not add telemetry to production.
- Record exact expected/actual behavior and the first failed transition in
  Results. A selector timeout is not proof of a read-cursor bug unless it
  demonstrates that the existing UI cannot open the stored session.
- Preserve mixed-session semantics. No running agent does not imply read; an
  inactive sibling deliberately remains unread until its transcript is viewed.

## Out of scope

- Any production changes, live-instance mutation, task-wide mark-read behavior,
  lifecycle-based unread filtering, or agent restart.
- Adding tests that assert copied fields, debug wording, or a mocked echo.
- Claiming a root cause from a missing cursor alone.

## Acceptance

1. A disposable desktop/phone scenario distinguishes an intentionally unread
   failed sibling from a session that remains unread after explicit reading;
   the run records selected session, cursor, and unread-list evidence.
2. If a violation is reproduced, a deterministic RED assertion identifies the
   first failing boundary and its owning source file. Update Task 02's concrete
   scope, regression command, and Results input before unblocking it.
3. If the whole scenario passes, record conformance and keep Task 02 blocked.
   Remove experimental test additions that do not expose a new consumer-visible
   boundary; do not invent a fix or change the requirement to manufacture RED.

## ASCII UI preview

### UI-01: Desktop inactive-session read transition

See the [combined preview](plan.md#ascii-ui-preview).

```text
Primary displayed: task (*) | [#1 primary] [#2 failed] [+ reopen]
Select/reopen #2:   task     | [#2 failed SELECTED] stored output
```

### UI-02: Phone inactive-session read transition

```text
Task picker: task (*) -> [Session #1 v] -> existing Sessions sheet
Tap #2 Failed -> sheet closes -> [Session #2 v] -> stored output
Reopen task picker: task has no dot only when no unread sibling remains
```

This is the expected .6/.9/.12/.17 outcome, not a claim that the after-reading
failure has already been observed. Preserve the existing controls, row height,
focus, transcript scroll owner, and phone surface; no new product copy.

## Verification

Run from the task repository root. The following creates a disposable cache-backed
copy of the current working tree, including untracked tests and deletions. It does
not test committed HEAD alone. See [snapshot rules](plan.md#verification): refresh
after every source edit; apply formatting corrections back with normal edits,
not generated artifacts. Stop on skipped required source symlinks or cache-full
conditions. All checks below run inside the snapshot in the same Bash shell.

```bash
set -euo pipefail
SOURCE=$(git rev-parse --show-toplevel)
SNAPSHOT=$(mktemp -d "${TMPDIR:-/tmp}/unread-verification.XXXXXX")
VERIFY="$SNAPSHOT/repo"
git clone --no-hardlinks "$SOURCE" "$VERIFY"
rsync -a --safe-links --delete --exclude='/.git' --exclude='node_modules/' \
  --exclude='/apps/web/dist/' --exclude='/apps/backend/bin/' \
  --exclude='/apps/backend/.build/' --exclude='/apps/web/e2e/test-results*/' \
  --exclude='/apps/web/e2e/blob-report/' --exclude='/apps/web/e2e/playwright-report/' \
  --exclude='/apps/web/.pr-assets/' "$SOURCE/" "$VERIFY/"
export TMPDIR="$SNAPSHOT/tmp" GOTMPDIR="$SNAPSHOT/tmp"
export XDG_CACHE_HOME="$SNAPSHOT/cache" npm_config_cache="$SNAPSHOT/npm"
export GOCACHE="$SNAPSHOT/go-build" GOMODCACHE="$SNAPSHOT/go-mod" GOPATH="$SNAPSHOT/go"
export CARGO_HOME="$SNAPSHOT/cargo" CARGO_TARGET_DIR="$SNAPSHOT/target"
export PYTHONDONTWRITEBYTECODE=1
mkdir -p "$TMPDIR" "$XDG_CACHE_HOME" "$npm_config_cache" "$GOCACHE" "$GOMODCACHE" "$GOPATH" "$CARGO_HOME" "$CARGO_TARGET_DIR"
cache_only() {
  local p
  p=$(realpath -m "$1")
  while [[ ! -e "$p" ]]; do p=$(dirname "$p"); done
  [[ -e "$p" ]] || return 1
}
for p in "$VERIFY" "$VERIFY/apps/node_modules" "$VERIFY/apps/web/dist" \
  "$VERIFY/apps/backend/bin" "$VERIFY/apps/backend/.build" \
  "$VERIFY/apps/backend/cmd/plugin-fixture/fixture-package/ui" \
  "$VERIFY/apps/web/e2e/test-results" "$VERIFY/apps/web/e2e/blob-report" \
  "$VERIFY/apps/web/e2e/playwright-report" "$VERIFY/apps/web/.pr-assets" \
  "$TMPDIR" "$XDG_CACHE_HOME" "$npm_config_cache" "$GOCACHE" "$GOMODCACHE" \
  "$GOPATH" "$CARGO_HOME" "$CARGO_TARGET_DIR" "$PLAYWRIGHT_BROWSERS_PATH"; do cache_only "$p"; done
cd "$VERIFY"
(cd apps && pnpm install --frozen-lockfile --store-dir "$SNAPSHOT/pnpm-store")
desktop_rc=0
if (cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts -- --grep "clears a cursorless failed sibling only after its transcript is read") >"$SNAPSHOT/desktop.log" 2>&1; then
  desktop_rc=0
else
  desktop_rc=$?
fi
mobile_rc=0
if (cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts -- --grep "clears a cursorless failed sibling only after its transcript is read") >"$SNAPSHOT/mobile.log" 2>&1; then
  mobile_rc=0
else
  mobile_rc=$?
fi
cat "$SNAPSHOT/desktop.log" "$SNAPSHOT/mobile.log"
printf 'desktop_exit=%s mobile_exit=%s\n' "$desktop_rc" "$mobile_rc" | tee "$SNAPSHOT/diagnostic-status.txt"
# Collect both outcomes, but do not convert a failed diagnostic into a pass.
(exit "$((desktop_rc != 0 || mobile_rc != 0))")
```

Keep `--host --shards 1`: multi-shard logs and runner cleanup use hardcoded
`/tmp` paths. Do not invoke the runner's `clean` command.

Both commands build fresh production assets. Setup/install commands remain
fail-fast; only the two diagnostic invocations capture failures to allow both
viewports to run. Original statuses and full logs remain under `$SNAPSHOT`;
the final aggregate status is nonzero if either invocation failed. Collection
is not acceptance: inspect each log and record behavioral RED versus runner,
fixture, storage, or selector failure separately before completing Task 01.
Confirm discovery includes the intended case in both projects. Exercise the
same selection in a real browser on the disposable instance and inspect the
phone rendering; store scratch captures on the cache drive.

## Files likely touched

Test-only changes, after the later implementation request:

- `apps/web/e2e/helpers/unread-divider.ts`
- `apps/web/e2e/tests/task/task-unread-dot.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-unread-dot.spec.ts`
- This order's Results and `task-02-repair-read-path.md` evidence/scope.

Read-only trace inputs:

- `apps/web/components/task/session-reopen-menu.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/chat/use-session-read-tracking.ts`
- `apps/web/components/task/chat/session-read-ownership.ts`
- `apps/web/components/task/task-route-session-hydration.tsx`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/lib/session-unread-divider.ts`

## Dependencies

None. Existing requirements/design are the input contract. Task 02 depends on
this order producing a demonstrated violation, not merely on running a test.

## Risks

- Existing agent-less WAITING_FOR_INPUT coverage can pass while a failed
  sibling's selection or hydration path differs.
- Hiding the selected session's dot can conceal a failed persisted write.
- A mock-runtime fixture must not silently create an execution for the failed
  session; inspect the seeded session and observed launch traffic/state.
- This order may establish that the supplied shape conforms to requirements.
  In that case the missing prerequisite is reproduction evidence, not permission
  to suppress unread siblings.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/task-unread-dot.md), .2/.4/.6/.9/.12/.17.
- [Design](../../specs/tasks/system-design/task-unread-dot.md#inactive-session-acknowledgement).
- [Plan evidence and confidence](plan.md#evidence-and-confidence).
- Existing `seedUnreadDotTasks`, `SessionPage`, and causal WebSocket helpers.

## Results

Initial diagnostic checkpoint: done, contract conformance. At that checkpoint,
no violation of AC .6 was reproduced on desktop or phone; Task 02 was blocked
and no production file had changed. The later failed-resume focus-loss
reproduction unblocked the repair, recorded in
[Task 02's causal reproduction and results](task-02-repair-read-path.md#results).

### Disposable desktop/phone scenario

`seedFailedSiblingTask` (`apps/web/e2e/helpers/unread-divider.ts`) seeds the
reported shape on the isolated E2E backend: a read `WAITING_FOR_INPUT` primary,
a cursorless `FAILED` sibling (completed, no execution) with stored user
prompt, `agent_boot` `script_execution`, `New session started` status, and
`Agent produced no output since start.` error rows, an unread `RUNNING` sibling,
and a separate navigation task. Both specs gained
`clears a cursorless failed sibling only after its transcript is read`. Each
proves, against persisted state rather than the selected row:

- Opening the task shows the primary. Desktop hides the unread failed tab
  through the shipped close button without activating it, then verifies its
  null cursor and unread contribution. Reload preserves unread state; because
  these agent-less seeds have no environment, hide state is visit-local and the
  inactive tab rematerializes. Hide it again in the new visit before reopening.
  A 1.5 s negative-assertion dwell confirms unread remains `[failed, live]`.
- Reopening the hidden failed tab through the desktop `+` menu, or selecting it
  through the phone picker, renders the stored error and persists its tail.
  Only desktop claims the hidden-unread boundary (.12), not environment-backed
  hide persistence (covered separately by session-tab-management E2E).
- The page sent a mark-read request for the failed session with that ID and
  none for the live sibling. The live cursor stays `null`, and the primary
  cursor is unchanged. No `session.launch`, `session.ensure`, `session.recover`,
  or `task.launch.recover` frame was sent; the session stays `FAILED` with no
  `agent_execution_id`.
- After navigating to another task and reloading, the task keeps its dot only
  because the live sibling is unread. Reading that sibling (desktop reopen
  menu `reopen-session-<id>`; phone picker) empties the list, and the dot
  stays absent after leaving and reloading. Phone has no horizontal overflow.

### Commands and outcomes

Snapshot verification created from the current working tree with the documented clone/rsync steps.
Environment notes:

- `pnpm install --frozen-lockfile` used an isolated store for verification.
- Go and Node runtimes verified clean builds and tests.

Results:

- Playwright `--list`: `chromium` and `mobile-chrome` each discover the new case
  (2 tests per file).
- Diagnostic block, first attempt: `desktop_exit=1 mobile_exit=1`. Both stopped
  at `build-agentctl` because the Go modules could not be downloaded. This was a
  runner/storage failure, not a behavioral RED result.
- Diagnostic block, rerun with the offline module proxy:
  `desktop_exit=0 mobile_exit=0` (`1 passed (46.3s)`, `1 passed (36.5s)`).
- Snapshot-only mutation check: an added `state === "FAILED"` early return in
  the mark-read effect of `use-session-read-tracking.ts` made both cases fail.
  The failure was at the persisted-cursor poll (`Expected: <error row id>`,
  `Received: null`; desktop spec line 165, phone line 128). Exit status was 1
  for both. One phone retry also hit a backend port collision and restart that
  returned a 404. That was retry noise. The mutation was discarded by
  refreshing the snapshot. The cases therefore detect lifecycle-based read
  suppression, which the existing single-session `WAITING_FOR_INPUT` coverage
  cannot.

### Browser smoke

A throwaway, snapshot-only hold spec seeded the same shape on a disposable E2E
backend. It was driven in a managed headless Chromium, not the user's browser,
and the instance was torn down afterward. Captures are under
`<snapshot>/smoke/`.

- Desktop (1440x900): primary `#3` was shown, and the sibling dot was visible.
  Clicking tab `#1` showed `Agent produced no output since start.` with the
  existing `Session recovery failed` card. Nothing resumed. The cursor became
  the error row, and unread became `[live]`. After moving to another task and
  reloading, the dot remained (live unread). After reopening the live session
  from the `+` menu, unread became `[]`, and after leaving and reloading there
  was no dot.
- Phone (Pixel 5 emulation): the Sessions sheet listed `#1 Failed`,
  `#2 Running`, and `#3 Waiting for input`. Tapping `#1` closed the sheet and
  showed the stored transcript and recovery card in one scroll owner. The
  cursor and unread results matched desktop, and there was no horizontal
  overflow. The task switcher showed the dot after reload while the live
  sibling was unread and none after it was read. The untouched desktop-smoke
  task kept its own dot until it was read separately.

### Reported task (read-only follow-up)

The evidence was read without writing through `mode=ro` and `immutable=1`
queries of the live DB and the two retained backups. The parent report was
created at `2026-11-07T12:55:46Z`. Session `6ecceca2-…` now has
`last_read_message_id = 9f1ca37a-…`, which is its newest row (the `error` row).
The row's `updated_at` is `2026-11-07T16:23:25Z`. The only cursor writers are
the monotonic mark-read repository update and the E2E force-set route, and both
set `updated_at`. Backups from `2026-11-10` and `2026-11-15` hold the same cursor
and an empty `unread_session_ids` (summary revision 2006). The current summary
(revision 2147) is also empty, with `has_running_session: false`. The failed
sibling was therefore acknowledged about 3.5 hours after the report and has
stayed read. Backend logs from that day have rotated, so the trigger cannot be
attributed [INFERENCE: the user viewed it]. This is consistent with the
conformance above.

### Historical prerequisite for Task 02

At the initial checkpoint, explicitly viewing a stored session did not leave
its own cursor or summary contribution unacknowledged. That case still conforms.
The later incident exposed automatic focus loss before history loaded, not a
failure after explicit viewing. See [Task 02](task-02-repair-read-path.md#results)
for the causal reproduction, production correction, and independent review loop.
