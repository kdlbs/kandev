---
id: "01-office-native-admission"
title: "Share capacity admission with Office runs and native routes"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-003.1
  - AC-AGENTS-SESSION-CEILING-003.3
  - AC-AGENTS-SESSION-CEILING-003.4
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Office and native-route shared admission

Join Office run-sessions, taskless runs, and native routes to the existing
session-ceiling admission owner. The Office run-session persists its frozen grade
with its own reservation before allocation; the shared census counts that
persisted attempt exactly once, and a capacity wait keeps the original durable
attempt as the continuation owner instead of spending a provider attempt.

## Outcome and scope

Office run admission reserves through the same controller used by task launches
(`orchestrator/run_session_capacity.go`, `backendapp/office_run_capacity.go`).
Taskless Office runs, queued Office run sessions, and task-bound Office runs keep
their existing wait disposition and priority. The queued launch origin and the
resolved workflow selection survive the capacity wait
(`orchestrator/queued_start_selection.go`, `ceiling_origin.go`). A live native
reservation is released only by its owner, not by launch-budget expiry
(`expireStaleReservations`).

The disabled default is preserved: with the automatic-only policy the Office
run-session bridge returns no reservation, and `RunSessionCapacityEnabled` is
false, so no new refusal or deferral appears.

## Out of scope

G4 awaited-stop transports, native utility/review ownership producers (P5/P6),
closure work, and fork-only control UI. Ordered-tier profile selection history is
a separate slice; this task does not import it.

## Entry points

- `orchestrator.Service.ReserveRunSessionCapacity` and
  `RunSessionCapacityEnabled`; `SetRunSessionCapacitySource`.
- `backendapp.officeRunSessionLauncher.reserveOfficeCapacity` and
  `SetExecutionCapacityOwner`.
- `orchestrator.sessionCeilingController.admitAllLocked`,
  `inspectAllAdmissionLocked`, `prepareQueuedTaskCapacityLocked`,
  `consumeQueuedTaskCapacityLocked`.
- `office/scheduler.SchedulerService.deferRunSessionAdmission` and
  `providerAttemptCount` (Office capacity waits are not provider attempts).
- `backendapp.officeCapacityAttempt.prepareCandidate` and
  `office/repository/sqlite.RequeueUnboundRunSession`: a transient candidate
  binding error may defer only when one transaction closes the exact still
  unbound, uncancelled preparing session and requeues its still-claimed run.
  A changed owner, cancellation, other active session or failed transaction
  does not authorize that retry; ordinary errors remain errors. Binding errors
  or a lost candidate CAS can have an unknown owner, so the losing launcher
  does not finalize that session without proof.

## Adaptation and limits

The upstream range was authored on an ordered-tier lineage that the target base
does not carry, so the port adapts it:

- `NativeRouteOwnersEnabled` gates on the existing shared dynamic-routing flag
  instead of an ordered-tier engine flag. When dynamic routing is off, taskless
  dynamic Office runs are refused exactly as before; when it is on, an Office
  dynamic run uses its run-session as the durable route owner. No new toggle is
  introduced.
- The selection-ordinal pieces are excluded because the base has no
  `dynamic_route_attempts.selection_ordinal`. Queued launch origin, workflow
  selection, and existing waiter priority are carried by the ported
  `queued_*_capacity` and `ceiling_origin` code and do not depend on the ordinal.
- Native utility/review admission hooks (`runtime/native_route_capacity.go`,
  `orchestrator/native_route_capacity.go`, `backendapp/native_route_capacity.go`)
  are wired but have no producer in this slice: the only code that builds
  `native:utility-call:` or `native:review-run:` route IDs lives in the excluded
  P5/P6 groups. The Office taskless path builds `native:office-run-session:` and is
  fully connected through `ReserveRunSessionCapacity`.
- The optional `transferNativeRouteLocked` alias behavior remains unverified
  for the eventual P5/P6 producer paths. Do not treat this draft as validation
  of those consumers or their transfer semantics.

## Executed verification

Historical focused build and check results for the original source slice;
they do not by themselves establish the result of a later integration:

```text
go build ./internal/agent/runtime ./internal/orchestrator ./internal/backendapp      pass
go build ./internal/office/... ./internal/runs/models ./internal/runs/repository/... pass
go test ./internal/office/scheduler -run 'TestProviderAttemptCount|TestSameWaitingCandidate' pass
go test ./internal/orchestrator -run 'TestReserveRunSessionCapacityDisabledWhenPolicyOff|TestNativeReservationSurvivesStaleSweep' pass
bash scripts/lint-go-changed <changed go files>   pass (0 issues, each commit)
```

The original source added regression cases to existing test files:
`office/scheduler/run_actor_from_context_test.go` (Office capacity waits do not
consume provider attempts; the deferred launch selection is preserved) and
`orchestrator/session_ceiling_test.go` (opt-in-off Office run-session capacity is
inert; a live native reservation survives the stale sweep). The stacked draft
does not add those persistent test changes: selected cases were run from a
disposable overlay. Its current merge verification is reported separately in
the PR rather than being attributed to the historical source.

The full backend suites still show pre-existing environment failures that are not
this slice: Windows clock equality (`updated_at did not advance`), git
clone/pull/push fixture setup, and timing-sensitive scheduler tests. Those were not
compared against a clean baseline run here and are reported as unverified rather
than attributed to this change.

## Review status

Independent review is requested through the gate RV entry point with the fixed
artifact for the final head. The delivery package is not merged or published by
this work order.
