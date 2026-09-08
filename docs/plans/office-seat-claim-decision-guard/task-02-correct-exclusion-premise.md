---
id: "02-correct-exclusion-premise"
title: "Correct the stale exclusion premise"
status: pending
wave: 2
depends_on:
  - "01-cover-claim-decision-guard"
plan: "plan.md"
requirements:
  - REQ-OFFICE-SEAT-GUARD-002
acceptance_criteria:
  - AC-OFFICE-SEAT-GUARD-002.4
  - AC-OFFICE-SEAT-GUARD-002.5
  - AC-OFFICE-SEAT-GUARD-001.7
system_design:
  - ../../specs/office/system-design/seat-claim-decision-guard-01.md
---

# Task 02: Correct The Stale Exclusion Premise

## Summary

Two pieces of prose assert that role-carrying decision recording and
registration lock on non-contending exclusions, or that the claim's condition
is therefore unreachable. Both stopped being accurate when the shared seat
exclusion landed. Re-document them to state what is true, without changing any
assertion, iteration count, or pool size.

## In scope

- Rewriting `TestPostgresAddTaskParticipant_ClaimDoesNotOverwriteAConcurrentDecision`'s
  docstring to claim only what its assertions support: that the two writers
  serialize on the shared seat exclusion, complete without deadlock in either
  acquisition order, and leave no reattributed decision.
- Correcting the comment at `participants.go:169-173` so it states that the
  exclusion closes the window for a role-carrying decision while the `UPDATE`'s
  condition remains the defense for a roleless one, and names the test from
  Task 01 as the coverage.
- A scan of the office and workflow suites for any other test documentation
  repeating the non-contending-namespaces premise.

## Out of scope

- Deleting, skipping, or weakening the existing concurrency test, reducing its
  iterations, or shrinking its connection pool.
- Any change to production behavior, SQL, or lock acquisition.
- Any acceptance criterion added to or amended in the
  participant-seat-provenance contract.
- Renaming the existing test.

## Acceptance

- No test documentation in the office or workflow suites asserts that
  role-carrying decision recording and registration lock on non-contending
  namespaces.
- The existing concurrency test's assertions, iteration count, and pool size
  are byte-identical to before this work order.
- The corrected production comment distinguishes the role-carrying case from
  the roleless one and states the invariant rather than the history of the bug.

## Verification

```bash
# From the repository root:
cd apps/backend && go test ./internal/office/repository/sqlite/... -race -count=1
make -C apps/backend lint
git diff --cached --name-only | grep '\.go$' | xargs -r gofmt -l
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/participant_claim_postgres_test.go`
- `apps/backend/internal/office/repository/sqlite/participants.go`

## Dependencies

- Task 01 adds the test that the corrected comment names as covering the
  roleless window.

## Risks

- A comment that argues the code is correct instead of stating the invariant
  reintroduces the failure this work order is fixing, in a new form. State
  which defense covers which window and stop.
- Rewriting the docstring is easy to combine with "tidying" the assertions.
  Confirm with a diff that only comment lines changed in the test file.
- Other suites may carry the same premise in passing. The scan is part of the
  work order because `AC-OFFICE-SEAT-GUARD-002.5` is written over the whole
  suite, not over one file.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/requirements/seat-claim-decision-guard.md`,
  REQ-OFFICE-SEAT-GUARD-002.
- `docs/specs/office/system-design/seat-claim-decision-guard-01.md`, section
  "Verification obligations", items 2 and 3.
- `apps/backend/internal/office/repository/sqlite/participant_claim_postgres_test.go:78-93`.
- `apps/backend/internal/office/repository/sqlite/participants.go:166-173`.
- `CLAUDE.md`, "Code Quality", on comments stating the invariant rather than
  the argument for it.

## Results

Pending.
