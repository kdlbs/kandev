---
created: 2026-09-17
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-AGENTS-SESSION-CEILING-001
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
legacy_specs: []
---

# Implementation plan: Ceiling replay and cancellation deadlock

## Overview

Remove the lock cycle that blocked deferred launches and task-state
reconciliation. The implementation narrows replay, claim, deferral, and route
critical sections, orders review reconciliation consistently, and adds
deterministic regression coverage with focused desktop/mobile verification.

The task system owns the correction because task entry ownership and runtime
state reconciliation form the failed boundary. Agent ceiling counting remains
owned by the [agent requirement](../../specs/agents/requirements/session-concurrency-ceiling.md).

## Evidence and root cause

Read-only investigation on 2026-09-17 used the retained backend log, SQLite
database, and the live `/debug/pprof/goroutine?debug=2` endpoint on port 38429.
Process discovery returned no instance, but the live endpoint was accessible.
The database recorded build `v0.94.0-212-g57bbae10ad`.

Affected task: `dffcacff-2515-4e02-ad27-d7ed20b001d6`.
Session: `383db1ce-2150-43f0-8d70-b03d3ce7ecae`.
ACP session: `01a0afa0-01e8-7b70-a02f-7e316282df8e`.

At 19:27:08 Lisbon time, provider cancellation acknowledgement and lifecycle
escalation occurred in the same millisecond. The session became
`WAITING_FOR_INPUT`, and its active turn closed. The task remained `IN_PROGRESS`.
Both task sessions were waiting in the database. The log also recorded disabled
cancellation-driven workflow completion. That setting does not explain the
missing runtime `REVIEW` write.

The live stacks established these blocked paths:

| Path | Held boundary | Waiting boundary |
| --- | --- | --- |
| Boot-ready callback through `writeTaskReviewState` | Global `taskRuntimeStateMu` | Task admission channel in `ceiling_entry_lock.go:32` |
| Ceiling sweep through `replayCeilingDeferral` and `claimSessionRunningForPrompt` | Task admission lock across replay | Session cancellation guard |
| Stream activity through `setSessionRunningForExecution` | Session cancellation guard | Global `taskRuntimeStateMu` |
| Explicit cancellation through `reconcileCancelledTaskReview` | Its session cancellation guard | Global `taskRuntimeStateMu` |

Source establishes lock ownership around those stack frames. The replay and
boot-ready stacks showed approximately 31 minutes of blocking. Multiple unrelated
state writers also waited on the same global mutex. The incident task is a
victim of the shared lock cycle, not evidence of a Luna-specific defect.

`ListAdmittedSessionIDs` counts `STARTING` and `RUNNING`, not task `IN_PROGRESS`.
The diagnostic database query found two counted sessions at that later instant.
Process-local reservations were not included in that query. The queue worker
was blocked regardless of the apparent free capacity. Changing a task row alone
cannot release these locks.

Evidence pointers are `/root/.kandev/logs/backend-logs.log` around lines
14564–14600 and `/root/.kandev/data/kandev.db`. These are mutable local sources,
not portable test inputs. No transcripts, raw ACP frames, or full stack dump are
copied into this package. Reproduce with isolated fixtures, not production data.

## Requirement conformance

- [Cancellation](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md):
  AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1 already requires settled
  cancellation and review-ready reconciliation. Criterion 001.2 extracts that
  outcome explicitly, including the existing sibling and queued-destination rules.
- [Queued ownership](../../specs/tasks/requirements/queued-session-ownership.md):
  AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.1–6 define state, exact delivery, retry,
  restart, and successor protection. Their existing draft status remains unchanged.
- AC-AGENTS-SESSION-CEILING-001.3, .5, and .6 define counting, replay, and
  reservation identity. This repair does not change capacity policy.

The [design correction](../../specs/tasks/system-design/queued-session-ownership.md#replay-and-reconciliation-locking)
defines short critical sections and dispatch revalidation. No new requirement
identity, database schema, or ADR is necessary. Existing claims and lifecycle
ownership provide the repair mechanism.

## Scope

### In scope

- Remove task admission ownership across runtime dispatch and session-lock waits.
- Order task admission before the global task-state mutex.
- Preserve final entry validation, exact claim settlement, and event publication.
- Prove cancellation settlement, unrelated task progress, and continued queue retry.
- Preserve working-sibling, queued-destination, archive, Office, and terminal rules.

### Out of scope

- Live restart, task data repair, task creation, or deployment.
- Changing ceiling limits, WIP occupancy, manual overrides, or queue ordering.
- Changing cancellation workflow policy, timeout values, or provider adapters.
- New worker pools, durable leases, schema migrations, UI markup, or copy.
- Repairing unrelated existing documentation or historical verification gaps.

## Technical approach

`ceiling_replay.go:replayCeilingDeferral` currently encloses concrete launch
seams in `lockCeilingEntryAdmission`. Narrow that scope to local validation.
Use the claim already acquired by `retryOneDeferredCeilingLaunch` for dispatch
ownership. Do not add another lease or discard the entry binding.

Audit all seven replay kinds through `validateContextCeilingEntry` and
`validateClaimedCeilingBinding`. Preserve atomic ordering between route mutation
and final local admission. Use existing session guards before task admission
for that short boundary. Do not hold task admission during provider acceptance
waits, startup waits, or synchronous callbacks. Revalidate after preparation
through `admitCeilingDispatch`, renewing the exact replay claim when present.

In `event_handlers_streaming.go:writeTaskReviewState`, acquire task admission
before `taskRuntimeStateMu`. Reload task, sessions, and queue identity within
the protected section. Preserve conditional writes and normal service events.
Audit every task-admission holder for reverse lock acquisition and reentrant
publication, including route promotion, deferral publication, and Send Now.

Scope the context ownership marker to the held section. Runtime callbacks use
their own context and must acquire their own guards. Passing a marker across
goroutines would conceal the deadlock by weakening serialization.

## Tests

Add `apps/backend/internal/orchestrator/ceiling_replay_lock_order_test.go`.
Use the real task repository and public service paths with a controllable runtime.
Use channels to establish lock ownership, not sleeps or repeated random races.

| Planned test | Evidence | Criteria |
| --- | --- | --- |
| `TestCeilingReplayBootReadyAndCancellationProgress` | Deferred prompt replay, boot-ready, and stream activity settle. An unrelated explicit cancel reaches REVIEW and returns. A second deferred task dispatches once. | cancellation 001.2; queued ownership 002.2–3; ceiling 001.5 |
| `TestCeilingReplayRevalidatesAfterRuntimePreparation` | Route replacement before admission blocks the old prompt. Its claim cannot clear a successor. Cover every replay kind through a table. | queued ownership 002.5–6; ceiling 001.6 |
| `TestCeilingReplayPreservesReconciliationPrecedence` | Any working sibling preserves active state. With no working sibling, a valid deferred destination preserves Scheduling. Otherwise cancellation reaches Review. | cancellation 001.2; queued ownership 002.1–2 |
| `TestCeilingReplayClaimAndShutdownProgress` | Concurrent replay/Send Now cannot duplicate delivery. Cancelled attempt cleanup releases only its claim. The sweeper stops after released fixture barriers. | queued ownership 002.3–6; ceiling 001.3, .5–6 |

The first test is the mandatory behavioral RED. Bound its failure and cleanup
so the pre-fix deadlock does not strand the test runner. A subprocess harness is
appropriate when blocked production mutexes cannot be released by fixture cleanup.
Keep all helper entry points private and explicit to avoid recursive test binaries.

Existing `ceiling_replay_test.go`, `route_action_lock_order_test.go`, and
`task_operations_test.go` supply claim, barrier, and cancellation patterns.
Existing claim tests cover expired claims and detached cleanup contexts.

## E2E tests

Run existing queued-session and workflow-cancellation specs on Chromium and
mobile Chrome. These verify visible settlement, exact queued delivery, and
unchanged cancellation policy through the shipped UI. The new Go service test
owns deterministic reproduction of the lock cycle. Browser timing is not the
RED gate for this defect. No rendered UI change or ASCII preview is needed.

## Work orders

- [x] [Task 01: Remove the replay lock cycle](task-01-remove-replay-lock-cycle.md)

## Related delivery records

The [queued-session package](../queued-session-ownership/plan.md) and its Task 02
own the original implementation. This package owns the newly observed lock
regression. Existing PostgreSQL gaps and original work-order statuses remain
unchanged. Their prior passing tests do not prove this concurrency intersection.

The [session ceiling](../session-concurrency-ceiling/plan.md) and
[cancelled-turn completion](../cancelled-turn-completion/plan.md) packages remain
compatibility inputs. Do not reopen their implementation or alter their results.

## Verification results

The [review remediation record](task-01-remove-replay-lock-cycle.md#review-remediation)
supersedes the initial callback-only evidence with the full cancellation and
sweep-progress regression. It also records final dispatch-claim fencing and the
seven-kind admission matrix.

Design and implementation validation on 2026-09-17:

- `python3 scripts/list-docs.py validate`: passed (287 decisions, 1002 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog discovery found the affected requirement and system design.
- `git diff --check`: passed.
- Package status includes both new files and the companion links.
- The mandatory RED test failed behaviorally before the production change by
  timing out while a synchronous dispatch callback reconciled task state.
- The four new replay lock-order tests passed in a focused run.
- The four new replay lock-order tests passed under the race detector for 10
  repetitions.
- Final dispatch-admission coverage passed for all seven replay kinds, including
  successor-claim rejection and route mutation serialization.
- The targeted orchestrator race selection passed.
- The targeted SQLite repository race selection passed.
- Chromium passed 3 tests from the queued-session and cancellation specs.
- Mobile Chrome passed 2 tests from the queued-session and cancellation specs.
- No live instance state changed. The package remains uncommitted.

## Risks

- Removing the outer lock without final admission fencing can dispatch stale work.
- A released context can falsely claim lock ownership in nested dispatch helpers.
- Synchronous event subscribers can re-enter a lock held by their publisher.
- Fixing only the global mutex order leaves the replay/session-guard cycle intact.
- Tests that mock away boot-ready or task publication can miss the production cycle.
- Restart clears in-memory locks but is not a permanent fix or authorized delivery step.
