---
id: "01-cover-claim-decision-guard"
title: "Cover the claim decision guard"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SEAT-GUARD-001
  - REQ-OFFICE-SEAT-GUARD-002
acceptance_criteria:
  - AC-OFFICE-SEAT-GUARD-001.1
  - AC-OFFICE-SEAT-GUARD-001.2
  - AC-OFFICE-SEAT-GUARD-001.3
  - AC-OFFICE-SEAT-GUARD-001.4
  - AC-OFFICE-SEAT-GUARD-001.5
  - AC-OFFICE-SEAT-GUARD-001.6
  - AC-OFFICE-SEAT-GUARD-001.8
  - AC-OFFICE-SEAT-GUARD-001.9
  - AC-OFFICE-SEAT-GUARD-002.1
  - AC-OFFICE-SEAT-GUARD-002.2
  - AC-OFFICE-SEAT-GUARD-002.3
system_design:
  - ../../specs/office/system-design/seat-claim-decision-guard-01.md
---

# Task 01: Cover The Claim Decision Guard

## Summary

Add a test-only interleaving hook between the statement that selects a
claimable seat and the statement that reassigns it, and use it to drive a
deterministic test that fails when `claimAutoSeat`'s `NOT EXISTS` condition is
removed. Production behavior is unchanged: with the hook unset the call site is
a nil check on a value nothing writes.

## In scope

- An unexported package-level function value in
  `internal/office/repository/sqlite`, invoked from `attemptClaim` between
  `findClaimableAutoSeat` and `claimAutoSeat`, receiving the in-flight
  transaction and the selected seat identifier.
- A deterministic in-package test that makes a roleless decision exist against
  the selected seat inside the window and asserts the seat is not reassigned.
- Assertions that the auto seat's agent profile, provenance, decision-required
  flag, position and creation time are unchanged, that the decision is
  unchanged, that a fresh manual seat exists for the registering agent, and
  that the reported outcome is `Inserted` with no displaced agent.
- A case proving a superseded decision blocks the reassignment identically.
- A case proving the repeat registration writes nothing further.
- A Postgres-gated variant committing the roleless decision from a second
  connection, skipped unless `KANDEV_TEST_POSTGRES_DSN` is set.
- A recorded by-hand check that removing the `NOT EXISTS` condition makes the
  deterministic test fail.

## Out of scope

- Any change to `claimAutoSeat`, `findClaimableAutoSeat`,
  `insertManualParticipant`, the seat exclusion, or the registration's
  transaction boundaries.
- Any change to `recordStepDecisionTx`, including making the roleless path
  acquire the seat exclusion.
- The prose corrections in `participants.go` and the existing Postgres
  concurrency test, which Task 02 owns.
- Adding a `superseded_at` filter to either defense.
- Any exported symbol, schema change, or migration.

## Acceptance

- With the hook unset, every existing claim test passes unchanged and the
  production path is the behavior that ships today.
- The deterministic test enters the window on every run and cannot pass by
  skipping it.
- Deleting `NOT EXISTS (SELECT 1 FROM workflow_step_decisions WHERE
  participant_id = ?)` from `claimAutoSeat` makes the deterministic test fail;
  restoring it makes the test pass.

## Verification

```bash
# From the repository root:
cd apps/backend && go test ./internal/office/repository/sqlite/... -race -count=1
cd apps/backend && go test ./internal/workflow/repository/... -race -count=1
make -C apps/backend lint
git diff --cached --name-only | grep '\.go$' | xargs -r gofmt -l
git diff --check

# Postgres-gated variant, when a server is available:
cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" \
  go test ./internal/office/repository/sqlite/... -run ClaimDecisionGuard -count=1
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/participants.go`
- `apps/backend/internal/office/repository/sqlite/participant_claim_decision_guard_test.go`

## Dependencies

None.

## Risks

- A hook that grows a return value or a branch stops being a yield point and
  becomes a policy hook in production code. Keep it a bare call with no result
  the production path reads.
- Racing goroutines instead of using the hook would pass most of the time by
  never entering the window, which is the defect being fixed. Do not fall back
  to a race even if the hook proves awkward.
- The office sqlite suite sets `SetMaxOpenConns(1)`. A test that opens a second
  connection while the registration transaction is open blocks indefinitely
  rather than failing; the embedded-engine test must write through the
  in-flight transaction.
- The Postgres variant depends on the roleless decision path continuing to skip
  `ParticipantRoleSeatLockKey`. If that stops being true it will deadlock
  rather than fail clearly.
- The by-hand removal check is the only thing proving the test is not vacuous.
  Record its result in this work order rather than asserting it was done.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/requirements/seat-claim-decision-guard.md`,
  REQ-OFFICE-SEAT-GUARD-001 and -002.
- `docs/specs/office/system-design/seat-claim-decision-guard-01.md`, sections
  "Two defenses, two windows" and "Making the window reachable", read together
  with the plan's Open questions entry on the hook signature.
- `apps/backend/internal/office/repository/sqlite/participants.go:160-243`
  (`ensureParticipant` fallthrough and `attemptClaim`) and `:437`
  (`claimAutoSeat`).
- `apps/backend/internal/workflow/repository/phase2_sqlite.go:1080-1103` for
  the roleless path's lock skip.
- `apps/backend/internal/office/repository/sqlite/participant_claim_postgres_test.go`
  for the isolated multi-connection Postgres helper.

## Results

Pending.
