---
id: "01-durable-admission"
title: "Durable ordinary queue admission"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUE-ADMISSION-001
acceptance_criteria:
  - AC-TASKS-QUEUE-ADMISSION-001.1
  - AC-TASKS-QUEUE-ADMISSION-001.2
  - AC-TASKS-QUEUE-ADMISSION-001.3
  - AC-TASKS-QUEUE-ADMISSION-001.7
system_design:
  - ../../specs/tasks/system-design/queue-admission.md
---

# Task 01: Durable ordinary queue admission

## Summary

Add replay-safe ordinary admission using the existing optional client queue ID.
Keep receipt persistence atomic with insertion, merge, and attachment claims.

## In scope

- Add typed ordinary admission input and receipt storage under the current task/session locks.
- Validate all client IDs and request fingerprints. Preserve unidentified and plan-comment callers.
- Return replay success before capacity, mutable reference checks, or repeated upload claims, after authorization and identity checks.
- Preserve Auto-merge ON/OFF, full-capacity fold, staged claim exclusion, order, and Auto-run behavior.
- Add additive SQLite/PostgreSQL schema and session/task cleanup. Update scoped guidance if a new queue persistence convention needs documentation.

## Out of scope

Client retries, copy, composer UI, and global WebSocket buffering.

## Acceptance

1. Exact concurrent replay yields one content admission and one attachment claim; changed payload or stale identity never mutates work.
2. Replay survives merge, reservation, dispatch, remove, clear, and restart. Session deletion cleans receipts and reset rejects old incarnation requests.
3. Receipt and queue writes roll back together. Existing ordinary merge, capacity, and comment-consumption tests still pass.

## Verification

Run from the repository root. Add named tests using TDD before production changes.

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator/messagequeue -run 'TestQueueAdmissionConcurrentReplay|TestQueueAdmissionLifecycle|TestQueueAdmissionAttachmentReplay' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add parity cases to the existing PostgreSQL test harness. Use its configured test database and record any environment skips explicitly.
Do not call skipped PostgreSQL tests passing parity evidence.

## Files likely touched

- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_admission_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_admission.go` (new)
- `apps/backend/internal/orchestrator/messagequeue/repository_admission_test.go` (new)
- `apps/backend/internal/orchestrator/messagequeue/repository_postgres_durability_test.go`
- Queue repository lifecycle deletion helpers located from the current session/task purge call sites.

## Dependencies

None. Server support must exist before Task 02 enables ordinary retries.

## Risks

Read-then-insert without database uniqueness is unsafe. Process locks alone do not provide restart or database concurrency guarantees.
Storing inline attachment bytes in receipt snapshots duplicates large payloads.
A queue-row foreign key with cascading deletion defeats receipt retention.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/queue-admission.md)
- [Design](../../specs/tasks/system-design/queue-admission.md), Server admission and Persistence
- [Receipt decision](../../decisions/2026-09-14-durable-queue-admission-receipts.md)
- `repository_plan_comment.go`, `repository_plan_comment_test.go`, and attachment admission tests as transaction patterns
- `apps/backend/AGENTS.md`

## Results

Implemented durable ordinary admission across the SQLite and PostgreSQL-backed repositories, with memory parity for tests. Admission receipts bind the request fingerprint to task, session, and incarnation identity; exact replays return the stored result, conflicts fail without mutation, and receipt writes remain atomic with queue insertion, Auto-merge, and attachment claims. Task and session cleanup remove receipts while clear and queue-row removal preserve replay protection.

Verification passed:

- `go test -tags fts5 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers -count=1`
- `go test -tags fts5 -race ./internal/orchestrator/messagequeue -run 'TestQueueAdmissionConcurrentReplay|TestQueueAdmissionLifecycle|TestQueueAdmissionAttachmentReplay' -count=1`
- `make -C apps/backend build`
- `make -C apps/backend lint`
- `go run ./cmd/sqlguard ./internal`
- `go test -race ./internal/persistence/storeconformance`
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`

The PostgreSQL receipt durability test was added and run with `-v`; it skipped because `KANDEV_TEST_POSTGRES_DSN` was not set.

Review remediation passed: full-capacity identified admissions with staged attachment IDs return `queue_full` before any claim or Auto-merge mutation, and memory admission follows the same rule. Full-capacity folds now assign one acceptance timestamp before insertion or folding, with SQLite and memory regressions proving the stored timestamp advances and exact replay does not advance it again.
