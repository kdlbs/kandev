---
created: 2026-09-19
status: in_progress
requirements:
  - REQ-OFFICE-TASKLESS-001
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
legacy_specs: []
---

# Implementation Plan: Taskless Office Run Sessions

## Overview

`REQ-OFFICE-TASKLESS-001` requires a lightweight (taskless) Office wake to start
a real agent session and reach a visible terminal outcome **without creating any
task or task session**. The run-owned session mechanism that satisfies it has
landed: `office_run_sessions` (Office repository, unique `(run_id, attempt)`),
the `RunSessionLauncher` seam, its `officeRunSessionLauncher` implementation,
`runs.session_id` binding, and startup reconciliation via `ReconcileRunSessions`.

**Scope was cut on September 19, 2026.** `AC-OFFICE-TASKLESS-001.5` (operator
stop controls) and `.6` (restart reconciliation) were removed from the
requirement after five rounds of spec review, and the four work orders that
served them — Tasks 02, 03, 04 and 07 — are withdrawn. Their files stay in this
directory, each carrying a withdrawal banner, because a follow-up should start
from them. The deferral and the accepted gaps are recorded under
[Deferred: stop controls and restart recovery](../../specs/office/requirements/taskless-run-sessions.md#deferred-stop-controls-and-restart-recovery).

**Three results remain, and all three are coverage.** No production change is in
scope; the three production work orders were the withdrawn ones, except for the
single run event Task 06 adds for a failed continuation-summary write.

The first: the end-to-end proof that an **armed cron trigger** reaches a launched
taskless session is still skipped. `TestRoutine_CronFire_LightweightReachesSession`
carries `t.Skip("blocked on card 49894d63: the lightweight (taskless) routine
flow has no task_sessions row to assert on yet")`, and its intended assertion
targets the wrong table: the shipped design deliberately writes no
`task_sessions` row. Its heavy-routine sibling in the same file already passes.
This is the clause the card owes Beta. Task 01 also closes the manual/webhook
half of `AC-OFFICE-TASKLESS-001.3` and the two-fires half of `.2`.

The second: the provider-routed taskless path has no test at all — not for the
routed launch itself, not for the actual adapter/model recorded on that path,
and not for a delayed event from an older attempt of the same run, which
provider fallback produces routinely. Task 05.

The third: two event-path guarantees are asserted by nothing — `.2`'s "a failed
attempt shall not replace the last successful summary", and task consumers not
acting on run-owned events. Task 06 also adds the one run event `.2` gained in
Spec round 4 for a failed summary write, which the current code swallows
silently.

## Prior art

### Our own prior reasoning (compiled wiki)

Searched the vault at `/Users/henry/Documents/henry/wiki` (pinned `@henry`) via the
QMD MCP `wiki` collection, 578 documents, index last updated 2026-09-18. The
`obsidian-wiki` CLI is not installed on this machine, so the GraphRAG pre-pass
was skipped and retrieval was QMD lexical + semantic + HyDE. Direct filesystem
reads of the vault are sandbox-blocked here, so QMD was the only transport; the
skill's `log.md` append could not be performed for the same reason.

Two pages answer this initiative's design fork directly and outrank fresh
reasoning:

- `[[agent-session-object]]` (score 0.76, `lifecycle: draft`, updated
  2026-09-10) — a session is "the addressable unit of agent work: one durable,
  server-side record with an identity, a lifecycle, an owner, attached
  artefacts, consumption, and derived analytics", and explicitly **not** a loop
  iteration. Names "conflating session with conversation" as an anti-pattern.
  This is the case for the session being first-class in its own right rather
  than a child of a work item.
- `[[event-triggered-agent-activation]]` (score 0.83) — "a trigger is a standing
  order that opens a session with no human in the initiating position". Notes
  that filtering must happen **before** session creation so non-matching events
  cost nothing, and warns that "a daily cron on a no-op still opens a session;
  cost scales with firings, not with useful work". That is the rationale for the
  idle-skip gate this design preserves.
- `[[kandev]]` — records that the legacy `heartbeat` wakeup source is retired
  and periodic wakes now flow through a per-coordinator cron routine, which
  matches the dispatch chain under test.

The wiki has **no** page on the specific storage question (nullable foreign key
versus synthetic placeholder row), so that fork was resolved from the code and
the system design rather than from prior reasoning.

### What other products shipped (saas-kb)

Queried the `saas-kb` MCP with `category: "ai_sdlc"`, per platform rather than
once, because retrieval collapses onto whichever vendor documents best.

- **Warp — Scheduled Agents.** A cron schedule "starts a fresh agent session for
  every run"; "no state is carried over between runs". The session is the unit;
  a task appears only as an *output* the run links to. Also documents the
  identity trap this repo handles separately: a CLI-created schedule "runs as
  its creator and opens pull requests under that person's GitHub account"
  unless an agent identity is set.
- **Augment Code — Automations/Schedules.** "If a Session is one conversation, an
  automation is the standing order that starts those conversations without you
  in the loop."
- **Paperclip — Heartbeats & Routines.** The outlier: there, a routine fire
  "creates (or reopens) a task" and the assignment wakes the agent, so every
  scheduled fire has a visible execution issue. Its stated motivation is
  traceability ("why did this agent run?"). It also ships the
  concurrency (coalesce / skip / always-enqueue) and catch-up (skip-missed /
  capped-batch) policies this repo already has.

Treat all of the above as vendor claims, not evidence of correctness.

### What we are doing differently, and why

Warp and Augment converge on the session being the unit and a task not being a
prerequisite, which supports this design. Paperclip materialises a task per fire;
we deliberately do not, because Kandev already has a dedicated
`office_routine_runs` record that supplies the traceability Paperclip gets from
its execution issue, so a task per fire would add a board row no human asked for
and pollute task counts.

Where we differ from **both** shapes: rather than reusing the task-bound session
table with an optional task, this design adds a separate Office-owned
`office_run_sessions` record. The system design forbids the two options this
initiative was originally filed to weigh: "No changes to task-session
nullability or synthetic task creation are permitted." That keeps every existing
task-session invariant and every reader that assumes a task intact, at the cost
of a second session table, whose reconciliation and retention are specified
alongside it.

## Scope

### In scope

- Un-skip `TestRoutine_CronFire_LightweightReachesSession`
  (`apps/backend/internal/backendapp/office_routine_cron_to_session_test.go`)
  and rewrite its assertion against the storage seam that actually shipped:
  an `office_run_sessions` row for the dispatched run, and `runs.session_id`
  bound to it.
- Assert the negative half of `AC-OFFICE-TASKLESS-001.1` explicitly: the fire
  creates **no** `tasks` row and **no** `task_sessions` row.
- Correct the stale skip comment above the test, which describes a
  `task_sessions` row the design forbids.
- Assert the manual/webhook half of `AC-OFFICE-TASKLESS-001.3` and the
  two-distinct-sessions half of `.2` (Task 01).
- Cover the provider-routed taskless launch, which has no test (Task 05).
- Pin the two event-path guarantees nothing asserts, and the new failed-write
  outcome on `AC-OFFICE-TASKLESS-001.2` (Task 06).

### Out of scope

- **Operator stop controls and restart reconciliation.** Cut from the
  requirement on September 19, 2026 along with `AC-OFFICE-TASKLESS-001.5` and
  `.6`. Tasks 02, 03, 04 and 07 are withdrawn with them. Three accepted gaps
  follow and are named rather than left silent: run cancellation, agent disable
  and agent removal do not stop a live run-owned execution; a crash between an
  attempt's terminal write and its run's terminal write leaves a run claimed
  behind a terminal attempt; and the launch-failure path discards the
  `runtime.Stop` error and writes the attempt terminal anyway. All three, and
  the two problems a follow-up must resolve first, are recorded under
  [Deferred: stop controls and restart recovery](../../specs/office/requirements/taskless-run-sessions.md#deferred-stop-controls-and-restart-recovery).
- Any production change beyond the one run event Task 06 adds for a failed
  summary write. Every remaining result is coverage. If a test cannot be written
  without changing production behavior, that is a finding against the
  requirement, not a licence to edit the code under test.
- Pruning run-session rows when run-history retention deletes their run. Named
  out of scope in the requirement, with what a follow-up would need.
- Storing a route-attempt-to-session association. The route ledger has no
  session column and no criterion needs one.
- The heavy-routine path and its test, which already pass.
- `REQ-OFFICE-COORDINATOR-AUTHORITY-*` (the taskless run's board-read and
  annotation authority). Delivered separately and already `implemented` under
  [`office-taskless-coordinator-authority`](../office-taskless-coordinator-authority/plan.md).
- Interactive taskless chat, synthetic tasks, and cross-fire ACP resumption,
  which the requirement lists as out of scope.

## Technical approach

The dispatch chain under test already exists end to end:

```text
TickScheduledTriggers
  -> processCronTrigger (claims the armed next_run_at slot)
  -> dispatchRoutineRun (task_template == "" => lightweight branch)
  -> agent_wakeup_requests row
  -> wakeup dispatcher -> fresh taskless run
  -> scheduler processRun -> launchAgent (taskID == "")
  -> RunSessionLauncher.StartRunSession
  -> office_run_sessions row + persistLaunchedSession -> runs.session_id
```

The test harness in the file (`newRoutineCronHarness`) already stands up the
SQLite store, office repository, orchestrator and a `stubAgentManager` whose
`LaunchAgent` captures the request, which is what the passing heavy-routine test
uses. The work is to drive the same harness down the lightweight branch and
assert on the Office-owned session record instead of the task-owned one.

## Tests

Rebuilt from assertions rather than filenames. Spec Review round 2 found the
previous table cited neighbouring files for six criteria: a test in the right
area is not evidence for the criterion. Every entry below names the test
function that asserts the clause, and a clause with no such test is listed as a
gap with the work order that closes it.

| Acceptance criteria | Evidence |
| --- | --- |
| `.1` starts a real session, no task or task session | `office/service/taskless_lifecycle_test.go` — `TestTasklessRoutineRunLaunchesRunSessionAndCompletes` (run session reaches a terminal outcome; asserts zero `tasks` rows). Cron-fire end to end: Task 01 |
| `.2` continuation scope is chosen once and round-trips | `office/service/continuation_summary_reader_test.go` — `TestLoadContinuationSummary_RoutineScope_RoundTripsThroughRealWriterAndReader`, `..._AgentScope_...`, `..._ScopeSurvivesCoalesceAfterClaim` |
| `.2` every fire and retry uses a fresh, never-reused session | **Gap.** The scope tests above assert selection and round-trip, not session identity. Nothing asserts that two fires and a retry get distinct sessions. Two fires: Task 01. A retry on one run: Task 05 |
| `.2` a failed summary write leaves the last successful summary intact | **Gap.** The write failure is swallowed today. Task 06 |
| `.2` a failed attempt does not replace the last successful summary | **Gap.** No test drives a failure and reads the summary back. Task 06 |
| `.3` periodic idle-skip admission on a cron wake | `office/service/wo46_idle_skip_routine_dispatch_test.go` — `TestIdleSkip_RoutineDispatchNoTasks_Skipped`, `TestIdleSkip_RoutineDispatch_CoordinatorNotSkipped`. Both queue `shared.RunReasonRoutineDispatchCron`, so they cover the periodic half of the clause and only that half |
| `.3` idle skipping does not consume a manual or webhook wake | **Partly covered; the remaining half is a gap.** `office/shared/runreasons_test.go` — `TestRoutineDispatchReason_SourceMapping` asserts a manual or webhook fire carries `RunReasonRoutineDispatchEvent`, and `TestIsPeriodicTasklessWake_LegacyRoutineDispatchIsNonSkippable` pins the ambiguous legacy literal non-skippable. Nothing asserts `IsPeriodicTasklessWake(RunReasonRoutineDispatchEvent)` is false — that value reaches the `default` branch untested — and nothing drives `checkIdleSkip` with that reason and `SkipIdleRuns` on. Task 01 |
| `.3` capabilities apply to a taskless run | `office/runtime/tasks_list_test.go` — `TestListTasks_TasklessRunWithCapabilitySucceeds`; `office/repository/sqlite/failure_test.go` — `TestHasPriorTasklessFailedRun`, `TestHasPriorTasklessFailedRun_IsolatedByContinuationScope` |
| `.3` coalescing applies to a taskless run | `office/service/continuation_summary_reader_test.go` — `TestLoadContinuationSummary_ScopeSurvivesCoalesceAfterClaim` (coalesce into a claimed taskless run); `office/routines/service_reconciliation_test.go` — `TestDispatch_LightweightRoutine_SourceKeysAreUnique`, `TestDispatch_LightweightRoutine_WebhookRequestKeyIsStable` |
| `.3` budget admission applies | **Inherited structurally, not cited.** `admitRun` runs before the task/taskless branch in `launchAgent`, so a taskless run cannot reach launch without passing the same five gates. Recorded here rather than cited, because no taskless-specific assertion exists or is owed. |
| `.3` retry and backoff apply | **Inherited structurally, not cited.** A failed taskless launch settles through the shared run-failure path (`failTasklessRun` → `MarkRunFailed`) and the shared routing backoff, neither of which has a task predicate. Recorded rather than cited for the same reason as budget admission. |
| `.3` provider-routed profiles work | **Gap.** The routed taskless path has no test. Task 05 |
| `.4` run history shows the exact session and runtime outcome; usage attributed once | `office/service/event_subscribers_run_output_test.go` — `TestHandleTasklessAgentCompleted_RecordsOutputSummaryAndKeepsContinuationSummary`; `office/service/taskless_lifecycle_test.go` — `TestTasklessUsageDuplicate` |
| `.4` run history shows the **actual** adapter/model | `office/dashboard/run_detail_test.go` — `TestGetRunDetailActualInvocation` (reads the recorded adapter and model back off the record), `TestGetRunDetailUsesNewestRunSessionInvocation` (two attempts on one run: detail shows the newest attempt's session, adapter and model). Previously uncited; the three tests on the row above only *supply* an adapter and model on a mock launch result and never read them back. The routed path, where the recorded pair can differ from the profile's, is covered by Task 05's first bullet |
| `.4` a delayed event from an older *run* does not clear a newer run | `office/service/taskless_lifecycle_test.go` — `TestTasklessLateCompletionDoesNotFinishSuccessor` (two runs, attempt 1 each) |
| `.4` a delayed event from an older *attempt of the same run* does not clear the newer attempt | **Gap.** Provider fallback produces exactly this shape routinely — three candidates, three attempts on one run — and no test injects a delayed predecessor event into it. Task 05 |
| `.7` task-decision tools reject a taskless run; failed lookup fails closed | `office/runtime/handler_decision_test.go` — `TestRuntimeHandler_RecordAgentDecisionRejectsTasklessRun`; `office/runtime/scope_derivation_test.go` — `TestBuild_TasklessRunMaterializesRunnerSet`, `TestBuild_RunnerSetQueryErrorFailsClosed`, `TestBuild_WhitespaceOnlyPayloadTaskIDIsTaskless` |
| `.7` workspace boundary retained | `office/runtime/actions_annotation_test.go` — `TestPostComment_TasklessRunRefusesCrossWorkspaceTarget` (a taskless run in `ws-1` targeting a task in `ws-2` gets `ErrTaskOutOfScope` and no comment is written) and `TestPostComment_TasklessRunRefusesNonexistentTargetWithSameSentinelAsCrossWorkspace` (the anti-oracle: a missing task returns the same sentinel, so the refusal leaks no existence). The permitted-success case is `TestRuntimeHandler_TasklessRunAnnotatesWorkspaceTaskOverHTTP` in `office/runtime/handler_taskless_annotation_test.go`, which on its own is not evidence for this clause — it asserts only a 201 |
| `.7` capability boundary retained | `office/runtime/tasks_list_test.go` — `TestListTasks_TasklessRunWithCapabilitySucceeds` shows a taskless run traversing the capability gate, and `TestListTasks_WithoutCapabilityDenied` shows that gate returning 403 with the lister never called. The pair is the evidence: the denial test is not itself taskless-specific, and it is cited for the gate a taskless run is proven to pass through, not as a neighbouring test |
| `.8` task-bound launches and their task/session lifecycle unchanged | `office/service/scheduler_taskless_launch_test.go` — `TestSchedulerTick_TaskBoundRunStillLaunches` |
| `.8` a taskless failure does not auto-pause the agent or re-enter the inbox | same file — `TestSchedulerTick_TasklessRunsDoNotAutoPauseAgent`, `TestSchedulerTick_RepeatTasklessFailures_OnlyFirstStaysInInbox` |
| `.8` historical unsupported taskless failures are not automatically replayed | **Satisfied structurally; no work order owes it.** Nothing in scope can replay a recorded failure: the unfinished-session query selects `preparing` and `running` only, so a `failed` run is in no population any code path here inspects, and no change in scope adds a channel that relaunches from run-session state. The system design states this. It stops being free the moment the deferred flows land, which is why the follow-up owes it a case |
| Reservation CAS and schema | `office/repository/sqlite/run_sessions_test.go` — `TestRunSessionReservationCAS`, `TestRunSessionSchemaExists` |
| Run-owned events ignored by task consumers | **Gap.** Enforced structurally by an empty task-session identity; nothing asserts it. Task 06 |

## E2E tests

None. `git diff --stat -- apps/web` is empty for this change: there is no
`apps/web` change, no new user-visible surface, and no UI behavior to drive.
The taskless run's existing surfaces (run detail, inbox, costs) are unchanged.
The three surviving work orders are coverage only — the earlier form of this
decision reasoned about Tasks 03, 04 and 07 adding backend-only behavior, and
those are withdrawn, so the conclusion holds more strongly rather than less.

## Work orders

- [completed] [Task 01: Prove the lightweight cron fire reaches a session](task-01-prove-lightweight-cron-to-session.md)
- [completed] [Task 05: Cover routed (provider fallback) taskless launch](task-05-cover-routed-taskless-launch.md)
- [completed] [Task 06: Pin the taskless event-path guarantees no test asserts](task-06-pin-event-path-guarantees.md)

Withdrawn on 2026-09-19 with `AC-OFFICE-TASKLESS-001.5` and `.6`. Not to be
built; kept for a follow-up:

- [withdrawn] [Task 02: Cover restart reconciliation of unfinished taskless attempts](task-02-cover-restart-reconciliation.md)
- [withdrawn] [Task 03: Stop live run sessions on run cancel and agent disable/removal](task-03-stop-run-sessions-on-cancel-and-agent-change.md)
- [withdrawn] [Task 04: Fail closed on discarded requeue and stop errors](task-04-fail-closed-on-recovery-and-stop-errors.md)
- [withdrawn] [Task 07: Settle a claimed run from its terminal attempt instead of relaunching](task-07-settle-terminal-attempt-before-relaunch.md)

## Dependency order

```text
Task 01   Task 05   Task 06
```

**No dependencies.** All three are wave 1, all three declare `depends_on: []`,
and all three are coverage touching disjoint test files. They may run in any
order or together. The `Task 02 -> Task 04 -> Task 07` chain this section used
to state is gone with its three members; it was the only ordering constraint
this plan had.

## Verification results

Pending Task 01. Baseline receipt taken on
`d355a672b1db624048f8ff87bc456a1729f78cb0`:

```text
$ go test ./internal/backendapp/ -run 'TestRoutine_CronFire' -v -count=1
=== RUN   TestRoutine_CronFire_HeavyRoutineReachesSession
--- PASS: TestRoutine_CronFire_HeavyRoutineReachesSession (0.09s)
=== RUN   TestRoutine_CronFire_LightweightReachesSession
    office_routine_cron_to_session_test.go:295: blocked on card 49894d63: the
    lightweight (taskless) routine flow has no task_sessions row to assert on yet
--- SKIP: TestRoutine_CronFire_LightweightReachesSession (0.00s)
PASS
ok  	github.com/kandev/kandev/internal/backendapp	1.169s
```

## Risks

- The test drives a multi-stage asynchronous chain (cron tick to wakeup row to
  run claim to launch). It must wait on an observable condition rather than a
  fixed sleep, matching the heavy-routine test's polling shape, or it will be
  flaky on a loaded CI shard.
- If the lightweight chain turns out to break somewhere between the wakeup row
  and `launchAgent` when driven by this harness, the correct response is to
  report that as a behavior finding against `REQ-OFFICE-TASKLESS-001`, not to
  weaken the assertion or re-skip the test.

## Open questions

None outstanding. The storage-seam question this initiative was filed to decide
is settled in the system design: neither a nullable `task_sessions.task_id` nor
a synthetic task is permitted; the record is a separate Office-owned
`office_run_sessions` row.
