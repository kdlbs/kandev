---
id: "01-auto-merge-repository"
title: "Build automatic tail merge"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-auto-merge.md"
---

# Task 01: Build automatic tail merge

## Acceptance

1. The repository contract and memory/database implementations atomically fold
   an exact source ID into only its immediate predecessor, return the surviving
   target and a merged/skipped result, and never mutate either row on skip or
   error.
2. Automatic compatibility enforces source, task, model, plan mode, metadata,
   lifecycle, attachment, reference, and context rules from the spec while
   preserving target identity and target-then-source order. Manual merge
   behavior remains unchanged.
3. Table-driven memory, SQLite, and env-gated Postgres tests cover happy paths,
   every mismatch/limit fallback, missing/drained candidates, and atomic
   rollback.

## Verification

```bash
cd apps/backend && go test -count=1 ./internal/orchestrator/messagequeue
cd apps/backend && go test -race -count=1 -run 'Test.*AutoMerge' ./internal/orchestrator/messagequeue
```

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/merge.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/send_now.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_auto_merge_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_postgres_auto_merge_test.go`

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- Spec: `What`, `Data model`, `Admission lifecycle and concurrency`, `Failure
  modes`, and compatibility/limit scenarios.
- Plan: `Backend > Automatic tail-merge contract`.
- Existing patterns: `MergeIntoAbove` transaction and memory lock,
  `buildSendNowEnvelope` attachment/reference validation, persisted metadata
  normalization, and env-gated Postgres merge tests.

## Risks

- JSON-round-tripped metadata types must compare semantically, not by Go slice
  representation.
- Incompatibility is a successful skip. Do not reuse manual merge errors for it.
- Keep new Go functions within repository lint limits by extracting pure
  compatibility and union helpers.

## Output contract

Report summary, files changed, exact commands/outcomes, blockers, risks, and
update this task plus `plan.md` status in the same conversation.

## Results

- Added `AutoMergeIntoAbove` to the repository contract and implemented it for
  memory plus database-backed repositories without changing manual merge.
- Added automatic-only compatibility, semantic metadata comparison, stable
  entity/context unions, attachment limit enforcement, and empty-body joining.
- Added shared memory/SQLite/env-gated Postgres contract coverage, including
  mismatch and limit skips, immediate-predecessor selection, missing/drained
  sources, identity preservation, and transaction rollback.
- Review remediation caps merged context-file metadata at 200 descriptors and
  makes the identity assertion tolerant of Postgres timestamp precision.
- Verification passed:
  - `cd apps/backend && go test -count=1 ./internal/orchestrator/messagequeue`
  - `cd apps/backend && go test -race -count=1 -run 'Test.*AutoMerge' ./internal/orchestrator/messagequeue`
- Blockers: none.
