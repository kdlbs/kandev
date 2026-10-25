---
created: 2026-10-18
status: done
requirements:
  - REQ-OFFICE-SCHEDULER-002
system_design:
  - ../../specs/office/system-design/scheduler-01.md
legacy_specs: []
---

# Implementation Plan: Hold Office assignment wakes behind pending blockers

## Overview

Repair [issue #4546](https://github.com/kdlbs/kandev/issues/4546) at Office assignment
admission. Extend the existing scheduler requirement and shared eligibility design;
implement and verify one vertical change in one sequential work order after a later
explicit implementation release to this same primary session.

Office owns autonomous wake policy. Task/workflow storage supplies edges, states,
and runner seats; the shared runs service still owns persisted run admission.
The authoritative pair is [scheduler requirements](../../specs/office/requirements/scheduler.md)
and [scheduler design](../../specs/office/system-design/scheduler-01.md#assignment-wake-step-eligibility).
No separate repair specification or ADR is needed: this restores the existing
subtask-sequencing contract within its existing owner and boundaries.

## Confirmed root cause and admission evidence

At admission, `shared.IsAssignmentWakeEligible` read only the step auto-start policy.
The event, recovery, and assignee-reactivity producers therefore admit an assignment
while an already-persisted prerequisite is unfinished. Task creation already writes
its blocker edges before publishing `TaskCreated`; reordering that producer is unnecessary.

Admission base: clean managed branch `feature/hold-office-assignme-crx` at
`5df3c35d1374d2327d846006a2137c8709b9f05e`, equal to authoritative remote main on
2026-10-18. All 11 qualification dependency blobs matched once. ROOT's corrected
qualification at `97b8e0d302e2eddb3939bd977659fce777bc82a4` remains applicable; it was
not replayed here. Evidence read only:
`/tmp/kandev-root-office-blocker-qualification-ldbf7mit/fixture-corrected/qualified-proof.json`,
`receipt.json`, `startup.json`, `stdout.log`, `stderr.log`, and `preparation.json`.

The corrected fixture used real settings/task/workflow/Office constructors and
writers with FK enforcement and a real CLI parent plus explicit AgentID. The real
`TaskCreated` bus -> handler -> queue -> shared runs service persisted **one**
`task_assigned` run with an IN_PROGRESS prerequisite, where zero was required.
Three controls passed: no blocker -> one; completed blocker -> one; non-auto-start
backlog -> zero. Corrected execution receipt: native 95897, join d68155, wrapper 0,
Go 1, package 0.881s. The receipt's `qualified:false` is its raw failing-Go result;
`qualified-proof.json` records the later causal qualification. The original fixture
failed before the producer on a missing CLI FK parent and is not causal evidence.

No actual agent/provider launch was proved. Recovery and reactivity were source
inspections at qualification; this work order must test their actual producer paths.
ROOT's private candidate source and runner are not implementation inputs. Write
permanent tests independently only after implementation release.

## Scope

In scope:

- One shared authoritative blocker check alongside existing step eligibility.
- Existing event-created/updated, unstarted recovery, and assignee mutation producers.
- Known-pending precedence over step uncertainty or another unreadable blocker.
- Existing blocker-resolution release and recovery quota/no-stranding controls.
- A committed Office success-event bridge to the existing sessionless blocker cascade;
  cover normal MoveTask prewrites without changing completion/approval guards.
- Structural onboarding lookup compatibility, because it is another existing helper
  caller; its creator accepts no blocker input and needs no new behavior.

Out of scope:

- FAILED/CANCELLED/archive/missing-blocker policy reconciliation (including #4548),
  #4553 and PR #4559, generic Kanban changes, and unrelated automatic launch paths.
- Claim-time checks, global scheduler redesign, blocker/queue transactions, schema
  changes, runtime flags, provider work, metrics projects, UI/copy, or lifecycle cleanup.
- Delegation, extra sessions/tabs, model switches, and protected work/resource cleanup.

## Technical approach

Extend the structural lookup used by `IsAssignmentWakeEligible` with the Office
repository's `ListTaskBlockers` and `IsTaskInTerminalStep`. Keep all three existing
producer calls on that single predicate. If needed, rename the lookup type to
reflect its broader responsibility and embed it in onboarding's `Repository`.
Do not add optional type assertions that silently bypass the gate for a real producer.

Combine step eligibility and blocker readiness. Keep the existing step-only helper
logic separately if that avoids its early fail-open returns bypassing the blocker
check. Reuse the terminal reader rather than importing task service or writing a
second state classifier. Failed reads log Warn and preserve prior assignment
admission, but scanning must still find any positively known pending blocker.
A known ineligible step still refuses admission even if blocker reads fail.

Keep the previous-assignee interrupt ahead of the gate. Keep runner writes,
assignment occurrence generations, keys, rate/pause/causation behavior, and run
admission unchanged. A held wake produces no run or placeholder. Recovery retains
its candidate lookback and continues beyond blocked candidates without using the
admitted quota. Preserve both existing resolution producers and their keys. Add only the committed
completion bridge specified below; the earlier assumption that existing wiring
always released sessionless held work was disproved by the MoveTask diagnostic.

### Corrective completion bridge

Keep REQ-OFFICE-SCHEDULER-002 and its nine criterion IDs. AC002.7 already requires
normal final completion to release held work; clarify its valid mixed-identity case. The independent diagnostic confirmed
that MoveTask commits COMPLETED before the orchestrator terminal consumer returns
at its existing terminal guard. The legacy Office moved-event consumer cannot infer
completion from modern ID-only payloads. Direct canonical Office completion passed.

Add `scheduler/task_completion.go`: `RegisterTaskCompletionSubscriber` and its
successful committed-state handler. Register exactly once in
`backendapp.activateOfficeServices`, after dependencies are wired, and fail activation
on subscription error. Reject malformed/missing transition evidence, non-COMPLETED
new state, already-COMPLETED old state, and a persisted source no longer COMPLETED.
Do not require source Office identity: task-service validateDependencyPair allows
same-workspace Kanban prerequisites for Office dependents. Office admission belongs
to the dependent. Reuse `cascadeBlockersResolved` with a narrow callback checking
that each selected dependent is Office-owned in the prerequisite workspace before
`QueueRunCtx`. Existing cascade code and all other callers remain unchanged.

Reuse current blocker terminal reads, runner selection, digest key, payload and
system actor/root causation. Duplicate delivery and canonical Dashboard completion
plus its subsequent state event must yield one persisted resolution run for the
same dependent/runner/blocker set. No status rewrite, approval rerun, dependent
session, deferred launch intent, recovery extension or global dispatcher change.
Read/queue failures are reported through existing error/logging paths; this adds
no durable retry or new failure-policy promise.

Compatibility constraints come from the completed
[step-eligibility package](../office-assignment-step-eligibility/plan.md), existing
[rate-limit package](../office-assignment-wake-rate-limit/plan.md),
[pause/replay package](../office-paused-assignment-replay/plan.md), and
[global scheduler ADR](../../decisions/2026-08-01-global-run-scheduler-ownership.md).
Those historical packages are inputs, not work to reopen or relabel complete here.
The task system's successful-only dependency policy is separate from the current
Office terminal reader; retain that known policy difference for separate work.

## Tests

The following tests were written independently, using per-test temporary databases with
real schema constructors, real workspace/CLI/profile/workflow/task/blocker writers,
FK enforcement, and real run admission. Assert run reason, task/agent identity,
counts, keys where applicable, and persisted runner/generation preservation.

| Criteria | Planned test and producer evidence |
| --- | --- |
| 002.1-.6, .9 | `service/assignment_blocker_admission_test.go`: `TestOfficeAssignmentBlockerAdmission` drives real TaskCreated and TaskUpdated bus subscribers. Pending and completed+pending sets yield zero; no blockers/all completed yield one; Backlog remains zero. Include generation-present and stored-runner fallback cases. |
| 002.1-.6, .8-.9 | `service/assignment_blocker_recovery_test.go`: `TestOfficeAssignmentBlockerRecovery` invokes `OfficeRecoveryHandler.Tick`; five held candidates ahead of an eligible task cannot starve it. Recheck held task after blocker completion within the existing recovery window. |
| 002.1-.6, .9 | `service/assignment_blocker_admission_test.go`: `TestOfficeAssignmentBlockerMutation` drives `ApplyTaskMutation` and its real queue. Reassignment still interrupts the old runner while held; same-runner assignment has no interrupt; admitted positive controls retain key/actor/generation behavior. |
| 002.7 (original producers) | `service/assignment_blocker_resolution_test.go`: `TestOfficeAssignmentBlockerResolution` starts with zero assignment runs, completes two blockers through real writers/mutations, asserts zero after the first and one persisted `task_blockers_resolved` after the final. Exercise the service completion event through its actual dispatcher to real run admission as well, not just a spy callback. |
| 002.3-.6, .8-.9 | `shared/assignment_blocker_eligibility_test.go`: `TestAssignmentBlockerEligibility` covers empty/missing/error step policy, blocker-list/state errors, and both unknown/pending orders. The real bus admission matrix covers FAILED/CANCELLED, archived terminal/nonterminal, and missing-row compatibility. Use narrow fakes for error injection; real producer tests remain mandatory. |

Corrective `service/assignment_blocker_resolution_test.go` test
`TestOfficeAssignmentBlockerCommittedCompletion` reuses the existing real fixture.
Drive actual task-service MoveTask and the registered completion subscriber into
real queue admission, with no dependent session/intent and a 48-hour-old task
excluded by the actual 24-hour recovery query. Complete two prerequisites: zero
runs after the first, one after the final. Cover canonical Dashboard completion,
its following canonical state event, duplicate delivery, same digest key/current
runner and unchanged generation/system actor/root causation. Include a valid Kanban-prerequisite/Office-dependent positive control through
the real dependency writer/validator, and generic Kanban dependent negative controls
(even with a runner), wrong-workspace, malformed,
stale/reopened and non-success events. Keep the original resolution controls.
The earlier actual orchestrator-terminal-consumer diagnostic remains causal evidence;
port the changed-consumer regression into the shared permanent fixture rather than
copying its temporary setup. No synthetic named TaskMoved event substitutes for the
normal MoveTask producer in this new regression.

Existing step, rate-limit, pause/replay, causation, blocker-key convergence, and
onboarding tests run in their complete owning packages. Extend a new test file if a
policy control is absent. Avoid SQL-text-only/source-shape regressions, sleeps,
manual stand-in task schemas, or assuming bus Publish proves asynchronous delivery.
Use observable completion barriers and cleanup for subscriptions/goroutines.

## End-to-end evidence

The backend integration tests cover persisted edges -> real producer -> actual run
admission, including final-resolution release. No rendered UI changes or Playwright
suite is needed. Agent/provider execution is neither required nor claimed.

## Work orders

- [x] [Task 01: Gate assignment admission on pending blockers](task-01-blocker-admission.md)

One sequential work order; no implementation delegation is authorized. Producer
regressions share a real fixture in the external service test package, which calls
the scheduler public mutation API as well as service events and recovery. This
avoids duplicating schema setup; all four owning packages retain race coverage.

## Documentation impact

Internal scheduler requirements/design and this delivery package change during
planning. Public task documentation already describes dependency deferral; Office
surfaces are explicitly provisional. This restores documented intent without new
API/configuration/terminology. No public-page edit is planned. Reassess only if the
released implementation changes a public contract.

## Verification results

Assignment admission and the committed-completion correction are complete. Independent causal RED exposed unwanted persisted assignment
admission from all three producers while positive controls passed. Focused GREEN
passed six top-level tests and 51 subtests. One complete race pass passed shared
(2.639s), service (18.247s), scheduler (2.299s), and onboarding (1.292s).

Scoped actual-Go-path lint passed with zero issues after replacing a deprecated
Run alias in the fixture. The affected producer tests passed again after that
import/type correction; no production logic changed and the complete package pass
was not repeated. Catalog, all-spec lint and whitespace checks exited 0. Actual
runtime documentation coverage is `covered` with `errors: []`; the earlier docs-only
`exempt` result remains historical. [Work-order Results](task-01-blocker-admission.md#results)
records commands, setup corrections, logs, actual handles/joins and resource budgets.

Corrective diagnostic: setup-only handle18676/join1b3686 failed compilation; corrected
handle76226/join44b67d exited 1 causally. Real MoveTask committed COMPLETED then
produced zero resolution runs; direct terminal-consumer/Dashboard/cascade control
produced one. Both diagnostic groups joined and are absent. Detailed receipts and
limits are in the existing work order. Permanent real-producer RED reproduced both
Office and valid Kanban prerequisites, while canonical Dashboard completion passed.
The implemented subscriber passes focused completion/identity/convergence controls.
One complete correction-owning race pass passed service (21.347s), scheduler (2.292s)
and backendapp (171.627s). Scoped lint of the three actual correction Go paths
passed with zero issues. The work order records actual RED/GREEN, fixture correction,
package/lint joins, logs and checksum cleanup; original shared/onboarding evidence
was retained without replay.

No actual agent/provider execution was claimed. All local test/lint command groups
are absent after their actual joins. Normal active hooks and hosted delivery receipts
are maintained in the external task record.

## Risks

- An unknown blocker read retains prior assignment admission; it can misfire during
  a store fault. A known pending blocker must always win, even after an unknown read.
- This admission-time check does not synchronize later blocker mutations or cancel
  runs admitted earlier. Do not claim claim-time or concurrency linearizability.
- Completion near assignment can exercise existing independent run causes. Preserve
  current keys and coalescing rather than inventing a new cross-cause guarantee.
- Recovery remains bounded by existing task eligibility/lookback. The normal final
  blocker-resolution producer must be proved so held work does not depend solely
  on a later recovery sweep.
