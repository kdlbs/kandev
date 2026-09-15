---
id: "01-store-boundary-guard"
title: "Seat store refuses a registration naming no agent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SEAT-ASSURANCE-001
acceptance_criteria:
  - AC-OFFICE-SEAT-ASSURANCE-001.1
  - AC-OFFICE-SEAT-ASSURANCE-001.2
  - AC-OFFICE-SEAT-ASSURANCE-001.3
  - AC-OFFICE-SEAT-ASSURANCE-001.4
  - AC-OFFICE-SEAT-ASSURANCE-001.5
  - AC-OFFICE-SEAT-ASSURANCE-001.6
  - AC-OFFICE-SEAT-ASSURANCE-001.7
system_design:
  - ../../specs/office/system-design/participant-seat-provenance-assurance-01.md
---

# Task 01: Seat store refuses a registration naming no agent

Implements `REQ-OFFICE-SEAT-ASSURANCE-001` (AC-001.1 through -001.7).

## Acceptance

- An exported sentinel `ErrEmptyAgentProfileID` exists in the
  `repository/sqlite` package. It is one identity, not a formatted string, so a
  caller can distinguish this failure from the store's *unchanged* outcome
  without matching on a message (`AC-001.2`).
- `Repository.AddTaskParticipant` returns it with the zero
  `ParticipantWriteResult` as its **first statement** — before `BeginTxx`, and
  therefore before `lockParticipantRoleSeat` acquires the shared exclusion of
  `AC-OFFICE-SEAT-PROVENANCE-004.2` (`AC-001.3`). A rejected call contends with
  no other writer and leaves nothing to roll back.
- The comparison is against the empty string only. No trimming, no
  normalisation (`AC-001.5`). A whitespace identifier still reaches the store
  and stays governed by `AC-OFFICE-SEAT-PROVENANCE-005.8`: it claims no seat and
  displaces no `auto` seat.
- No seat is written, no seat is claimed, and no seat's provenance is promoted
  on the rejected path (`AC-001.1`).
- The guard consults no store state, so it reaches the same decision on both
  supported dialects and on every call (`AC-001.4`, `-001.7`).
- The existing surface check in `dashboard/participants_handlers.go`
  (`req.AgentProfileID == ""`) stays exactly where it is and is not moved,
  weakened or collapsed into this one. Two layers is the intent.
- The guard is **not** placed in the service layer beside the
  `participantFields[role]` lookup in `addOrRemoveParticipant`. Placing it there
  would reproduce the gap one layer down.
- Nothing maps the sentinel to an HTTP status. No shipped route can reach it; it
  falls through the existing generic error mapping.
- No behaviour observable through the shipped registration surface changes
  (`AC-001.6`).

## Tests

New file in `repository/sqlite` (do not append to
`participants_ops_test.go`, which is at 438 lines and owned by task 02):

- An empty identifier returns `ErrEmptyAgentProfileID` (assert with
  `errors.Is`), and the task's slate for that role is unchanged — assert against
  a seeded slate holding an undecided `auto` seat, so the case proves the seat
  was neither claimed nor promoted, not merely that no row was added.
- The same call twice yields the same failure and no state (`AC-001.4`).
- A whitespace-only identifier is **not** rejected by this guard — it proceeds
  and leaves the `auto` seat unclaimed per `AC-OFFICE-SEAT-PROVENANCE-005.8`.
  This pins the boundary so the two layers cannot drift about what "empty"
  means (`AC-001.5`).
- The rejection is distinguishable from the *unchanged* outcome the store
  reports for a task at no step and for a task that does not exist: those return
  a nil error with `ParticipantWriteOutcomeUnchanged`, this returns a non-nil
  sentinel (`AC-001.2`).

## Verification

```bash
cd apps/backend && go test ./internal/office/repository/sqlite/... -run 'Participant|Seat'
cd apps/backend && go test ./internal/office/...
make -C apps/backend lint
```

Confirm `AC-001.6` directly: `go test ./internal/office/dashboard/...` passes
unchanged, with no test edited to accommodate the guard.

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/participants.go`
- `apps/backend/internal/office/repository/sqlite/` — new guard test file
- this task file

## Inputs

- `docs/specs/office/requirements/participant-seat-provenance-assurance.md`
  (REQ-OFFICE-SEAT-ASSURANCE-001)
- `docs/specs/office/system-design/participant-seat-provenance-assurance-01.md`
  ("The store-boundary guard")
- `/tdd`

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, risk tags, exact RED/GREEN verification commands and
results, uncertainties, and this task status set to `done`. Do not edit
`plan.md`.
