---
id: "02-repository-provenance-cases"
title: "Repository cases: probe order, re-registration, seat invariants, writing handle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/participant-seat-provenance-assurance.md"
---

# Task 02: Repository cases — probe order, re-registration, seat invariants, writing handle

Implements `AC-OFFICE-SEAT-ASSURANCE-002.1`, `-002.2`, `-002.9` and `-002.3`.
All four criteria of `participant-seat-provenance.md` they pin are already
satisfied by shipped code. This task makes them falsifiable. No production
change.

## Acceptance

### Identity probe precedes claim search (`-002.1` → `AC-OFFICE-SEAT-PROVENANCE-002.4`)

- A role slate at the task's current step holds a `manual` seat for agent A and
  exactly one undecided `auto` seat for a different agent B. Agent A is
  registered again in that role.
- Assert the registration reports `ParticipantWriteOutcomeUnchanged`, **and**
  that B's seat still names B with provenance `auto`. The second assertion is
  the one that bites: an implementation that searched for a claimable seat
  before probing for the named agent's own would consume B's seat.
- Extend `participant_provenance_test.go` (488 lines, has room).

### Re-registering a displaced agent (`-002.2` → `-002.10`)

- A claim displaces agent A in favour of agent B; A is then registered again in
  the same role at the same step.
- Assert the slate holds exactly two seats, both with provenance `manual`,
  naming A and B.

### Claim leaves the rest of the seat alone (`-002.9` → `-002.2`)

- Extend the existing coverage at `participants_ops_test.go` (currently checks
  agent and provenance across a claim) to also assert the seat's **identifier**,
  **decision-required flag**, **position** and **creation time** are unchanged.
- Capture all five before the claim and compare after. Creation time is a real
  column on the seat table that the office-side projection does not carry, so
  read the column directly rather than the projected struct.

### The step is resolved on the writing handle (`-002.3` → `-004.10`)

- Build the office repository with its writer on one store and its reader on a
  **second** store, where the same task row names a *different* current step.
- Register a participant. Assert the seat landed at the step the **writer's**
  store names.
- This must be decisive in both directions. The shipped implementation reads
  through `stepIDForTaskTx` on the transaction and lands at the writer's step;
  an implementation using `stepIDForTask` on the read-only pool lands at the
  reader's step. Verify by temporarily making that one-word swap and confirming
  the case goes red — today that swap passes the whole suite. Revert the swap.
- Prefer a **divergent** step id over an absent reader row: divergence proves
  the seat landed at the writer's step, absence only proves the reader was not
  consulted.
- Two setup constraints: schema initialisation runs against the writer, so give
  the reader store its own `tasks` table and row explicitly; and every assertion
  must read through the writer handle directly, never the repository's reader
  accessor, which by construction now points at the wrong database.
- New file — this needs its own two-store fixture and does not belong in either
  existing file.

## Constraints

- No production code changes in this task. A case that cannot be made to pass is
  evidence about the spec, not licence to edit the implementation
  (`AC-OFFICE-SEAT-ASSURANCE-002.12`).
- Do not weaken or delete any existing assertion. `-002.9` **extends** the
  existing case; `-002.1` and `-002.2` are new cases beside existing ones.
- Measure file length before appending. Revive's 800-effective-line limit applies
  to test files.

## Verification

```bash
cd apps/backend && go test ./internal/office/repository/sqlite/... -run 'Participant|Seat|Claim'
cd apps/backend && go test ./internal/office/...
make -C apps/backend lint
```

Record the RED result for the `-002.3` swap experiment described above.

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/participant_provenance_test.go`
- `apps/backend/internal/office/repository/sqlite/participants_ops_test.go`
- `apps/backend/internal/office/repository/sqlite/` — new step-resolution test file
- this task file

## Inputs

- `docs/specs/office/requirements/participant-seat-provenance.md` — full AC text
  for `-002.4`, `-002.10`, `-002.2`, `-004.10`
- `docs/specs/office/system-design/participant-seat-provenance-assurance-01.md`
  ("Making each window reachable")
- `/tdd`

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, risk tags, exact RED/GREEN verification commands and
results (including the swap experiment), uncertainties, and this task status set
to `done`. Do not edit `plan.md`.
