---
created: 2026-11-16
status: done
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
legacy_specs: []
---

# Implementation Plan: Clear unread state when reading inactive sessions

## Overview

Preserve per-session read semantics and make reading stored output independent
of a live agent. Read an inactive session through the existing transcript and
acknowledge its loaded tail without starting, resuming, or restarting it.
Reading the primary session alone must not acknowledge an unread sibling.

The requirements source is the existing active
[task unread dot requirement](../../specs/tasks/requirements/task-unread-dot.md),
especially AC-TASKS-TASK-UNREAD-DOT-001.2, .4, .6, .8, .9, .12, and .17.
No requirement rewrite or new ID is needed: .6 already applies to every session,
and .9/.17 explicitly preserve unread siblings. Tasks owns the package because
it owns the task's unread aggregate; Office's
[read cursor design](../../specs/office/system-design/unread-divider.md)
remains the acknowledgement contract.

Task 01 found that explicitly reading stored failed output conforms. The later
reported failed-resume incident exposed a different boundary: automatic session
handoff hid the failed transcript before history recovery loaded its new output.
Task 02 repairs both automatic selection paths, retaining the failed view and
its recovery controls while normal acknowledgement catches up. Unread scope,
backend primary promotion, and completed/cancelled handoff remain unchanged.

Automatic lifecycle and primary-follow decisions read authoritative predecessor
state before changing focus, even when a terminal snapshot or unseen sibling
beats promotion. Accepted primary projections retarget the owned decision.
Committed removal also triggers scoped membership proof without a primary change.
A complete list with exactly one replacement primary permits removal handoff;
generic errors do not. Confirmed eviction fences older membership publications.
Superseded reads cannot overwrite cursors, displace a newer pin, or select an
older target. Explicit workflow focus retains its committed-recipient path.

## Evidence and confidence

Prior read-only inspection of reported task
`7b7c443a-f20a-40a5-ae4e-455022afef50` found:

| Session | Persisted state | Read evidence |
| --- | --- | --- |
| `411dd59f-8fa3-4ea2-a8b7-2fb70937bdd0` | WAITING_FOR_INPUT | Primary; cursor equals newest message. |
| `820cbaf7-10a2-45de-95f5-644835ecf398` | FAILED | Cursor equals newest row. |
| `6ecceca2-ad50-494b-96c3-37b7dbc717ca` | FAILED | No cursor; stored agent output includes boot, status, and error rows. |

The persisted task summary had `has_running_session: false` and exactly
`["6ecceca2-ad50-494b-96c3-37b7dbc717ca"]` in `unread_session_ids`.
This proves why the task is unread. A missing cursor does not prove whether
the user previously opened that transcript. It does not establish a defective
write, a visibility bug, or a dependence on agent execution.

Source confirms that `useSessionReadTracking` has no agent-liveness guard.
It requires visible ownership, route/session hydration, initial-message
readiness, and a loaded acknowledgement target. The desktop reopen menu lists
failed sessions without a lifecycle filter. The hook's default unit-test
fixture is RUNNING; existing unread-dot E2E helpers already seed agent-less
WAITING_FOR_INPUT sessions. Therefore adding only a lifecycle parameter to a
mocked hook test is not a sufficient failing reproduction.

The earlier frontend diagnostic bundle was partial; it is not UI reproduction
evidence. No live database or task state was changed during diagnosis.

Follow-up (Task 01, read-only): that cursorless snapshot predates
`2026-11-07T16:23:25Z`, when `6ecceca2-…` was acknowledged up to its newest row.
Its cursor and the empty summary have persisted through the 2026-11-10 and
2026-11-15 backups to the current database. See
[Task 01 Results](task-01-establish-failure-boundary.md#results).

## Scope

### In scope

- Trace the failed sibling's real selection, transcript load, acknowledgement,
  persisted cursor, projected summary, and subsequent navigation/reload.
- Reproduce on disposable tasks with the same mixed-session shape, including
  no runtime/execution backing the failed session.
- Repair the first demonstrated broken boundary in the existing path.
- Desktop and phone proof that reading the inactive session clears its own
  contribution durably, while unread siblings remain unread.
- Preserve cursor monotonicity, response correlation, divider first-visit
  behavior, visibility guards, message eligibility, and presentation-only delay.

### Out of scope

- Clearing all sessions when a task is selected; ignoring stopped or failed
  sessions; rewriting production cursors; deleting unread history.
- New mark-all-read controls, per-session dots, counters, settings, or copy.
- Changes to agent launch, execution recovery, session deletion, or Quick Chat.
- New schema, endpoint, flag, metrics, polling, retry policy, or subscriptions.
- Treating a task with only read primary output and unread sibling output as
  a product defect merely because the sibling's agent is stopped.

## Technical approach

### Boundary and state flow

1. Select the failed session explicitly. Desktop uses a session tab or
   `SessionReopenMenuItems`; phone uses `MobileSessionsPicker`. A hidden tab is
   reopened, not deleted. Selection must not launch an agent.
2. Load its persisted session and message history through the existing session
   hooks. `useSessionMessages` exposes `isInitialMessagesLoading`; runtime
   availability must not substitute for history readiness.
3. `TaskChatPanel` derives `newestAcknowledgeableMessageId(messages)` from the
   raw loaded rows, not just rendered items. `isSessionReadableInPanel` proves
   ownership of the shown task; route hydration proves the selected identity.
4. `useSessionReadTracking` acknowledges once the existing visibility and
   readiness conditions are met. It keeps the cursorless first visit divider-
   ineligible, but still calls `markSessionRead` when either unread setting is
   enabled. Retain per-session request generations and stale-response guards.
5. The existing monotonic mark-read write emits
   `TaskSessionReadCursorChanged`; the projector removes that session from
   `unread_session_ids`. No frontend-only summary clearing is permitted.
6. Navigate to another task, then return/reload. The persisted cursor and
   authoritative summary must retain the read state without a new agent event.

### Diagnosis gate

Task 01 records the first failing boundary and an exact reproduction in its
Results. Distinguish these outcomes before changing code:

| Observed result | Required disposition |
| --- | --- |
| Failed sibling reads correctly when explicitly selected | No production fix is justified by the supplied task. Record contract conformance; keep Task 02 blocked pending evidence of the reported failure. |
| Session cannot be selected or history never becomes ready without a runtime | Record the precise owner/guard that waits for runtime state. Repair only that selected stored-session path. |
| Correct loaded session is visible but no mark-read is dispatched | Record the incorrect visibility, route identity, or readiness transition; cover that transition, not a hypothetical lifecycle check. |
| Mark-read fails or cursor/summary remains stale after success | Record the HTTP/service/projector failure and applicable boundary. Do not conceal it with a display-rule change. Reconcile the work order's exact owned files before implementation. |

No branch above permits inventing a root cause. A new persistence, API, or
product contract requires revising this package before implementation.

### Runtime compatibility

| Shape | Intended outcome | Evidence |
| --- | --- | --- |
| Stored FAILED session, no execution, cursorless | View stored output and persist its loaded tail without launching. | Primary defect-specific desktop and phone scenario. |
| COMPLETED/CANCELLED/IDLE/WAITING_FOR_INPUT, no live execution | Same read boundary, no lifecycle exemption. | Exercise the state implicated by Task 01; existing agent-less waiting-session E2E remains a baseline. |
| Read primary plus unread failed sibling | Reading primary leaves sibling unread; reading failed sibling removes only its ID. | Mixed-session UI flow plus cursor/summary assertions. |
| Another unread sibling or a live sibling | Reading failed session must not clear the other contribution or change its execution. | Mixed-state regression. |
| Hidden tab, visible reopen menu | Hide unread failed tab without activation, verify its null cursor/contribution across reload, hide again in the new visit, then explicitly reopen and acknowledge. | Desktop regression; these environment-less seeds have visit-local hiding. Environment-backed hide persistence is owned by session-tab-management E2E; phone picker covers selection only. |
| Missing/deleted session or failed history read | No invented tail, no mark-read, preserve other unread contributions. | Existing error guards plus the defect's targeted regression where applicable. |

## ASCII UI preview

### UI-01: Desktop inactive-session read transition

Entry: existing task sidebar and Dockview chat. Dot enabled, divider disabled,
hide delay zero for the regression. The before state below is supported by the
stored unread summary and source display rule. This explicit stored-session
case now conforms; the reproduced resume-failure focus loss is shown below.

```text
BEFORE: primary displayed; failed sibling unread
+----------------------+-----------------------------------------+
| Tasks                | [#1 primary] [#2 failed] [+ reopen]      |
| > Reported task  (*) | #1: previously read output               |
+----------------------+-----------------------------------------+

EXPECTED: explicitly open #2 and load its stored transcript
+----------------------+-----------------------------------------+
| Tasks                | [#1 primary] [#2 failed SELECTED] [+]     |
| > Reported task      | #2: stored failure output                |
|                      | Agent remains stopped. No resume needed. |
+----------------------+-----------------------------------------+
```

`(*)` represents the existing localized unread indicator, not a new badge.
After persisted acknowledgement, returning to #1 or another task must not bring
it back. If any other session is unread, the dot remains. Preserve existing
row height, error indicator, tab ordering, focus, and transcript scroll owner.

The reproduced resume-failure variant retains the same selected tab:

```text
Before: #1 Starting -> #1 Failed -> #2 Waiting (failure hidden, #1 unread)
After:  #1 Starting -> #1 Failed [still selected]
        history loads -> failure + recovery actions visible -> #1 read
```

The backend may promote #2 to primary without dismissing #1. Explicit session
selection remains available; completed/cancelled handoff is unchanged.

### UI-02: Phone inactive-session read transition

Entry: the existing task picker leads to the task; its existing session pill
opens `MobilePickerSheet`. Use that focused picker, not desktop Dockview.

```text
[Task picker: Reported task (*)]
               | open task
[Session: #1 v] | #1 transcript
               | tap existing session pill
+-----------------------------------+
| Sessions                          |
| #1 Primary                        |
| #2 Failed                  [tap]  |
+-----------------------------------+
               | select #2; picker closes
[Session: #2 v]
| Stored failure output             |
| Agent remains stopped.            |
[Chat] [existing navigation]

Reopen task picker: Reported task     (no dot if all sessions read)
```

The existing mobile picker/session transcript composition is retained. It uses
one transcript scroll owner and existing dynamic viewport, safe-area, and touch
handling; this repair adds no controls or geometry changes. The primary action
is tapping the existing failed-session row. Shared session/read state determines
both layouts. Check absence of document horizontal overflow and capture the
rendered phone view. ASCII labels and spacing are illustrative, not new copy.
Both previews map to .6/.9/.12/.14/.15/.16/.17.

For the failed-resume variant, keep the same phone chat selected:

```text
[Session #1 v] Starting -> Failed [same selected chat]
                         failure + recovery actions
                         history loads -> #1 read
```

The existing session picker remains the explicit way to choose a sibling.

## Tests

All abbreviated AC IDs below use prefix `AC-TASKS-TASK-UNREAD-DOT-001`.

| AC | Evidence to retain or add after diagnosis |
| --- | --- |
| .2, .4, .6 | Defect-specific transition in `apps/web/components/task/chat/use-session-read-tracking.test.ts` if the defect is in read ownership/readiness/correlation; use a persisted cursor assertion in E2E, not a mock echo alone. |
| .6 | `apps/web/lib/session-unread-divider.test.ts` retains raw-tail acknowledgement coverage. Add a case only if Task 01 proves a missing eligibility boundary. |
| .5, .9, .17 | Retain `task-unread-dot-visibility.test.ts`, particularly `keeps the open task's dot while another of its sessions is unread`. Do not re-pin its expectation to hide unread siblings. |
| .6, .8, .9 | Desktop and phone regression `clears a cursorless failed sibling only after its transcript is read` in the existing unread-dot spec files; final empty list is correlated with the remaining sibling's exact persisted tail before and after reload. |
| .12 | Desktop variant hides the unread failed tab, verifies its null cursor/unread contribution across reload, hides it again in the new environment-less visit, and reopens through the session menu. Phone picker selection does not claim desktop hidden-tab coverage. |
| .6, .8 | Failed-resume focus regressions in `agent-session.test.ts`, `tasks.test.ts`, and both unread-dot E2E specs reproduce a real resume failure with paged history and primary promotion; the failed view remains selected until its persisted tail is acknowledged. |

The first RED assertion must expose the actual failed boundary recorded by Task
01 or the later incident's causal reproduction. A test that is already green,
a missing selector, or an intentionally unread sibling is not RED. Never alter
production behavior to force a hypothetical lifecycle defect to exist.

## E2E tests

Extend `apps/web/e2e/tests/task/task-unread-dot.spec.ts` (`chromium`) and
`apps/web/e2e/tests/task/mobile-task-unread-dot.spec.ts` (`mobile-chrome`) with
the same causal scenario after Task 01 proves a violation:

- Seed a read primary and a cursorless FAILED sibling with representative stored
  boot/status/error output and no execution. Enable dot only; use zero hide delay.
- Open primary. Assert failed sibling still contributes, with unchanged cursor.
- Select failed sibling through the real desktop tab/reopen menu or phone session
  picker. Show its stored output; do not call mark-read directly from the test.
- Assert its persisted cursor equals the newest acknowledgeable row and it leaves
  the authoritative unread list. Assert no agent was started and lifecycle stayed
  FAILED. Navigate away and reload to prove the dot stays clear.
- Include another unread sibling (including a live sibling) and prove it is not
  acknowledged by reading the failed session. Preserve existing suite scenarios.
- Arm causal observers before actions; restoring modified user settings is
  mandatory because worker state is shared. Inspect the phone rendering.

## Work orders

Execute sequentially. No subagent authorization is implied.

- [x] [Task 01: Establish the inactive-session failure boundary](task-01-establish-failure-boundary.md) (done: conformance, no violation reproduced).
- [x] [Task 02: Repair the demonstrated inactive-session read path](task-02-repair-read-path.md) (done: R31 file admission and workflow/layout corrections verified. Independent review continues; broad suites/live NATS remain uncertified).

The prior [task unread dot package](../task-unread-dot/plan.md) is historical
completed delivery. The [visible-visit divider package](../unread-divider-visible-session-clear/plan.md)
owns a different visit-anchor defect. Neither package's recorded checks are
verification for this repair; retain their scopes and results unchanged.

## Verification

Use a fresh cache-backed copy of the **current working tree**, not committed
HEAD. Run this Bash setup and the checks below in the same shell. It clones Git
metadata for tools, then overlays tracked/untracked edits and deletions; existing
node_modules and regenerable outputs are excluded. Inspect any unsafe symlink
warnings from rsync and stop if a required source was skipped. Do not run builds
during design/review. Refresh this snapshot after every source/test edit.

All checks run inside the snapshot. Formatting changes stay there: apply any
required source corrections back through normal repository edits, then refresh
and reverify. Do not copy build products or dependencies back to the task tree.
Retain only results in the work orders. Do not delete the snapshot while a process
uses it. If cache space is insufficient, use the cache-full cleanup/alert procedure;
never choose another drive.


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
cd "$VERIFY"
(cd apps && pnpm install --frozen-lockfile --store-dir "$SNAPSHOT/pnpm-store")
(cd apps/web && pnpm exec vitest run components/task/chat/use-session-read-tracking.test.ts components/task/task-unread-dot-visibility.test.ts lib/session-unread-divider.test.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts)
make fmt
make typecheck test lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/inactive-session-unread-clear
```

Force host mode and one shard: the multi-shard runner hardcodes logs under
`/tmp`, while its single-shard branch writes directly to the console. Do not
use the runner's `clean` command, which also addresses those `/tmp` paths.

Managed E2E performs fresh builds; run desktop and mobile sequentially. Add the
exact narrow regression command for any newly implicated boundary to Task 02
before editing it. Browser smoke must exercise the selected stored transcript,
then navigate away and observe durable dot clearing; tests alone do not replace
that observation.

## Verification results

Design-package validation:

- `python3 scripts/list-docs.py validate`: passed (436 decisions, 1931 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs/tasks docs/plans/inactive-session-unread-clear`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual docs-only paths are
  exempt. A prospective graph check including the candidate read-tracking source
  path validates both work orders as covered, with zero reference errors. The
  candidate path was supplied only to exercise graph validation; it was not edited.
- Worktree status shows the modified system design and three new package files,
  unstaged and uncommitted. The active requirement is reused unchanged.

Product verification (implementation run, cache-backed snapshot): Task 01
reproduced the reported shape on desktop and phone and found conformance;
its Results record commands, statuses, a mutation check, and browser smoke.
The later failed-resume incident was reproduced with a real mock-agent resume
failure: focus moved from the failed session to its waiting sibling before its
failure output loaded, leaving it unread. Task 02 corrects the two selection
rules in `apps/web/lib/ws/handlers` and records RED/GREEN, E2E, and browser
smoke in its Results. Parent integration accepts retaining the failure view:
it preserves loaded-transcript acknowledgement (AC .6) and the visible recovery
error required by
[launch failure recovery AC .8](../../specs/tasks/requirements/task-launch-failure-recovery.md).
This is not task-wide read authorization or a change to workflow recipients.
The first production fix was pushed as `e4e01b8b35` after rebasing onto
`7087545475`. Its distinct failed-resume review loop is recorded below; earlier
design and diagnostic reviews did not cover that fix.
Final checks for the retained E2E regressions from the first run are below.

These checks ran after the last source edit in snapshot
`~/.cache/tmp/unread-verification.m9zRZH`, using the Task 01 environment
deviations. Host `KANDEV_*`/`AGENTCTL_*` variables were unset because they
leaked into config tests. The changed files are three E2E files and four docs,
with no Go or production web source:

- Focused `vitest run` (read tracking, dot visibility, divider): 3 files, 41
  tests passed.
- `pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts`:
  2 passed.
- `... --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts`: 2
  passed.
- `make fmt`: exit 0. The prettier wrapping of two new E2E lines was applied
  back by normal edits, and a second run was stable.
- `make typecheck`: exit 0 with `NODE_OPTIONS=--max-old-space-size=8192`. The
  default heap aborted with a `tsc` out-of-memory error. Ignored
  `apps/web/generated/*.json` was first regenerated, because snapshot `rsync
  --delete` removes it.
- `make test`: exit 2 in `test-backend`, in unchanged Go packages (examples:
  `task/handlers` and `task/service` report "parent directory cannot be
  accessed" under the cache-drive `TMPDIR`; `executor_ssh_dial_test`
  `bind: invalid argument` from an over-long unix socket path; `agentctl`
  process timeouts; `task/repository/sqlite` 10-minute timeout).
  `make test-web` was run separately: 38 failed files and 159 failed tests out
  of 30223, from browser-mode timeouts and `ECONNREFUSED 127.0.0.1:3000`. No
  vitest file imports the changed E2E files. `test-cli` passed. `test-scripts`
  failed because `cargo` is not installed.
- `make lint`: `lint-backend` failed on a pre-existing `goconst` finding in
  the unchanged
  `internal/agentctl/server/adapter/transport/acp/dialect_claude_prompt_suggestions.go:31`.
  `lint-web` (oxlint `--deny-warnings`), `lint-harness`, `lint-specs`, and
  `lint-architecture` passed.
- `python3 scripts/list-docs.py validate` (436 decisions, 1931 specifications),
  `lint-spec-files.test.py` (36 tests), `lint-spec-files.py --all`, and
  `git diff --check` (docs and `apps/web/e2e`): passed.

## Risks

- The supplied task currently demonstrates valid unread-sibling behavior, not a
  reproduced failure after reading the failed session. Task 01 can establish that
  no repair is justified for that shape; do not manufacture one.
- A selected row can hide a displayed session's dot before persistence succeeds.
  Cursor and post-navigation assertions are required to detect this false pass.
- Removing route/ownership guards can silently read a different task. Preserve
  them and fix only the verified readiness or identity transition.
- The original user might intend task-wide acknowledgement. That behavior was
  not selected and is explicitly excluded, rather than silently changing .9/.17.
- Repository build/E2E output defaults can target non-cache storage. Confirm and
  relocate outputs before execution; never fall back to the live instance DB.

## Adversarial review progress

- Round 1 (OMP Sol 6.1): one P2 finding, accepted and addressed. All three
  verification sections now include a cache-backed working-tree snapshot,
  isolated dependency/tool caches, and fail-closed output mount assertions.
  Host E2E is pinned to one shard to avoid hardcoded `/tmp` shard logs.
- Remediation checks: all three Bash blocks pass `bash -n`; read-only mount
  checks pass for every enumerated destination via its nearest existing
  ancestor. No snapshot, dependency installation, build, or product test ran.
  Specification catalog validation and whitespace checks passed.
- Round 2 (OMP Sol 6.1): one P2 finding, accepted and addressed. Task 01 now
  collects both desktop and phone diagnostic statuses/logs even when desktop
  reproduces RED, then returns a nonzero aggregate for either failure.
  Setup/install remain fail-fast; Task 02's GREEN checks remain fail-fast.
- Round 2 remediation smoke: documented Bash syntax passed. Throwaway shell
  control-flow simulations retained statuses for desktop failure/phone success
  (7/0), desktop success/phone failure (0/9), and both success (0/0). Both
  viewports ran in each case; setup failure aborted before either invocation.
  Failed diagnostics returned aggregate 1, both-success returned 0. No E2E,
  build, install, or product runtime was exercised by this shell-only check.
- Round 3 (OMP Sol 6.1): zero actionable findings. Both prior P2 corrections
  confirmed addressed; no product reproduction or implementation approval implied.
- Round 4 (OMP Sol 6.1): zero actionable findings; prior corrections independently
  confirmed addressed. No product checks or implementation approval implied.
- Round 5 (OMP Sol 6.1): zero actionable findings; both prior corrections remain
  resolved. No product reproduction or implementation approval implied.
- Completed rounds: 5. Consecutive clean rounds: 3 (rounds 3, 4, and 5).
  Review loop stopped at the requested three-consecutive-clean condition.
  At the design checkpoint Task 01 was pending. Its first implementation run
  found conformance; the later failed-resume reproduction and Task 02 repair
  are separate evidence, not findings certified by these design reviews.

## Historical diagnostic-only post-push review loop

This ledger covers the diagnostic E2E delivery, not the failed-resume production
fix. It is separate from the five design-only rounds above.

- Round 1 reviewed `81e7284b9885b5489ee23183d64923cf5847734c` over
  `75972b7e19fdaf80bedc3b76e20dfa25be7e19aa`: two P2 findings, accepted.
- Authoritative readback now requires exactly one task and a non-null summary;
  only omission of `unread_session_ids` normalizes to an empty list. Both
  viewport scenarios verify the remaining sibling's exact persisted tail
  immediately after reading and after navigation/reload.
- Desktop now actually hides the unread failed tab before reading, verifies its
  contribution across reload, hides the rematerialized tab in the new visit, and
  explicitly reopens it. Environment-less seeds do not persist hide state:
  `dockview-hidden-session-panels.ts` persists only with a layout environment.
  Environment-backed hide persistence remains separately exercised.
- Fault probes of the real helper: missing task, missing/null summary, and
  duplicate task reject; a present summary with omitted unread IDs returns `[]`.
  Before correction the first three missing-authority cases returned `[]`.
- Initial combined desktop run: 10 passed, 1 failed because the new test wrongly
  expected environment-less hide state to survive reload. The corrected test
  preserves the unread contract without claiming environment-backed persistence.
- Final commands (cache-backed snapshot, one worker, no retries):
  `pnpm e2e:run --host --shards 1 --no-build --project chromium tests/task/task-unread-dot.spec.ts tests/session/session-tab-management.spec.ts -- --retries=0`:
  11 passed.
  `pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts -- --retries=0`:
  2 passed. `--no-build` reused the fresh application/backend/fixture build from
  the preceding run; only E2E tests/helpers changed afterward.
- Rounds 2, 3, and 4 reviewed `b9494eca21`: no actionable findings in each.
  Completed diagnostic post-push rounds: 4. Consecutive clean: 3. Loop stopped.

## Failed-resume production-fix review loop

- Round 1: Sol child `1914c408-35bf-438f-b76e-c5fbf93c31a4` reviewed
  `e4e01b8b35` over `7087545475`. Two P2 findings (promotion-first delivery
  and incomplete E2E causal assertions) and one P3 finding (stale delivery
  status/diagnostic ledger), accepted for remediation.
- Round 2: Sol child `41d1583a-0bbc-4e5d-9d8b-b9865b4dd1f3` reviewed
  `460e55434d`. One P2 finding (hub/client-queue inversions survive callback
  FIFO) and one P3 finding (remaining stale plan/Task 01 status), accepted.
  R2 moves the focus decision to authoritative session readback and withdraws
  the insufficient transport guarantee; see Task 02 Results.
- Round 3: Sol child `e24cbb17-d8c5-45a1-bbb4-fc5aa9831963` reviewed
  `c3007ea060`. Two P2 findings (terminal list fallback defeats a pending primary
  follow; unrelated task deletion invalidates one) and one P3 finding (the
  runnable narrow command omits the deterministic regression), accepted.
  R3 coordinates accepted terminal notifications with the owned target and makes
  empty workflow cancellation a no-op; see Task 02 Results.
- Round 4: Sol child `354fcc20-781c-4a11-b795-11803504f45c` reviewed
  `4874a000ca`. Three P2 findings (old terminal snapshots bypass authority;
  another accepted primary strands an owned follow; removed predecessors stay
  selected after a 404), accepted and remediated. R4 withdraws the terminal
  shortcut, reconciles primary projections, and proves removal through scoped
  membership. Author checks: 804 focused tests, final authority replay 50,
  desktop selection/unread/removal 34 and phone unread 3; see Task 02 Results.
- Round 5: Sol child `4671b416-e21a-4eb8-885a-8e9fe1f1a459` reviewed
  `6b472c16f3`. Three P2 findings (lifecycle fallback before promotion bypasses
  authority; removing a retained non-primary failure has no core subscriber;
  stale membership reads recreate a proven-removed row), accepted and remediated.
  R5 verifies lifecycle focus before promotion, observes retained-session removal,
  and fences pending/admitted/dialog membership reads. Final author checks:
  1019 focused tests, 22 retirement-consumer tests, final desktop unread/removal
  5 and phone unread 3 passed; the earlier related desktop run passed 35.
  See Task 02 Results for exact commands, bundle provenance and limitations.
- Round 6: Sol child `fb78eae7-2e1c-4d70-9f55-b1bcf209a9a1` reviewed
  `70c33e2e0f`. Three P2 findings accepted: independent route/lifecycle admission
  resurrects removed rows; an older removal proof can select a second deleted
  recipient; delayed readiness inheritance overwrites recipient-owned execution.
  R6 adds confirmed-removal admission, membership-owned proof and recipient-owned
  readiness. Final author checks: 1620 focused tests, full desktop selection/
  unread/removal 35 and phone unread 3 passed. Task 02 records exact commands,
  frontend/backend provenance and limits. This review loop remains open.
- Round 7: Sol child `49b23364-853b-4497-98df-2186bbdf8ee2` reviewed
  `56850cf647`. Three P2 findings accepted: unchanged primary weakens lifecycle
  authority; a delayed recipient can fail before activation; remote retirement
  incorrectly requires a successor and retained view ownership. R7 remediation
  is implemented: 1628 focused tests, desktop 36 and phone 4 passed. Task 02
  records exact checks, assertion-mutation proof and runtime limits. The
  production review loop remains open.
- Round 8: Sol child `fd1d93e3-0a92-403e-b79e-d090dcb649b4` reviewed
  `3275ffafc4`. Four P2s and one P3 accepted: preserve independent workflow
  intent; continue an owned empty slot after promotion; exclude observed remote
  deletion before proof; reconsider an unhydrated projected recipient; rank
  eligible recipients by actual start time. R8 remediation is implemented:
  1644 affected tests and the final 16-case fixture run passed; 36 existing
  desktop cases plus the corrected new-case replay and 5 phone cases passed.
  Task 02 records intermediate failures, exact commands, captures and limits.
  The separate production review loop remains open.
- Round 9: Sol child `700e9f77-6a66-4c93-a172-fcdb55d2b276` reviewed
  `eb7ea5cb39`. Three P2s and one P3 accepted: older genuine promotions can
  displace a new STARTING attempt; observe-before-terminal loses a healthy
  handoff; selected local deletion guesses from stale membership; partial
  lifecycle rows lack durable chronology. R09 remediation is implemented,
  including existing-workflow precedence and preservation of an already selected
  chat across tab reordering. Final desktop 37/37 and phone 6/6 passed, with
  owning-file tests, typecheck and a fresh frontend. Task 02 distinguishes the
  intermediate fixture/browser failures from final targeted GREEN. The separate
  production review loop remains open.
- Round 10: Sol child `d8023e99-d6a5-4e41-bb03-83a68ac7dc1f` reviewed
  `32f12496a0`. Three P2s accepted: Preview deletion/display bypasses the shared
  selection owner; genuine retarget drops lifecycle recipient eligibility;
  an owned empty-slot handoff can leave a different Dockview sibling visible.
  Remediation is implemented and verified. Task 02 records 353 focused tests,
  ten new/owning Chromium cases, the wider desktop import repair, and six phone
  passes. Preview, lifecycle retarget and actual Dockview visibility each have
  consumer proof. The separate production review loop remains open.
- Round 11: Sol child `22791f6e-c198-4e89-a223-80669dc3b25e` reviewed
  `e09985a2fc`. Four source-derived P2s accepted for reproduction: an implicit
  Dockview fallback can acknowledge a sibling; promotion-first delivery omits
  lifecycle recipient eligibility; a late local ACK cancels the newly selected
  removed session's proof; stale Preview deep links can pin a missing session.
  Remediation is implemented and verified: 14 desktop and 6 phone cases,
  449 related cases plus 7 real Preview consumer/width controls, changed-file
  lint, and typecheck. After the normal hook's strict lint correction, 160
  affected cases and final-bundle desktop/phone failed-resume smokes passed.
  Task 02 distinguishes these from the earlier full 14/6 run and records
  intermediate fixture/hook failures. No unread contract changed.
- Round 12: Sol child `8a1943f9-ca64-4a60-87de-ab8d4c32ec48` reviewed
  `4a519db51b`. Five source-derived P2s accepted for controlled reproduction:
  direct remote removal misses the group lease; a hidden proven primary admits
  an unproven fallback; passive events replace a valid initial Preview URL;
  Preview acknowledges then loses workflow focus before membership; authoritative
  terminal reads lack recipient eligibility without a separate terminal notice.
  R12 remediation is implemented and verified: 373 targeted unit cases,
  18 removal/Preview desktop cases plus the three original desktop unread cases,
  and all six phone cases passed on the fresh frontend. Strict changed-file lint,
  typecheck, formatting and ratchets passed. Task 02 records RED, intermediate
  surface correction, exact commands, captures and retained-backend limits.
  Independent production review remains open; round 13 is next.
- Round 13: Sol child `a9fd7b4f-62fb-4826-9c58-67881212d9de` reviewed
  `c39bca5e2e`. Two source-derived P2s accepted for controlled reproduction:
  hidden proven recipients bypass reconciliation/recovery/input admission;
  phone route fallback can read a sibling during the owned empty removal slot.
  Remediation is implemented and verified, including the visual follow-through
  that hidden C must retain its task dot. Final-bundle desktop 22/22 and phone
  7/7 passed; all changed regressions plus adjacent dot consumers passed 47/47,
  following the earlier 20-file / 403-case core run. Strict lint, typecheck,
  formatting/ratchets and docs checks passed. Task 02 qualifies intermediate
  failures and bundle provenance. Independent production review remains open;
  round 14 is next.
- Round 14: Sol child `ba7d78c1-ba09-40af-8c35-f86e54051185` reviewed
  `f8729c164b`. Three source-derived P2s reproduced: same-scope right-pane
  rebuild drops deletion ownership; hiding another tab adopts the removed
  group's fallback; workflow failure releases the phone's owned null slot.
  Controlled RED is four failures / 28 controls. Corrections retain the group
  lease through restoration, scope hide reconciliation to its original owner,
  and end empty-slot ownership only at actual selection/navigation.
  Scoped verification is complete: 279 unit cases, all 23 desktop cases and all
  8 phone cases passed on the fresh frontend, with strict lint/typecheck/static
  gates. Task 02 records RED, fixture corrections, captures and retained-backend
  limits. Independent review remains open; round 15 is next.
- Round 15: Sol child `0c0b0561-6023-4ccc-9acd-639c2e9464cc` reviewed
  `793af6672b`. Four source-derived P2s accepted: failed desktop workflow loses
  passive hidden ownership; maximization parks the leased group; cancelled
  empty slots lack WS continuation; inactive visible-tab recipients lose dots.
  Corrections and scoped verification are complete: final 211 unit cases,
  24 desktop cases and 8 phone cases passed. A real remount cursor leak was
  reproduced and repaired after the fixture corrections. Task 02 records all
  intermediate/final receipts. Independent review remains open; round 16 is next.
- Round 16: Sol child `f69c0cc0-fed3-4ce0-8a3a-1c6a61e94fec` reviewed
  `f67355c127`. Four P2s reproduced: parked-first removal misses its group lease;
  maximize restoration can insert hidden C; unchanged primary updates fill a
  workflow-cancelled slot; terminal passthrough fallback bypasses read admission.
  Corrections and scoped verification are complete: 176 unit cases, all 18
  desktop removal/unread cases and eight phone cases passed on the fresh
  frontend. Real browser follow-through additionally repaired transient and
  disposed-portal visibility during maximization; Task 02 retains RED and
  intermediate failures, exact commands, captures and backend-reuse limits.
  Strict lint/typecheck/changed-code formatting/ratchets passed. Independent review remains
  open; round 17 is next.
- Round 17: Sol child `a1431207-19d1-47a0-b123-dfeabed9c6db` reviewed
  `7d12e0ef9b`. Four source-derived P2s reproduced: reconstruction loses the
  retirement edge; maximize changes a removal group's identity; visit-local
  hides bypass restore; delayed task creation fills an owned empty slot.
  A P3 also corrects the displayed-session design description under .17.
  Controlled RED: four failures / 54 controls. Corrections capture during
  reconstruction without pruning, preserve group identity, share live hide
  admission with restore, and restrict creation to retargeting an existing
  controller. Post-production units passed nine files / 221 cases, followed
  by the final type-narrowed identity test. Final-bundle verification passed
  all 21 desktop removal/unread cases and eight phone cases. Zero-warning
  code lint, typecheck, formatting/ratchets and docs gates passed. Task 02
  retains scaffold/type corrections and exact receipts. Independent review
  remains open; round 18 is next.
- Round 18: Sol child `846ff3a2-fdf0-4bf6-8123-8d3a73ddd6cb` reviewed
  `85c8e66142`. Two source-derived P2s accepted and reproduced: an undelivered
  visible successor stays inactive after maximize exit; a canonical terminal
  overlay is constrained as an ordinary right column on persisted restore.
  The new browser RED has no proven C transcript after exit; the fixup RED
  resizes the overlay as a right pane (14 controls passed). Corrections resume
  owned focus after restoration and exempt only verified overlays from right
  constraints while retaining logical group identity. Verified: 121 unit cases,
  two persisted-geometry cases, all 22 desktop removal/unread cases and eight
  phone cases passed. Strict code lint/typecheck/build/formatting/ratchets and
  docs gates passed. Task 02 records fixture corrections and exact receipts.
  Independent review remains open; round 19 is next.
- Round 19: Sol child `410b1e62-83e5-493d-8022-11207a7e8c27` reviewed
  `22cae38049`. Two P2s reproduced: passive restore adoption overwrites a
  committed workflow recipient; membership invalidation aborts an otherwise
  current initial route. Corrections preserve committed workflow requests and
  replace only invalidated membership generations under the retained route.
  Six owning/adjacent files passed 57 cases; all 23 desktop removal/unread
  cases and eight phone cases passed on the fresh R19 frontend. The new
  Preview-to-workbench case holds authentic initial membership across remote
  deletion and reaches the surviving transcript/read cursor without reload.
  Strict checks and fixture corrections are recorded in Task 02. Independent
  review remains open; round 20 is next.
- Round 20: Sol child `58c572af-0f9e-49e9-b69c-86c375356f06` reviewed
  `fabcfa652f`. One P2: replacement membership can pair with a captured task
  DTO and transiently select/read a newer completed sibling instead of the
  proven successor. Reproduced stale shell selection and fulfilled membership
  reuse; the real held-HTTP browser flow advanced the unrelated sibling's cursor
  on the R19 bundle.
  Corrections preserve membership generation through identity
  admission, use the membership primary, and keep newer selection ownership
  through hydration and ordinary task-page synchronization. The final 28
  affected unit files passed 349 cases; all 23 desktop and eight phone cases
  passed on the fresh R20 frontend. C is displayed/read, B retains its null
  cursor and receives no mark-read. Strict checks and causal receipts are in
  Task 02. Independent review remains open; round 21 is next.
- Round 21: Sol child `a011e8ad-0f88-467b-84b2-b92c4b87748b` reviewed
  `e7a64f2f3d`. Two P2s: an old removal receipt vetoed a later explicit
  conversation link; independently fetched DTO/membership primaries could
  disagree and read the former primary. Both were reproduced through real
  route consumers and authentic gated browser requests. Corrections admit
  validated explicit links against entry-time ownership and reconcile surviving
  primary conflicts before shell selection. Newer pins, workflow recipients
  and removal owners retain precedence. Execution receipts and exact replay
  commands are in Task 02. Independent review remains open; round 22 is next.
- Round 22: Sol child `bc9daed1-188a-48fe-bbad-10b7281715f1` reviewed
  `75906e0814`. Three P2s: a new primary missing from the first membership,
  hidden explicit route targets, and route admission discarded by a pending
  workflow that later fails. Parent reproduced all three at real reader,
  hydrator/page and Dockview boundaries. Corrections and execution receipts
  are in Task 02. Independent review remains open; round 23 is next.
- Round 23: Sol child `30f558db-99d9-4c9e-bafb-10602a412a20` reviewed
  `fc98c6ba5a`. Three P2s: boot-backed explicit routes lacked presentation
  ownership; unrelated primary reconciliation could discard proven explicit
  membership; successful agentless moves left deferred routes pending.
  Parent reproduced all three. Corrections carry boot ownership, retain
  current-generation explicit-member proof, and forward a positively known,
  entry-correlated no-focus disposition. Task 02 records the wire scope and
  execution evidence. Independent review remains open; round 24 is next.
- Round 24: Sol child `061313ff-2a9f-47f8-b659-fbeba258e5c7` reviewed
  `69ffdf9e4e`. Three P2s: maximized explicit activation, deferred replay after
  observed deletion, and skip-only entries with no effective launch.
  Parent reproduced each through hook, route/removal and HTTP/WS regressions.
  Corrections and final verification are recorded in Task 02; round 25 is next.
- Round 25: Sol child `971fd8fb-efa6-4d9a-8bfa-bfe6351742ac` reviewed
  `9170e2b280`. Three P2s: explicit parked-group targets under maximize,
  Restore losing the consumed selection, and generic hydration bypassing
  observed-removal admission. Parent reproduced all three at real Go-boot,
  Dockview and held React-commit boundaries. Results are in Task 02; R26 is next.
- Completed rounds: 25. Consecutive clean: 0. Stop after three consecutive
  clean rounds or 50 rounds. Earlier loops do not count.

### Production review R26

Sol child `2b3d97d4-83fc-4d98-949b-6560ca8907f2` reviewed pushed
`4da96e018e8fec2fdc8d10ab89f5883f1b10d016`: four P2s, not clean.
The tree-backed Restore, hidden-before-maximize target, confirmed proof before
shell commit, and phone retained-Plan cases have parent RED evidence and
targeted corrections in Task 02. Completed production rounds 26; consecutive
clean 0. Independent R27 starts only after verified commit/push.

R26 final author evidence: 27 files / 469 focused tests; strict changed-file
lint, typecheck, fresh frontend and docs pass. Five desktop Go-boot and ten
phone cases pass on the final bundle, one worker/no retries. Tree/hidden
captures and phone later-Plan preservation inspected. Task 02 has exact replay
and retained logs; R27 is the next independent round.

### Production review R27

Sol child `447beb44-1163-4396-8efb-aacec8fca564` reviewed
`90b4a0b8f6d6c8f5d6ed1790c6718b6fcb9a164b`: four P2s, not clean.
Repeated same-URL conversation activation, legacy maximize group IDs, scoped
predecessor 404 admission, and a stale exit frame persisting a new environment
have parent reproductions and targeted corrections in Task 02.
Completed production rounds 27; consecutive clean 0. R28 follows verified push.

Final R27 author receipts: 20 files / 360 targeted tests pass after the final
refactor/fixture correction; strict lint/typecheck/fresh frontend/docs pass.
Seven desktop and ten phone cases pass on the final bundle, one worker/no
retries. Legacy Restore and repeated-hit phone captures inspected. Task 02
records failed intermediate runs separately and gives exact scoped replay.

### Production review R28

Sol child `205d8bc3-1c18-48e4-a2b0-91d5532be760` reviewed
`8822a18b3dfb91913350bb4273975019d3988fdb`: four P2s, not clean.
HTTP activation, serialized Saved Layout ownership, ordinary membership after
uncertain proof and phone Plan dots have parent RED/targeted corrections in
Task 02. Completed production rounds 28; consecutive clean 0.
R29 follows verified delivery; earlier review loops do not count toward this one.

R28 final author receipts: 14 files / 304 tests plus an overlapping final owning
reader run (10 cases). Strict lint/typecheck/fresh frontend/docs pass after
test-only lint/type corrections. Eight desktop and ten phone cases pass,
one worker/no retries, on the final bundle. Foreground-survivor and phone
captures inspected; Task 02 records RED/fixture failures and exact replay.

### Production review R29

Sol child `20cac842-75ee-4d19-ab1f-3cc5fe6dfccb` reviewed
`9e524f6df2e9e6d01fa38c05e01d64ca60164cfc`: three P2s, not clean.
Completed production rounds 29; consecutive clean 0. Task 02 records the
confirmed-slot, coarse-tablet and stale Saved Layout completion corrections.
R30 follows verified delivery.

R29 final author receipts: 13 files/314 targeted tests, seven desktop/tablet
and ten phone cases pass after the final production edit. Strict lint,
typecheck, fresh frontend, harness/docs checks pass. Final tablet file/dot,
Go-boot Chat and local foreground survivor captures inspected. Task 02 records
the additional restoration RED and intermediate non-green runs separately.

### Production review R30

Sol child `2386540f-7cb4-4536-905a-d0d0d6dc0b87` reviewed
`4e2547e4d2e903f337726a67892e2c03cbbf4f03`: four P2s, not clean.
Completed production rounds 30; consecutive clean 0. Task 02 records the
corrections and targeted RED. R31 follows verified delivery.

R30 author receipts: 14 files/303 targeted tests plus overlapping final
12/4-case fixture replays; strict lint/typecheck/fresh frontend and
harness/spec/catalog checks pass. Final single-worker/no-retry browser runs:
nine desktop/tablet and ten phone cases pass. Final dirty-editor/dot and
committed-workflow Chat captures inspected. Task 02 separates the initial
non-green UI fixtures from these final receipts and preserves scope limits.

### Production review R31

Sol child `b061016f-243c-4873-8177-68bf2abab888` reviewed
`dee0fe48b586f4e430eb108c0e4c47eb192e6955`: five P2s, not clean.
Completed production rounds 31; consecutive clean 0. Task 02 records the
targeted tablet and layout RED, corrections and delivery evidence.

R31 author receipts: 16 files/325 targeted tests; strict lint/typecheck/fresh
frontend pass. Ten desktop/tablet/maximize and ten phone cases pass on the
final bundle, one worker/no retries. Final coarse editor/dot and workflow
captures inspected. Task 02 records intermediate fixture/type failures and
the visit/negative-mutation/rollback control REDs separately. R32 follows
normal verified delivery; the review loop remains open.

### Production review R32

Sol child `5a3aacd8-8802-43fb-8c2a-2ef150c56757` reviewed
`9c8085e549675489011baa253ed48ecf686a1e34`: five P2s, not clean.
Completed production rounds 32; consecutive clean 0. Task 02 records the
Files-tree admission/deletion, installed-alias selection, Git readiness and
both-format layout rollback corrections and RED receipts.

R32 author unit receipt: 21 files/355 tests, strict changed-file lint,
production typecheck and fresh frontend build pass. Spec/catalog/harness
checks pass after keeping the owning design within its size limit. Ten
desktop/tablet/maximize and ten phone cases pass on the final fresh bundle,
sequential/one worker/no retries. Tablet Chat and failed-resume phone captures
inspected. Task 02 retains the exact limits and intermediate non-green
receipts. R33 follows normal verified delivery; the loop remains open.

### Production review R33

Sol child `2381536b-2336-4269-a1a8-77baeaa19248` reviewed
`02ba6642589f12017d0ce1bb0bcf27492ddd070d`: seven P2s, not clean.
Completed production rounds 33; consecutive clean 0. Task 02 records the
pending-file mutation/selection, current-tab retirement, immediate metadata,
multi-repo readiness and coordinated layout rollback corrections.

Final R33 author receipts: 24 files/387 targeted tests, strict changed-file
lint, typecheck and fresh frontend build pass. Ten desktop/tablet/maximize
and ten phone cases pass, sequential/one worker/no retries. Dirty tablet
editor/dot and phone failed-resume captures inspected. Task 02 records the
RED, fixture/type corrections and exact browser/rollback limits. R34 follows
normal verified delivery; the review loop remains open.

### Production review R34

Sol child `c4530143-78ce-4b8f-a9a6-c021ab8a45be` reviewed
`c64cd5070beb906fb008a76b6e51b5d023fe2cd8`: one P2, not clean.
Completed production rounds 34; consecutive clean 0. With unknown inventory,
tablet ready-empty now treats an exclusively unnamed status as the single scope;
named repositories stay unknown. An intermediate tablet refresh was withdrawn
after it delayed restored file tabs. Task 02 records the evidence; R35 follows
normal verified delivery.

### Production review R35

Sol child `30e33040-dc31-4c0a-9b41-6c636bd4c581` reviewed
`51c96a7dcd34be451eecec31734f0f49b1e2f569`: one P2, not clean.
Completed production rounds 35; consecutive clean 0. Named refresh evidence
now also blocks the lone-unnamed inference. Task 02 records the RED, the
accepted untraced frame-order residual and verification; R36 follows normal
verified delivery.

### Production review R36

Sol child `0528c855-ecda-4eb0-ab30-8cfa5009ae94` reviewed
`ee2f77f5cc8452f597d3c15d81af40629ea691d7`: one P2, not clean.
Completed production rounds 36; consecutive clean 0. The lone-root inference
now also requires at most one registered task repository. Task 02 records the
RED and verification; R37 follows normal verified delivery.

### Production review R37

Sol child `b6919db6-a3c9-44f6-96db-89ab3b92091c` reviewed
`931b7e6cace251fb917e76582714908e1b5650af`: one P2, not clean.
Completed production rounds 37; consecutive clean 0. The repository-count guard
now resolves the task like the task page. Review rounds from R38 use the Opus
profile at the user's direction; R38 follows normal verified delivery.

### Production reviews R38-R40

R38 and R39 (Opus) reviewed `1de14a578eab4adb92eeb43176162b4d729e6767` clean.
R40 (Opus) found a same-task URL replace erasing a committed conversation
activation (P2) and a disproved removal observation never withdrawn (P3), and
noted a base gateway test removed by a withdrawn R1 experiment. Completed
production rounds 40; consecutive clean 0. Task 02 records the corrections,
the restored base test and verification; R41 follows normal verified delivery.

### Production review R41

Opus child `f500a039-b090-491a-a1f0-f22b43167a86` reviewed
`ceb9cdfa6150be2b7b0e74b4ebdc81a58eaf9ca6`: one P3, not clean. Completed
production rounds 41; consecutive clean 0. A disproved removal now also
releases the removal-sourced view owner. R42 follows normal verified delivery.

### Production review R42

Opus child `6b661e78-f546-46c5-beb8-2b14150b34a4` reviewed
`3ea22e37f236649175b5b42b91f7bbeaf6317cc1`: one P3, not clean. Completed
production rounds 42; consecutive clean 0. Follow intent that arrives while a
removal is unproven now resumes as ordinary verification when the removal is
disproved. R43 follows normal verified delivery.

### Production review R43

Opus child `ad1c32cb-38b9-4016-8080-19a6efab79b8` reviewed
`cdf1ec55c65ff0517825dfec3caea1137ac32680`: two P3s, not clean. Completed
production rounds 43; consecutive clean 0. A removal observed over an
in-flight follow now keeps that intent, and a later lifecycle transition no
longer replaces a deferred primary target. R44 follows normal verified delivery.

### Production reviews R44-R46: loop closed

Opus children `3e5c09d0-6f1a-4ede-a1a3-c962f2af17d7` (R44),
`528c7014-e3b2-4c36-81be-4ba821b79764` (R45) and
`40dced2d-b3a2-43a9-b471-73db37ca0ea5` (R46) each reviewed
`9a9a8825973da66d0e302fc1b0121ad645a39b06` independently and found nothing.
Completed production rounds 46; consecutive clean 3. The stop condition is
met and the failed-resume production review loop is closed. Task 02 records
the scopes, non-findings and the open product question.
