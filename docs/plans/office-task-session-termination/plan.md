---
created: 2026-09-08
status: implemented
requirements:
  - REQ-OFFICE-SESSION-TERM-001
  - REQ-OFFICE-SESSION-TERM-002
  - REQ-OFFICE-SESSION-TERM-003
  - REQ-OFFICE-SESSION-TERM-004
system_design:
  - ../../specs/office/system-design/task-session-termination-01.md
legacy_specs: []
---

# Implementation Plan: Office Task Session Termination

## Overview

An office session row is keyed `(task_id, agent_profile_id)`. Three paths end
one on the loss of a single capacity, without asking whether the agent still
holds another on the same task. An agent that is both the task's runner and a
seated reviewer therefore loses the conversation it is still coding through
when it is displaced from, removed from, or reassigned away from one of those
two standings.

The change is a precondition on three call sites: resolve whether the agent
still holds a capacity on the task, and terminate only when it holds none.
Task 01 delivers the determination, wires all three call sites, and lands the
two distinguishable log records the fail-closed path needs to be diagnosable.
Task 02 adds the suppression counter and the operator-facing documentation.
That order is forced: a fail-closed suppression that logs nothing is
indistinguishable from a path that never ran, so the records ship with the
behavior rather than after it.

## Confirmed root cause

- `officeSessionTerminator.TerminateOfficeSession`
  (`internal/orchestrator/office_session_terminator.go:32`) resolves the
  session by `(taskID, agentInstanceID)` alone and flips the row to
  `COMPLETED`. It has no notion of role and cannot acquire one without
  importing office.
- Three call sites in `internal/office/dashboard/service_tasks.go` reach it
  unconditionally after their own mutation commits: line 435
  (`sessionTermReasonRoleRemoved`), line 484 via `terminateDisplacedSession`
  (`sessionTermReasonSeatClaimed`), and line 913 in
  `runReactivityForAssigneeChange` (`sessionTermReasonReassigned`).
- `AC-OFFICE-REVIEW-SEATS-002.3` prefers a reviewer that is "neither the
  task's runner nor already seated in another participant role" and falls
  back when none exists, so an agent holding two capacities is a shipped,
  fallback-default outcome rather than a misconfiguration.
- With `features.officeSessionIdentity` disabled - the state every profile
  ships - a task that has a runner routes every office run through the
  runner's session, so the role-removal and seat-claim paths either no-op
  against a non-runner or destroy the task's only live session.
- `AC-OFFICE-SEAT-PROVENANCE-002.12` already scopes the claim path's
  termination to an agent holding a live session "in that role". The
  implementation is broader than its own frozen criterion because the session
  model cannot express the qualifier.

## Scope

### In scope

- One retained-capacity determination in the office dashboard service,
  reachable from all three guarded call sites and implemented once.
- Runner capacity resolved through the shared runner projection, not through a
  hand-written runner-seat read.
- Seat capacity resolved through the task's effective participant slate at its
  current step, covering template-projected seats and every role.
- Fail-closed behavior when either read fails: suppress, record, report
  success.
- Two distinguishable records - retained capacity, and read failure - plus a
  suppression counter labelled only by bounded dimensions.
- Regression coverage for the step-scoping bound, the reassignment decoupling,
  and the shipped-configuration reachability with the flag off.

### Out of scope

- Role-scoped session identity. `AC-OFFICE-SESSION-IDENTITY-004.5` forbids the
  discriminator column it needs, and the guard delivers the same observable
  outcome with no change to session identity.
- The cascade path (`TerminateAllForAgent`). It stays unguarded per
  `AC-OFFICE-SESSION-TERM-003.1`.
- Repairing sessions already ended by the current behavior. No migration, no
  backfill.
- Any change to participant casting, seat provenance, or claim behavior.
- Ending a session when an agent loses its last capacity through a path other
  than the three named ones.
- The `features.officeSessionIdentity` rollout decision.
- Any user-visible surface, copy, or i18n work.

## Technical approach

### The determination

A single unexported method on `*DashboardService` in
`internal/office/dashboard/service_tasks.go`, taking a task identifier and an
agent profile identifier and answering whether the agent retains a capacity,
plus which one it found. It returns a read error separately from the boolean so
the caller can distinguish `AC-OFFICE-SESSION-TERM-004.1` from `-004.2`.

The runner half reads the task's execution fields through the existing office
repository accessor, whose assignee value is produced by
`sqlite.RunnerProjection` (`internal/office/repository/sqlite/base.go:34`).
`AC-OFFICE-SESSION-TERM-002.2` requires this rather than a direct seat read,
and the projection's third tier is why: it deliberately reads runner rows
across every step ordered by `created_at DESC`, so a hand-written runner-seat
query would disagree with the rest of the system exactly on tasks that have
moved between steps.

The seat half reads `Repository.ListAllTaskParticipants`
(`internal/office/repository/sqlite/participants.go:517`), which already
resolves the step, returns an empty slate when the task has no current step,
merges template-level rows under per-task precedence, and spans every role.
`AC-OFFICE-SESSION-TERM-002.3`, `-002.4` and `-002.5` fall out of reusing it
rather than being separately implemented.

Both reads run on the read pool, take no lock, and join no transaction.
`AC-OFFICE-SESSION-TERM-001.10` explains why no coordination is needed: each
guarded path reads strictly after its own commit, so whichever commits second
observes both removals and terminates.

### The call sites

Three insertions in one file, each immediately before an existing
`TerminateOfficeSession` call and each leaving every other effect of its path
untouched:

- `addOrRemoveParticipant` (line ~429), after `RemoveTaskParticipant` commits.
- `terminateDisplacedSession` (line ~478), inside the already-detached context,
  beside but not entangled with `cancelDisplacedRun`.
- `runReactivityForAssigneeChange` (line ~911), after the reactivity pipeline's
  hard cancel has already run. `AC-OFFICE-SESSION-TERM-003.2` requires that
  cancel to stay unguarded, so the guard goes below `hardCancelTaskAsync`, not
  above it.

The empty-input case (`AC-OFFICE-SESSION-TERM-001.8`) short-circuits before the
determination. The terminator's own empty-input no-op stays as it is.

### Observability

An expvar map in the dashboard package following
`internal/orchestrator/office_stall_metrics.go`'s label idiom
(`"k1=v1;k2=v2"`), labelled by the guarded reason and the retained capacity -
three reasons and two capacities, both bounded. Suppression and read failure
log at the existing call-site loggers with the task, agent profile and reason,
distinguishably from each other.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-OFFICE-SESSION-TERM-001.1`, `-001.4` | `internal/office/dashboard/session_termination_test.go` - agent retaining no capacity still terminates with its existing reason |
| `AC-OFFICE-SESSION-TERM-001.2`, `-002.12` | Same file - determination observes post-commit state; a capacity granted before the read suppresses |
| `AC-OFFICE-SESSION-TERM-001.3`, `-001.6` | Same file - suppressed path writes no session row and returns success |
| `AC-OFFICE-SESSION-TERM-001.7`, `-002.8` | Same file - a failing slate read suppresses rather than terminates |
| `AC-OFFICE-SESSION-TERM-001.8`, `-001.9`, `-001.11` | Same file - empty inputs, no live session, repeated termination |
| `AC-OFFICE-SESSION-TERM-002.1` … `-002.7`, `-002.9` … `-002.11` | `internal/office/dashboard/session_capacity_test.go` - runner-only, seat-only, both, neither; template-projected seat; decision-free role; multiple seats; slate ordering; unresolvable profile |
| `AC-OFFICE-SESSION-TERM-002.3` | Same file - task moved off a step that still carries the agent's seat must not suppress |
| `AC-OFFICE-SESSION-TERM-003.1` | `internal/orchestrator/office_session_terminator_test.go` - cascade stays unguarded |
| `AC-OFFICE-SESSION-TERM-003.2` | `internal/office/dashboard/session_termination_test.go` - reassignment cancels the execution *and* leaves the session row untouched |
| `AC-OFFICE-SESSION-TERM-003.5` | Same file - reachability regression with `features.officeSessionIdentity` off |
| `AC-OFFICE-SESSION-TERM-004.1`, `-004.2`, `-004.4` | Same file - the two records are distinguishable; proceeding terminations keep their reason values |
| `AC-OFFICE-SESSION-TERM-004.3` | `internal/office/dashboard/session_termination_metrics_test.go` - counter increments, labels bounded |

## E2E tests

None. `AC-OFFICE-SESSION-TERM-003.4` limits the observable change to a session
row that is not written; the existing live participant indicators render from
session state that already exists, and the requirement's "Out of scope"
excludes any user-visible surface.

## Work orders

- [completed] [Task 01: Guard Session Termination On Retained Capacity](task-01-guard-retained-capacity.md)
- [completed] [Task 02: Record Suppressed Session Terminations](task-02-record-suppressed-terminations.md)

## Dependency order

```text
Task 01 -> Task 02
```

Task 02 counts an outcome Task 01 creates and documents behavior Task 01
verifies. The package is sequential.

## Verification results

- `internal/office/dashboard` and `internal/orchestrator` pass with `-race`;
  no pre-existing test required a change.
- The guard's tests were confirmed non-vacuous by short-circuiting
  `retainsTaskCapacity` to "no capacity": 10 new tests fail, and the three
  tests covering terminations that must still proceed pass in both states.
- Full-repo `golangci-lint run ./...` reports 0 issues; `go build ./...`
  passes; `gofmt` reports no files.
- Specification lint passes.

## Risks

- The determination is a suppression, so a mistake in the seat scope leaks
  sessions silently rather than failing. `AC-OFFICE-SESSION-TERM-002.3`'s
  step bound is the specific defense, and its test must move the task or it
  passes without exercising the bound.
- Existing tests in `session_termination_test.go` assert that removal
  terminates. Any that incidentally leave the removed agent as the task's
  runner will start suppressing and must be corrected by fixing their setup,
  not by weakening the assertion.
- The reassignment path's guard sits near `hardCancelTaskAsync`. An
  implementation that suppresses both together satisfies the session assertion
  and violates `AC-OFFICE-SESSION-TERM-003.2`; both halves must be asserted.
- Two extra read-pool queries per guarded termination. All three paths are
  operator-initiated and low frequency, so the cost is accepted rather than
  cached.
- `RunnerProjection` is inlined SQL rather than a callable predicate. Reusing
  it means reading the task through an existing repository accessor; adding a
  bespoke query would reintroduce exactly the disagreement `-002.2` forbids.

## Package handoff

Implementation follows the sequential TDD work orders. Update each work order's
status and the plan status after its verification commands pass.
