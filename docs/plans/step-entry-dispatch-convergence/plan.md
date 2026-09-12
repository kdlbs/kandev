---
created: 2026-09-02
status: in-progress
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-002
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-003
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-004
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-005
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-006
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-007
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-008
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
legacy_specs:
  - ../../specs/workflow-on-enter-action-dispatch/spec.md
---

# Implementation Plan: Step Entry Dispatch Convergence

## Overview

Two dispatchers execute a step's declared entry sequence on the same committed
arrival, and `clear_decisions` and `queue_run_for_each_participant` are handled
by both. Because the paths derive idempotency keys from different identifiers,
the runs queue does not collapse the duplicate, so the shipped `office-default`
Review and Approval steps fan out twice per arrival and quorum can count two
decisions for one round.

The ledger dispatcher becomes the single owner of every session-independent
kind, and the marker machinery is moved onto it rather than deleted. The order
below is forced by that decision: the ownership declaration is the table both
dispatchers read, so it lands first; the allocated marker position set must be
persisted before terminality can be derived from it; the reclaim and skip
primitives must exist before the startup scan can consume them; and the
diagnostics work depends on the skip primitive because an unwired adapter at a
marker-bearing position must reach a terminal marker rather than strand its
entry.

Turn-completion re-keying and the coalescing exclusion do not depend on
convergence, but they share files with it and are sequenced after the work that
touches those files.

## Scope

### In scope

- One ownership declaration mapping each action kind to its owning dispatcher
  and to whether it is marker-bearing, read by both dispatchers.
- Ledger-path ownership of the five session-independent kinds, including the
  marker claim, the abandon-remainder stop rule for marker-bearing positions,
  and the two-branch idempotency key.
- Unconditional entry allocation for a step declaring a marker-bearing kind,
  with the allocated position set persisted on the entry row.
- A fourth terminal marker state `skipped` with a recorded reason, and a
  state-predicated reclaim path distinct from the live-dispatch claim.
- A single startup recovery scan that retires or resumes stranded entries.
- Task-scoped turn-completion serialization, retaining the session-keyed
  redelivery check.
- Coalescing exclusion for entry-triggered runs, carried in memory and on the
  run row.
- Per-participant fan-out reporting, declaration-fault handling, and the
  warning at every discard point.

### Out of scope

- Ordering guarantees across owners. No shipped workflow declares a step whose
  entry sequence mixes owners, and interleaving the synchronous ledger path
  with the marker path's goroutine is a larger change than this design carries.
- Extending durability to kinds that do not carry a marker today. Marker-bearing
  stays at `clear_decisions` and `queue_run_for_each_participant`.
- Cross-process serialization. Both turn-completion maps are in-process
  `sync.Map`, so REQ-003's guarantee is scoped to one backend process.
- A migration for runs enqueued under the pre-convergence idempotency key.
- Any change to `docs/specs/workflow-on-enter-action-dispatch/spec.md`.

## Technical approach

### Ownership declaration

One exported table in `apps/backend/internal/workflow/stepentry` maps action
kind to owning dispatcher and to marker-bearing, as two independent columns.
`Engine.DispatchStepEntry` (`internal/workflow/engine/entrydispatch.go`) and
`dispatchOnEnterActions` (`internal/orchestrator/event_handlers_workflow.go`)
both read it; neither keeps a private list. The ownership column is seeded from
`sessionIndependentActionKinds` and the marker-bearing column from
`IsEngineOwnedOnEnter`; the seeds are different functions and are not
interchangeable. `configure_session` is written in by hand because
`CompileOnEnterAction` emits no `ActionKind` for it. The table is wider than
either engine map, so `sessionShapedActionKinds` is not edited.

### Ledger ownership, claim, and stop rule

`Repository.dispatchStepEntry` claims through `ClaimStepEntryMarker` before
executing a marker-bearing kind. `idempotencyKey` keeps both branches: a step
declaring at least one marker-bearing kind keys from the
`workflow_step_entries` row id on every route; a step declaring none keys from
the step-transition row id. Which branch applies is read from the ownership
declaration, not from whether an entry row exists. The ledger dispatcher
abandons the remainder of an entry's sequence when a marker-bearing position
fails or loses its claim, narrowing `Engine.DispatchStepEntry`'s
record-and-continue contract to marker-bearing positions only.

### Entry allocation and the position set

`workflow_step_entries` gains `marker_positions TEXT NOT NULL DEFAULT ''` via
`r.migrate.Apply` in `internal/task/repository/sqlite/base_migrations.go`.
`BuildPendingAllocation` already computes the marker-bearing position set to
decide whether to allocate; it now returns it, and
`allocateStepEntryIfPending` persists it inside the transition transaction.
Terminality is derived from the stored set, never re-read from the declaration.

### Marker states and reclaim

`workflow_step_entry_markers` gains `skip_reason TEXT`.
`SkipStepEntryMarker` upserts `state='skipped'` guarded by `WHERE state <>
'done'`. `ReclaimStepEntryMarker` is a state-predicated compare-and-set over
`state IN ('in_progress','failed')`, reporting from the affected row count.
`ClaimStepEntryMarker` is unchanged and still refuses any existing row.

### Startup recovery scan

A single scan in `startSchedulingRuntime`
(`internal/backendapp/main.go`), after the repositories are constructed and
before the runs scheduler and engine dispatcher serve, joins
`workflow_step_entries` with its markers, orders by `(task_id, step_id,
entry_seq)`, and applies the skip rules. No workflow declaration is loaded for
selection. Every failure inside the scan is logged and skipped; the scan never
blocks startup.

### Coalescing exclusion

`QueueRunRequest` gains `EntryTriggered bool` in both `internal/runs/service`
and `internal/workflow/engine`. `shouldCoalesceRun` returns false when it is
set. Because `CoalesceRun`'s subquery reads persisted columns only, `runs`
gains `entry_triggered INTEGER NOT NULL DEFAULT 0` via `r.migrate.Apply` in
`internal/office/repository/sqlite/base.go`, the insert path writes it, and the
subquery gains `AND entry_triggered = 0`.

### Migrated column probes

`runs.entry_triggered`, `workflow_step_entries.marker_positions`, and
`workflow_step_entry_markers.skip_reason` are each probed once at startup with
the `columnExists` shape `runs.outcome` uses. A failed probe emits one ERROR
and takes the degraded path rather than issuing a statement that cannot
succeed. A probe that errors for any other reason is treated as absent.

### Diagnostics and fan-out

The warning helper takes workflow id, step id, step name, and action type and
is called from all four discard points. The `ErrActionNotYetWired` callback is
replaced by an unwired check ahead of the claim, which for a marker-bearing
position writes a terminal `skipped` marker with reason `adapter_unwired`.
`queue_run_for_each_participant` gains a logger and emits per-participant ERROR
records plus one INFO summary. `roleSeatsForFanOut`'s `Position ASC,
AgentProfileID ASC` order is kept unchanged, because
`review-participant-seats.md` AC-OFFICE-REVIEW-SEATS-003.3 binds it.

## Tests

| Acceptance criteria | Evidence |
|---|---|
| 002.1, 002.2, 002.3, 002.6, 002.7 | `internal/workflow/stepentry/stepentry_test.go`, `internal/workflow/engine/entrydispatch_test.go` |
| 002.4, 002.10 | `internal/orchestrator/step_entry_dispatch_test.go` stop-rule case: fail `clear_decisions` at position 0 of the shipped Review sequence, assert the position-2 fan-out enqueues nothing |
| 002.5 | `internal/orchestrator/review_participant_seats_acceptance_test.go`, asserting an empty queue after the expected run |
| 002.8, 002.9, 008.2 | `internal/task/repository/sqlite/step_entries_test.go`, asserting two entries into one step yield different keys |
| 003.1, 003.2, 003.3, 003.4, 003.6 | `internal/orchestrator/step_entry_dispatch_concurrent_test.go` |
| 003.5 | new two-session-one-task case in `internal/orchestrator/step_entry_dispatch_concurrent_test.go` |
| 004.1, 004.3, 004.4, 004.5, 004.6, 004.12 | new `internal/office/service/step_entry_recovery_test.go` |
| 004.2, 004.11 | probe-failure case in the same file, asserting startup proceeds |
| 004.7, 004.8, 008.4 | `internal/task/repository/sqlite/step_entries_test.go`, distinguishing `ClaimStepEntryMarker` from `ReclaimStepEntryMarker` on the same row |
| 004.9, 004.10 | `internal/task/repository/sqlite/step_entries_test.go`, allocating at a step with a non-marker-bearing position and asserting `marker_positions` omits it |
| 005.1, 005.3, 005.4, 005.5 | `internal/runs/service/service_test.go` |
| 005.2, 005.6 | `internal/office/repository/sqlite/runs_test.go`, covering the target direction |
| 006.1, 006.2, 006.3, 006.5 | `internal/orchestrator/event_handlers_workflow_on_enter_warn_test.go` |
| 006.4 | same file: a marker-bearing position reaches terminal `skipped` with reason `adapter_unwired` and is not re-selected |
| 007.1 to 007.6, 007.8, 007.10, 007.11 | new `internal/workflow/engine/phase2_callbacks_test.go` |
| 007.7, 007.9 | same file, with a fixture whose agent-profile-id and row-creation orders disagree |
| 008.1 | `internal/orchestrator/step_entry_dispatch_atomic_test.go`, with the failure injected inside the repository method |
| 008.3 | `internal/task/repository/sqlite/step_entries_test.go`, asserting neither the entry row nor the step change survives |

## E2E tests

No Playwright coverage is added. Every criterion in this work package is a
backend dispatch, persistence, or logging guarantee with no UI surface; the
user-visible symptom, a duplicated reviewer fan-out, is asserted at the
production-shape boundary by
`internal/orchestrator/review_participant_seats_acceptance_test.go`, which wires
both dispatchers.

## Work orders

- [x] [Task 01: Ownership declaration for step-entry action kinds](task-01-ownership-declaration.md)
- [x] [Task 02: Persist the allocated marker position set](task-02-marker-positions.md)
- [x] [Task 03: Converge dispatch onto the ledger dispatcher](task-03-ledger-ownership.md)
- [ ] [Task 04: Terminal skipped marker state](task-04-skipped-marker-state.md) — deferred, see below
- [ ] [Task 05: Marker reclaim path](task-05-marker-reclaim.md) — deferred, see below
- [ ] [Task 06: Startup step-entry recovery scan](task-06-recovery-scan.md) — deferred, see below
- [ ] [Task 07: Task-scoped turn-completion serialization](task-07-task-scoped-turn-completion.md) — deferred, see below
- [ ] [Task 08: Entry-triggered runs bypass coalescing](task-08-coalescing-exclusion.md) — deferred, see below
- [ ] [Task 09: Discarded step-entry action diagnostics](task-09-discard-diagnostics.md) — deferred, see below
- [ ] [Task 10: Participant fan-out reporting](task-10-fanout-reporting.md) — deferred, see below

### Bounded-scope decision (2026-09-02)

This Build round completed Tasks 01-03 only — the ownership declaration, the
persisted marker-position set, and ledger convergence of `clear_decisions` /
`queue_run_for_each_participant` (including the AC-002.10 abandon-remainder
stop rule) — and stopped there rather than attempting all ten work orders in
one pass. Tasks 01-03 are the fix for the actual reported production defect
(office-default's Review/Approval steps double-dispatching per round); Tasks
04-10 add durability/observability/serialization hardening on top of a
now-correct dispatch path, each independently shippable and independently
tested against its own AC set. Attempting all ten with full TDD rigor,
migration coverage, and the full DoD gauntlet in one continuous session
was judged out of reasonable scope; splitting at the 01-03 boundary keeps the
delivered slice coherent (every AC in REQ-002 that's addressed is fully
addressed, nothing half-wired) and leaves 04-10 exactly as scoped by their
existing work-order files for a follow-up round. No frozen requirement or
system-design content changed to justify this boundary — it is a delivery
sequencing choice, not a scope reduction of the spec itself.

Within Task 02, "unconditional entry allocation" (extending allocation to
every write chokepoint, not just the existing `on_turn_complete`
ResultHolder-gated route in `workflow_store.go`/`applyTransition`) was also
narrowed: allocation stays on that one existing route. That route is exactly
where the production bug manifests (Review/Approval steps entered via
`on_turn_complete`), so the fix lands regardless. Other routes (manual step
move via `finalizeStepEnter`, WIP-promotion, workflow-switch) do not get an
allocated entry this round; a marker-bearing action reaching
`Engine.DispatchStepEntry` with `markerEntryID == 0` on those routes falls
back to the pre-convergence unprotected direct-execution path, unchanged from
today. Extending allocation to those routes is folded into Task 04's
remaining scope (terminal `skipped` state depends on knowing the full
declared position set on every route, which unconditional allocation would
provide).

Two pre-existing tests from an earlier build round
(`TestProcessOnEnter_ClearDecisionsFailure_BlocksSubsequentOnEnterActions` and
the renamed `TestProcessOnEnter_ClearDecisionsFailure_DoesNotBlockEarlierAutoStart`,
both in `step_entry_dispatch_test.go`) were updated during this round: the
first to match the ledger-side abort log line
(`dispatchEngineOwnedOnEnterAction`'s "DispatchStepEntry: marker-bearing
on_enter action failed") now that `clear_decisions` executes exclusively
through the ledger; the second inverted its assertion (auto-start now *does*
launch despite a same-entry `clear_decisions` failure) because the system
design's own "Out of scope: Ordering between actions owned by different
dispatchers" section states this cross-owner ordering was never a guarantee
this initiative provides — `auto_start_agent` is marker-owned and
`clear_decisions` is ledger-owned, and the two dispatch independently with no
barrier between them.

## Verification results

- `go build ./...`, `go vet ./...` — clean.
- `gofmt -l` on every changed file — clean.
- `golangci-lint run ./... --new-from-rev=646ff0063f5656ad2f0358fb9e01e39378ac113f` — 0 issues.
- `go test ./internal/orchestrator/... ./internal/backendapp/...` — all pass.
- `go test ./...` (full backend suite) — all pass except pre-existing,
  unrelated failures in `internal/worktree`, `internal/launcher`,
  `internal/agentctl/server/{process,config,api}`,
  `internal/agent/{managedruntime,runtime/lifecycle,settings/controller}`,
  `internal/common/config`, `internal/task/{service,handlers}`,
  `internal/system/storage/workspaces` — all traced to this macOS sandbox's
  `/var` -> `/private/var` symlink tripping a symlinked-path safety check in
  each package's `t.TempDir()`-based tests; confirmed unrelated to this
  change (none of those packages import `internal/task/repository/sqlite`,
  `internal/workflow/engine`, or `internal/orchestrator`'s step-entry code).

## Risks

- Moving ownership changes idempotency keys, so a deployment landing mid-round
  can duplicate one round's runs once. No migration is proposed.
- Allocation runs on the pre-existing `on_turn_complete` route only this round
  (Bounded-scope decision above), so the entry-row growth this PR ships is
  limited to a route that already allocated. Extending unconditional
  allocation to the other routes (manual move, WIP promotion, workflow
  switch) — which would add a row on routes that previously allocated
  none — is deferred to Task 04.
- The task lock widens a critical section that currently admits concurrent
  sessions of one task, making contention visible where it was previously
  silent corruption.
- Three `ALTER` migrations run on existing databases and `MigrateLogger.Apply`
  swallows failure at WARNING. A backend whose `marker_positions` ALTER failed
  runs indefinitely with no startup recovery and one ERROR as its only signal.
- `marker_positions` is a denormalized copy of the ownership declaration at
  allocation time, so an entry allocated before a kind's marker-bearing
  property changes keeps the old set and is retired as `digest_changed` rather
  than resumed.
- Seeding either ownership column from the other's function inverts the table.
  The ownership seed is `sessionIndependentActionKinds`; the marker-bearing
  seed is `IsEngineOwnedOnEnter`, whose name reads like an ownership predicate
  but reports what the marker system dispatches.
