---
id: "01-backend-repository-reorder"
title: "Backend repository reorder"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/message-queue-reorder.md"
---

# Task 01: Backend repository reorder

## Acceptance

1. `Repository` exposes `ReorderEntries(ctx, sessionID string, orderedIDs []string) error` and both implementations (`sqliteRepository`, `memoryRepository`) rewrite positions to `1..N` in the submitted visible order, interleaving reserved in-flight rows at their existing places.
2. Any set drift (missing/extra/duplicate id, empty list, reserved id) returns `ErrQueueChanged` and leaves the queue byte-for-byte unchanged (atomic rollback / no mutation).
3. New table-driven tests cover both implementations: reversal, no-op, every drift case, reserved-row interleaving, and a drain race.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/messagequeue/...
cd apps/backend && go vet ./internal/orchestrator/messagequeue/...
```

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/repository.go` — interface method.
- `apps/backend/internal/orchestrator/messagequeue/types.go` — `ErrQueueChanged`.
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go` — `ReorderEntries` (tx under `withSessionLock`, select-then-validate-then-rewrite; `RowsAffected` check per row; pattern: `MergeIntoAbove`/`applyMergeWrites`).
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go` — mirror under `r.mu`; sort a position copy before split (slice is not guaranteed sorted after `ReplaceSession`); bump `nextPosition`.
- `apps/backend/internal/orchestrator/messagequeue/repository_reorder_test.go` — new.

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- Spec: `## API Surface` (`message.queue.reorder`), `## Data Model`, `## Failure Modes`.
- Plan: `### Repository contract`, `### SQLite`, `### Memory`.
- Existing patterns: `MergeIntoAbove` in both repositories, `repository_send_now_test.go` (dual-impl table), `GetStatus` reserved filtering (`service.go`), `IsReservedInFlight()`.

## Output contract

Summary, files changed, exact test commands and outcomes, blockers, risks; update task + plan statuses in the same conversation.

## Results

- `cd apps/backend && go test -count=1 -run 'TestReorder' -v ./internal/orchestrator/messagequeue/...` → PASS: `TestReorderEntriesRewritesVisibleOrder`, `TestReorderEntriesNoOpKeepsPositions`, `TestReorderEntriesRejectsDriftAtomically` (missing/unknown/duplicate/empty), `TestReorderEntriesKeepsReservedRowInPlace`, `TestReorderEntriesDrainRaceRejected` — each green on memory, sqlite, and postgres.
- `cd apps/backend && go test ./internal/orchestrator/messagequeue/...` → `ok`.
- Files: `repository.go` (interface), `types.go` (`ErrQueueChanged`), `reorder.go` (new shared helpers), `repository_sqlite.go`, `repository_memory.go`, `repository_reorder_test.go` (new).
