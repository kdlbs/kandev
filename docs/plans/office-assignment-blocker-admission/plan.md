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

`shared.IsAssignmentWakeEligible` currently reads only the step auto-start policy.
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
- Structural onboarding lookup compatibility, because it is another existing helper
  caller; its creator accepts no blocker input and needs no new behavior.

Out of scope:

- FAILED/CANCELLED/archive/missing-blocker policy reconciliation (including #4548),
  #4553 and PR #4559, generic Kanban changes, and new automatic launch paths.
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
admitted quota. Do not alter either existing blocker-resolution producer or its
workflow trigger/key semantics.

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
| 002.7 | `service/assignment_blocker_resolution_test.go`: `TestOfficeAssignmentBlockerResolution` starts with zero assignment runs, completes two blockers through real writers/mutations, asserts zero after the first and one persisted `task_blockers_resolved` after the final. Exercise the service completion event through its actual dispatcher to real run admission as well, not just a spy callback. |
| 002.3-.6, .8-.9 | `shared/assignment_blocker_eligibility_test.go`: `TestAssignmentBlockerEligibility` covers empty/missing/error step policy, blocker-list/state errors, and both unknown/pending orders. The real bus admission matrix covers FAILED/CANCELLED, archived terminal/nonterminal, and missing-row compatibility. Use narrow fakes for error injection; real producer tests remain mandatory. |

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

Implementation complete. Independent causal RED exposed unwanted persisted assignment
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
