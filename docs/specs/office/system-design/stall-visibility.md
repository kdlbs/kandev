---
status: current
system: office
requirements:
  - REQ-OFFICE-STALL-VISIBILITY-001
  - REQ-OFFICE-STALL-VISIBILITY-002
  - REQ-OFFICE-STALL-VISIBILITY-003
---

# Office Stall Visibility System Design

## Purpose and boundaries

This design adds two detection-only passes to the orchestrator's existing
stuck-signal watchdog scan. The Office system owns the outcome because the
states are defined by Office primitives; the orchestrator owns the scan loop and
the session lifecycle, which this design reads and constrains but does not
change for non-Office tasks.

Adjacent contracts used and not owned:

- `internal/orchestrator/stuck_signal_watchdog.go` — the scan loop, the
  threshold, the cancellation guards, and the reclaim path.
- `internal/workflow/engine` `ParticipantStore` / `DecisionStore` — already wired
  onto `orchestrator.Service` as `engineParticipants` / `engineDecisions`, both
  step-scoped and nil-safe.
- `internal/office/repository/sqlite` — the `runs` queue.
- `internal/task/repository/sqlite` — the `tasks` table, the
  `workflow_step_participants` table, and `IsFromOfficePredicate`, the exported
  SQL expression that defines Office ownership.

The detector lives beside the existing watchdog rather than in the Office
scheduler for two reasons: the step-scoped participant and decision readers this
detector needs are already wired onto `orchestrator.Service` (the Office
repository's `ListActiveTaskDecisions` is deliberately task-scoped across all
steps), and co-locating keeps one place to look for stall detection.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-STALL-VISIBILITY-001` | [Splitting detection from recovery](#splitting-detection-from-recovery) |
| `REQ-OFFICE-STALL-VISIBILITY-002` | [Decision-waiting detector](#decision-waiting-detector) |
| `REQ-OFFICE-STALL-VISIBILITY-003` | [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- **`stuckSignalDisposition`** (new, `stuck_signal_watchdog.go`) — the single
  predicate both existing gate sites consult. Returns one of three outcomes and
  is the only place the `IsFromOffice` term is evaluated.
- **`surfaceOfficeStalledSignal`** (new) — emits the stranded-signal surfacing
  for an Office task and records the dedupe key.
- **`detectOfficeDecisionWaitingOnce`** (new file,
  `office_decision_stall_watchdog.go`) — the second detection pass, driven from
  the same reaper tick.
- **`officeDecisionCandidateLister`** (new narrow optional interface) — the
  candidate enumeration, implemented by the task repository as
  `ListOfficeDecisionWaitCandidates`.
- **`officeRunInFlightReader`** (new narrow optional interface) — reports whether
  a task has a `queued` or `claimed` run.
- **Office SQLite repository** — implements the reader with a dialect-portable
  JSON predicate.

## Data and contracts

### Shared disposition

```go
type stuckSignalDisposition int

const (
    stuckSignalNotCandidate stuckSignalDisposition = iota
    stuckSignalReclaimable
    stuckSignalSurfaceOnly // Office: visible, never reclaimed
)
```

Both `reconcileWaitingStuckSignalSessionIfDue` and
`stuckSignalCandidate` drop their inline
`task.IsFromOffice` term and call this helper instead. Routing both sites through
one predicate is what makes AC-001.2 and AC-001.3 structurally true rather than
separately maintained: the two predicates are near-identical today, and fixing
one in isolation would leave the other as a silent second gate.

`stuckSignalSurfaceOnly` is returned for an Office task that satisfies every
other candidacy term. Callers treat it as "not a recovery candidate" — the same
control flow the current `false` return produces — after calling
`surfaceOfficeStalledSignal`.

Passthrough sessions continue to return `stuckSignalNotCandidate` from both
paths. The inactivity gate cannot read them, so they are excluded from detection
as well as recovery.

### Run-queue reader

```go
type officeRunInFlightReader interface {
    HasInFlightRunForTask(ctx context.Context, taskID string) (bool, error)
}
```

Follows the optional-interface idiom already used by `stuckSignalSessionLister`
and `promptActivitySessionReader`: asserted at use, nil-safe, so lightweight test
doubles stay compatible. Wired from `internal/backendapp`, where the Office
repository is constructed, keeping the orchestrator decoupled from the Office
packages.

The repository implementation must use
`dialect.JSONExtract(r.ro.DriverName(), "payload", "task_id")` rather than a
literal `json_extract`. The `runs` table has no `task_id` column — task linkage
lives in the payload JSON — and Postgres is a supported driver. Precedent:
`internal/office/repository/sqlite/failure.go` and its
`failure_postgres_test.go`. The status predicate is `status IN ('queued',
'claimed')`; the table has no `running` status.

### Decision and participant reads

- Seats: `engineParticipants.ListStepParticipants(ctx, stepID, taskID)`, filtered
  to `DecisionRequired` seats with role `reviewer` or `approver`.
- Decisions: `engineDecisions.ListStepDecisions(ctx, taskID, stepID)`.

Both are step-scoped, which is what AC-002.1 requires. `ListStepDecisions`
returns superseded rows alongside active ones by its own documented contract
(quorum guards filter at the call site), and `SupersedeTaskDecisions` leaves a
reworked task's prior decision rows in place with `superseded_at` set rather
than deleting or re-scoping them. The detector must therefore filter on
`DecisionInfo.SupersededAt` itself — treating any non-empty result as
"decided" reads a re-entered step as permanently decided after the first
rework round, which is exactly the repeat stall this detector exists to find.

## Control flow

Both passes run inside `reclaimStuckSignalSessionsOnce`, on the idle reaper's
existing 30-second ticker. No new goroutine.

1. Existing per-session loop. For each session, the disposition helper decides
   `notCandidate` / `reclaimable` / `surfaceOnly`. `surfaceOnly` surfaces and
   continues; `reclaimable` follows the unchanged reclaim path.
2. A second pass enumerates candidates, then applies the remaining
   decision-waiting predicate to each: current step has a decision-required
   reviewer/approver seat → no decision recorded for (task, step) → no in-flight
   run. Order matters: the run check is the most expensive term and is evaluated
   last.

The two passes are siblings on the reaper tick, not nested: the decision-waiting
pass is a separate call in `startIdleSessionReaper`'s tick closure, because a
task waiting on a decision need not have any session at all and so cannot be
found by iterating sessions.

The decision-waiting pass is bounded by the same `stuckSignalScanBudget` already
guarding the tick, and shares its "defer the remainder to the next tick"
behavior so it cannot starve `reclaimIdleSessionsOnce`.

### Candidate enumeration

The driving query lives in the task repository, because it reads `tasks` and
must define Office ownership with `IsFromOfficePredicate` — the same expression
that backs `models.Task.IsFromOffice`. A second, hand-rolled definition of
"Office task" would drift, which is exactly the class of bug this feature
exists to remove.

```go
ListOfficeDecisionWaitCandidates(ctx, quietSince) ([]models.OfficeDecisionWaitCandidate, error)
```

It selects only the cheap, indexable half of the predicate: not archived, not in
a terminal state, at a step carrying a `decision_required` seat (template-level
or that task's own override), Office-owned, not a config-mode task, and
`updated_at < quietSince`. Everything else is judged in the orchestrator so each
rejection has its own countable reason and each unreadable input can fail closed
independently. The result is ordered oldest-first and capped, so a truncated
scan drops the freshest candidates and the next tick still sees them.

**Age anchor.** `tasks.updated_at` is an approximation of "when the task entered
this step". An unrelated edit to the task row restarts the clock and delays
surfacing. That error is one-directional — it under-surfaces, never
over-surfaces — which is the correct direction for a detector whose stated
non-goal is acting on what it finds. The exact anchor is
`task_step_transitions.occurred_at`, which has no read API yet; tightening to it
is a follow-up, not a correction.

### Threshold

The decision-waiting pass uses its own constant,
`officeDecisionWaitingThreshold`, set provisionally to 60 minutes. It must not
reuse `stuckSignalWatchdogThreshold`. The two measure unrelated phenomena: a
session that has emitted no events for ten minutes is anomalous, whereas a
decision waiting on a human for ten minutes is ordinary. Sharing the constant
would also mean a future tuning of session silence silently retunes
decision-waiting. Sixty minutes is still plausibly normal but worth counting; a
first alert that cries wolf gets muted, and a muted detector is worth nothing.

## Failure and recovery

This capability has no recovery path by design (REQ-OFFICE-STALL-VISIBILITY-003).
Every failure mode resolves to "do not surface, record the skip":

- Reader unwired or type assertion fails → skip. Absence of the run reader must
  never be read as absence of runs; that would surface every decision-waiting
  task including healthy ones.
- Reader returns an error → skip and count. Unknown is not "no run in flight".
- Participant or decision store unwired, or either read failing → skip and
  count. Mirrors the engine's existing `ReasonParticipantStoreUnwired` /
  `ReasonDecisionStoreUnwired` discipline.
- Candidate lister unwired, or the enumeration query failing → skip the whole
  tick and count. The next tick retries.

The asymmetry with the existing watchdog is deliberate. The reclaim path fails
closed because a wrong reclaim corrupts a live session; these passes fail closed
because a wrong alert trains operators to ignore the signal.

## Persistence

No schema change. No migration. No new table.

The stranded-signal dedupe key is in-memory on `orchestrator.Service`, keyed by
session ID, step ID, and the signal's `SignaledAt`, and dropped when the signal
clears or the session is reclaimed. A backend restart re-surfaces each still-
stranded signal once; that is acceptable and preferable to a schema change,
because the surfaced state is itself derived and cheap to recompute.

The decision-waiting pass is stateless per tick. Re-surfacing is suppressed by
the same in-memory dedupe keyed by task ID and step ID.

## Security

No new trust boundary. Both passes read state the orchestrator already reads
within the same process, and neither exposes a new endpoint. Surfaced payloads
carry task, session, and step identifiers plus a duration; they must not carry
prompt text, agent output, or decision notes.

## Observability

Structured logs, at `Warn`, on the existing watchdog logger:

- `office stall watchdog: Office task holds a stranded completion signal` with
  `task_id`, `session_id`, `step_id`, `signal_age`, and `gate` (which of the two
  sites observed it).
- `office stall watchdog: Office task is waiting on a decision that has not
  been recorded` with `task_id`, `step_id`, `task_updated_at`, `quiet_for`, and
  `threshold`.

Counters, following the `expvar.NewMap` + `metricLabel` convention already used
by `internal/office/scheduler/metrics_vars.go` and exposed through `/debug/vars`:

- `office_stall_stranded_signal_total`, labelled by `gate`.
- `office_stall_decision_waiting_total`.
- `office_stall_detector_skipped_total`, labelled by `reason`
  (`run_reader_unwired`, `run_reader_error`, `participant_store_unwired`,
  `decision_store_unwired`, `participant_read_failed`, `decision_read_failed`,
  `candidate_lister_unwired`, `candidate_list_failed`), so a silently degraded
  detector is distinguishable from a genuinely quiet system.

## Related decisions

None. This design adds no architectural decision beyond the existing watchdog
contract; the Office recovery exclusion it preserves was established by PR #2963.
