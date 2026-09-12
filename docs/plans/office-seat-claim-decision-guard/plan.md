---
created: 2026-09-08
status: implemented
requirements:
  - REQ-OFFICE-SEAT-GUARD-001
  - REQ-OFFICE-SEAT-GUARD-002
system_design:
  - ../../specs/office/system-design/seat-claim-decision-guard-01.md
legacy_specs: []
---

# Implementation Plan: Office Seat Claim Decision Guard

## Overview

`claimAutoSeat`'s `NOT EXISTS` condition over `workflow_step_decisions` is
covered by a test whose stated premise stopped being true when the shared seat
exclusion landed. The condition is still load-bearing, but on a path the test
does not reach, and the production comment beside it says the opposite.

No production behavior changes. Task 01 adds a test-only interleaving hook and
the guard test that actually distinguishes the condition being present from its
being absent. Task 02 corrects the two pieces of prose that assert a false
premise. That order is deliberate: the corrected prose cites the new test as
what covers the window, so the test must exist before the comment can point at
it.

## Confirmed root cause

- `claimAutoSeat` (`internal/office/repository/sqlite/participants.go:437`)
  carries `NOT EXISTS (SELECT 1 FROM workflow_step_decisions WHERE
  participant_id = ?)` on the reassigning `UPDATE`. It adds something
  `findClaimableAutoSeat` did not already provide only when a decision commits
  between the two statements.
- `recordStepDecisionTx`
  (`internal/workflow/repository/phase2_sqlite.go:1097-1102`) acquires
  `ParticipantRoleSeatLockKey` - the same exclusion registration holds for its
  whole transaction - but only `if d.Role != ""`. A roleless decision skips it.
- `validateDecisionParticipantSeatTx`
  (`internal/workflow/repository/phase2_sqlite.go`) returns early when
  `d.Role == ""`, so the roleless path takes neither the seat exclusion nor the
  seat validation. For that path the `UPDATE`'s condition is the whole defense.
- No shipped caller produces a roleless decision - both office entry points
  resolve a role or refuse - so this is latent, not a live defect.
- `TestPostgresAddTaskParticipant_ClaimDoesNotOverwriteAConcurrentDecision`
  (`internal/office/repository/sqlite/participant_claim_postgres_test.go:78`)
  documents itself as proving the condition load-bearing on the premise that
  the two writers "lock on two different advisory-lock namespaces that never
  contend". That premise is false since commit `a6c6af2d5`.
- The production comment at `participants.go:169-173` states that
  `recordStepDecisionTx` "now shares this transaction's
  ParticipantRoleSeatLockKey exclusion, so it cannot actually interleave here",
  which holds for a role-carrying decision and not for a roleless one.

## Scope

### In scope

- A test-only interleaving hook in the office sqlite package, called between
  the statement that selects the claimable seat and the statement that
  reassigns it, nil in every build that does not set it.
- A deterministic guard test that makes a roleless decision exist against the
  chosen seat inside that window and asserts the seat is not reassigned and a
  fresh seat is written for the registering agent instead.
- A Postgres-gated variant that commits the roleless decision from a real
  second connection, covering the literal cross-transaction window.
- Correcting the existing concurrency test's docstring to claim only what
  serialization can show.
- Correcting the production comment that describes the guard as unreachable.

### Out of scope

- Removing the `NOT EXISTS` condition. `AC-OFFICE-SEAT-GUARD-001.3` requires
  it to stay, and the roleless path is why.
- Making the roleless decision path acquire the seat exclusion. It has no role
  to key on and would have to derive one from the seat it references, turning a
  write into a read-then-lock for a path no shipped caller uses.
- Rejecting roleless decisions. Whether the store should require a role is a
  separate contract question about the decision model.
- Adding a `superseded_at IS NULL` filter to either defense.
  `AC-OFFICE-SEAT-GUARD-001.8` requires both to keep treating any decision as
  blocking.
- Deleting or weakening the existing concurrency test. Its iterations and
  multi-connection pool stay; only its docstring changes.
- The sibling follow-up's claim-target-removed test. If it wants the same yield
  point, it is the same hook at the same site, but neither card depends on the
  other.
- Any observability signal for the declined-claim outcome, which
  `AC-OFFICE-SEAT-GUARD-001.5` deliberately makes indistinguishable from "no
  claimable seat existed".

## Technical approach

### The hook

An unexported package-level function value in
`internal/office/repository/sqlite`, called from `attemptClaim`
(`participants.go:206`) between `findClaimableAutoSeat` and `claimAutoSeat`.
Unexported, so only the package's own tests can set it, and no build tag is
needed to keep it unreachable. Unset, the call site is a nil check on a value
nothing ever writes, so the production path is the behavior that ships today.
The hook carries no policy: it is a yield point.

The hook receives the in-flight transaction and the selected seat identifier.
That signature is what makes the deterministic test possible on the embedded
engine, and it is a deviation from the system design's "second connection"
wording - see Open questions.

### The deterministic test

Sets the hook to insert a roleless decision row against the selected seat
using the registration's own transaction, then lets `claimAutoSeat` run. The
`UPDATE` affects zero rows, `attemptClaim` returns nil, and the registration
falls through to `insertManualParticipant`. Asserts the auto seat still names
its original agent with its provenance, decision-required flag, position and
creation time unchanged; that the decision row is unchanged; that a fresh
manual seat exists for the registering agent; and that the reported outcome is
`ParticipantWriteOutcomeInserted` rather than `...Claimed`, so none of the
effects a claim earns is triggered.

This runs on the embedded engine, where the office sqlite suite sets
`SetMaxOpenConns(1)`. Removing the `NOT EXISTS` condition must make it fail;
that check is performed once by hand when the test is written and recorded in
the work order's Results.

### The Postgres-gated variant

`openIsolatedPostgresMultiConnForClaimRace` already provides a four-connection
pool. The hook commits a roleless decision from a second connection while the
registration's transaction is open. There is no deadlock: the roleless path
skips `ParticipantRoleSeatLockKey`, takes only `decisionLockNamespace`, and
writes no row the registration holds. Skips unless `KANDEV_TEST_POSTGRES_DSN`
is set, matching the neighbouring test.

### The prose corrections

The concurrency test's docstring is re-written to state what its assertions
support: that a role-carrying decision and a registration serialize on the
shared seat exclusion, complete without deadlock in either acquisition order,
and leave no reattributed decision. The production comment at
`participants.go:169-173` is corrected to say that the exclusion closes the
window for a role-carrying decision and that the condition remains the defense
for a roleless one.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-OFFICE-SEAT-GUARD-001.1`, `-001.2` | `internal/office/repository/sqlite/participant_claim_decision_guard_test.go` - roleless decision in the window blocks the reassignment |
| `AC-OFFICE-SEAT-GUARD-001.4` | Same file - seat's agent profile, provenance, decision-required flag, position and creation time unchanged, decision unchanged |
| `AC-OFFICE-SEAT-GUARD-001.5`, `-001.6` | Same file - outcome is `Inserted`, no displaced agent reported, registration succeeds |
| `AC-OFFICE-SEAT-GUARD-001.8` | Same file - a superseded decision blocks equally; the selection and the update agree |
| `AC-OFFICE-SEAT-GUARD-001.9` | Same file - repeating the registration writes nothing further under the already-seated rule |
| `AC-OFFICE-SEAT-GUARD-001.3`, `-002.1`, `-002.2` | Same file - the deterministic test fails with the `NOT EXISTS` condition removed, verified by hand |
| `AC-OFFICE-SEAT-GUARD-002.2` (cross-transaction reading) | Same file, Postgres-gated variant on a real second connection |
| `AC-OFFICE-SEAT-GUARD-002.3` | `internal/office/repository/sqlite/participants_test.go` - existing claim tests pass unchanged with the hook unset |
| `AC-OFFICE-SEAT-GUARD-002.4`, `-002.5` | `internal/office/repository/sqlite/participant_claim_postgres_test.go` - docstring asserts serialization and no-deadlock only |
| `AC-OFFICE-SEAT-GUARD-001.7` | No change to `docs/specs/office/requirements/review-participant-seats.md` or the seat-provenance contract |

## E2E tests

None. `REQ-OFFICE-SEAT-GUARD-001` records existing behavior and ships no
user-visible change.

## Work orders

- [completed] [Task 01: Cover The Claim Decision Guard](task-01-cover-claim-decision-guard.md)
- [completed] [Task 02: Correct The Stale Exclusion Premise](task-02-correct-exclusion-premise.md)

## Dependency order

```text
Task 01 -> Task 02
```

Task 02's corrected prose names the test Task 01 adds as what covers the
roleless window. The package is sequential.

## Verification results

- Four engine-agnostic guard tests pass; `internal/office/repository/sqlite`
  and `internal/workflow/repository` pass with `-race`.
- The by-hand acceptance check was performed: deleting `claimAutoSeat`'s
  `NOT EXISTS` condition makes the guard tests fail with the seat reattributed
  to the registering agent, and restoring it makes them pass.
- The Postgres-gated variant compiles, vets, and skips cleanly, but has never
  run against a real server: this runner has no PostgreSQL and the card forbids
  Docker. CI is its first execution.
- `golangci-lint` reports 0 issues across `internal/office/...` and
  `internal/workflow/...`; specification lint passes; `git diff --check` is
  clean.
- The open question below was resolved as proposed.

## Risks

- A test-only hook in a production file is a concession. It is acceptable only
  while it stays unexported, unset in production, and carries no behavior; a
  reviewer should check all three rather than the first.
- Racing real goroutines on the roleless path would reach the window but is not
  forced to, so such a test would pass most of the time by never entering it -
  the same defect being fixed, reintroduced. The hook exists to avoid that.
- The deterministic test's premise is that the `UPDATE` sees the decision. If a
  future change moves the decision read out of `claimAutoSeat`'s statement, the
  test would keep passing while covering nothing. The by-hand removal check is
  what catches this, and it must be repeated if the statement is restructured.
- The Postgres variant depends on the roleless path continuing to skip
  `ParticipantRoleSeatLockKey`. If that changes, the variant deadlocks rather
  than failing clearly; the deterministic test is the primary coverage for that
  reason.
- Correcting the concurrency test's docstring without weakening its assertions
  is the intent. A reviewer should confirm no `iterations` or pool size was
  reduced alongside the prose.
- The sibling seat-provenance hardening card proposes, as its ninth gap, "a
  test-only failpoint between `findClaimableAutoSeat` and `claimAutoSeat`" for
  `TestPostgresAddTaskParticipant_ConvergesWithConcurrentRemoveOfClaimTarget`.
  That is the same site as this hook, for a different test and a different
  acceptance criterion. The two do not overlap in coverage, but they collide in
  the file: whichever lands first owns the hook, and the second reuses it
  rather than adding a parallel one. Check for an existing hook at that site
  before writing Task 01.

## Open questions

- The system design's "Making the window reachable" section specifies that the
  hook's test "commits a roleless decision against the chosen seat from a
  second connection". The office sqlite suite opens its database with
  `SetMaxOpenConns(1)`, so a second connection issued while the registration's
  transaction holds the only pooled connection blocks indefinitely. This plan
  therefore has the hook receive the in-flight transaction, writes the decision
  through it for the deterministic embedded-engine test, and keeps the literal
  cross-connection reading as the Postgres-gated variant. The requirement's
  `AC-OFFICE-SEAT-GUARD-002.2` and `-002.3` are satisfied either way, since
  they constrain determinism and production inertness rather than the
  connection count. Confirm this deviation, or amend the system design's
  wording, before Task 01 starts.

  **Resolved as proposed.** The hook takes the transaction, the deterministic
  test writes through it and runs on every engine, and the literal
  cross-connection reading is kept as the Postgres-gated variant. The setter
  additionally lives in `export_test.go` rather than in an in-package test, so
  a production build has no setter at all.

## Package handoff

Implementation follows the sequential TDD work orders. Update each work order's
status and the plan status after its verification commands pass.
