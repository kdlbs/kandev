---
id: "02-atomic-reconstruction"
title: "Reconstruct missing delivery associations"
status: done
wave: 2
depends_on:
  - "01-retained-evidence"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.6
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.7
system_design:
  - ../../specs/platform/system-design/durable-agent-record-recovery.md
---

# Task 02: Reconstruct missing delivery associations

## Summary

Use verified journal evidence to restore the missing canonical association and
its recovery projection atomically. Connect this operation to state-only Retry.
Preserve the distinction between restored records and permission to continue.

## In scope

- Add the conditional repository operation and typed provenance. Keep submission,
  recovery metadata, and exact block binding in one transaction.
- Lock before absent-row checks, including PostgreSQL multi-connection races.
  Preserve all independent recovery causes and unrelated session metadata.
- Integrate reconstruction before `RetrySessionDelivery` returns
  `missing_canonical_submission`. Keep the existing negative test for genuinely
  missing, ambiguous, or inaccessible evidence.
- Support incomplete historical process identity explicitly. Import only proven
  associations. Do not impersonate the original execution with a resumed idle one.
- Project an unbound block on session load and prompt rejection so browser state
  survives reload. Mark the saved rejected instruction as blocked through the
  existing message delivery contract. Never select its new turn as the old turn.
- After import, use the existing reconciler and explicit continuation operation.
  Preserve queue claims, native identity, and original uncertainty.
- Add reciprocal dependency-package links after PR #4380 integration. Keep prior
  work-order status and verification results unchanged.

## Out of scope

New frontend layout, automatic resend, SQL-to-journal reconciliation without
evidence, synthetic completion, queue reordering, and production data edits.

## Acceptance

1. The incident-shaped real SQL/journal regression restores exactly one
   association and matching block binding. Retry emits zero provider prompts.
2. Concurrent requests and injected crashes leave either the original state or
   a complete reconstruction. Stale owners and conflicts change nothing.
3. Only the existing explicit continuation action can dispatch a new instruction.
   Missing process proof, a live owner, or another block prevents continuation.

## Verification

Run from the repository root:

```bash
(cd apps/backend && env -u KANDEV_TEST_POSTGRES_DSN go test -trimpath -race ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/handlers ./internal/agent/runtime/lifecycle -run 'Test(ReconstructAgentDeliverySubmission|RetrySessionDeliveryReconstructsMissingSubmission|MissingDeliveryRecordNoticeSurvivesReload|ReconstructedDeliveryContinuesOnlyExplicitly)' -count=1)
(cd apps/backend && env -u KANDEV_TEST_POSTGRES_DSN go test -trimpath -race ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/handlers ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestPostgresReconstructAgentDeliverySubmission' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && env -u KANDEV_TEST_POSTGRES_DSN go test -trimpath -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

Use the repository's isolated PostgreSQL fixture. Do not use the live database
or print its connection string. Record unavailable infrastructure as a blocker.
Run SQLite checks with that environment variable unset, then run PostgreSQL
checks with the fixture configured. A skipped PostgreSQL test is not evidence.

Required regression cases:

- One dispatching non-initial journal record, fully acknowledged output, no SQL
  submission or recovery metadata, an unbound delivery block, and saved history.
- The same case with a resumed idle execution in the current generation.
- Completed history plus one unresolved record, and then a second conflicting
  unresolved record. Only the first complete candidate set permits import.
- Existing identical and conflicting canonical rows, multiple backend owners,
  changed generation, archive/deletion, Stop, and concurrent successor admission.
- Failure before and after each transactional write. Reopen SQL and repeat Retry.
- Known terminal evidence, already projected terminal evidence, and missing
  historical process proof. No fabricated terminal event or double settlement.
- One explicit continuation after verified termination, then duplicate requests
  and restart. Count provider prompts and preserve native conversation identity.

## Files likely touched

- `apps/backend/internal/task/models/agent_delivery.go`
- `apps/backend/internal/task/models/session_continuity.go`
- Existing delivery interfaces in `apps/backend/internal/task/repository/`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_reconstruction.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_reconstruction_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_reconstruction_postgres_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state.go`
- `apps/backend/internal/orchestrator/session_delivery_recovery.go` (PR #4380)
- `apps/backend/internal/orchestrator/session_delivery_reconstruction.go` (new)
- `apps/backend/internal/orchestrator/session_delivery_reconstruction_test.go` (new)
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`
- `apps/backend/internal/orchestrator/agent_delivery_recovery_state.go`
- Existing task-session DTO and WebSocket projections
- `apps/backend/internal/orchestrator/handlers/interrupted_recovery.go` (PR #4380)
- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_recovery_without_execution.go` (PR #4380)
- Dependency plan and Task 02 under `docs/plans/agentctl-journal-shutdown-recovery/`

No schema change is planned. If a column becomes necessary, add its migration,
fresh/reopen/upgrade tests, DTO propagation, and store descriptor coverage.

## Dependencies

Task 01 and the integrated PR #4380 continuation/reconciliation path.

## Risks

`UpsertAgentDeliveryRecovery` currently requires an existing canonical row and
matching execution. Do not weaken that event-write guard globally. The new
reconstruction transaction needs its own verified owner snapshot.

The journal cannot participate in the SQL transaction. Fence its runtime lease
and immutable record identity before commit, and recheck before later actions.

## Parallelism

`sequential`

## Inputs

- [Design: atomic reconstruction](../../specs/platform/system-design/durable-agent-record-recovery.md#atomic-reconstruction).
- [Design: reconciliation and continuation](../../specs/platform/system-design/durable-agent-record-recovery.md#reconciliation-and-continuation).
- Existing delivery recovery and settlement repository tests.
- PR #4380's missing-canonical negative test and explicit continuation tests.

## Results

Implemented `ReconstructAgentDeliverySubmission` as a conditional repository
transaction. It locks session-turn writes and the PostgreSQL reconstruction key
before absent-row checks; revalidates the task, session, native generation,
workspace, saved user message, recovery block, cursor, and existing submissions;
then inserts the retained immutable prompt as `interrupted_unknown`, stores
bounded provenance, and binds only the matching delivery block. Retries return
the original revision and timestamp. A global submission-ID collision maps to
a typed reconstruction conflict. Independent recovery causes and unrelated
session metadata remain intact.

`RetrySessionDelivery` now inspects and imports a uniquely verified retained
record before returning `missing_canonical_submission`. A resumed idle execution
is not recorded as the owner of the old prompt. When process identity is
incomplete, the persistent result remains blocked and has no allowed action or
provider prompt. The journal descriptor includes the sole unresolved candidate's
stream snapshot in the same read transaction.

Verification passed:

- Focused journal and orchestration regressions passed with the missing-record
  and legacy missing-canonical cases.
- The exact four-package focused and full race runs passed for SQLite, the
  orchestrator, handlers, and runtime lifecycle.
- The concurrent reconstruction race passed on an isolated PostgreSQL
  multi-connection fixture; one request inserted and the other returned the
  unchanged committed identity.
- SQLite and PostgreSQL store conformance passed under race. The SQL guard and
  `git diff --check` passed.
- The SQLite trigger regression verified that a failure during block binding
  rolls back the inserted row and recovery metadata. Duplicate retry preserved
  the original recovery revision and timestamp.

The broad PostgreSQL repository race suite was not run. The targeted real
multi-connection regression and PostgreSQL store-conformance suite both ran
against a separate disposable PostgreSQL 16 container.

### Review remediation

Verified historical execution identity can now enrich an incomplete reconstructed
record once, guarded by the observed recovery revision. Repeated Retry is
idempotent. Successor prompts, recreated generations, idle executions, and
changed proof are rejected. Reconstruction never sends a provider prompt.
The explicit-continuation regression verifies one new instruction in the same
native conversation and preserves the old immutable payload and unknown outcome.
The continuation transaction compares reloaded recovery metadata by value,
including reconstruction provenance, while retaining all ownership fences.

The final command passed all eight packages:

```sh
cd apps/backend
GOMAXPROCS=2 go test -trimpath -p 2 -race -timeout=30m ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/handlers ./internal/agent/runtime/lifecycle ./internal/agentctl/journal ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/persistence/storeconformance -count=1
```

The focused reconstructed-continuation regression failed before the value
comparison fix and passed afterward. PostgreSQL reconstruction/enrichment races
and interrupted-continuation checks passed with the real disposable PostgreSQL
18 database. PostgreSQL store conformance passed. The SQL guard passed.
Integrated Go lint reported zero issues. The final browser and frontend results
are recorded in Task 03; real-provider compatibility remains unverified.
