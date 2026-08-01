---
id: message-queue-merge-01
title: Backend repository merge
status: completed
wave: 1
depends_on: None
plan: plan.md
spec: ../../specs/ui/message-queue-merge.md
---

# Task 01: Backend repository merge

## Acceptance

1. `Repository.MergeIntoAbove(ctx, sessionID, sourceID, queuedBy)` is defined
   in `internal/orchestrator/messagequeue/repository.go` and implemented in
   both `repository_sqlite.go` (single transaction) and `repository_memory.go`,
   with `ErrNoMergeTarget` and `MetadataSenderTaskID` added to `types.go`.
2. The same-sender-kind rules hold: user merges only into a user entry owned by
   the caller; agent merges only into an agent entry with the identical
   `sender_task_id`; everything else (head, workflow/server/system, mismatched
   kind, in-flight reserved target) returns `ErrNoMergeTarget`; a drained
   source returns `ErrEntryNotFound`.
3. Merged entry keeps the target's identity (id/position/queued_by/queued_at);
   content joins with `\n\n` (no leading blank line when the target content is
   empty), attachments concatenate, `entity_references` metadata is the
   ref-deduplicated union; the source row is deleted; ordering of remaining
   entries is preserved.

## Verification

- `cd apps/backend && go test -race ./internal/orchestrator/messagequeue/...`

## Files

- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite_merge_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service_test.go`

## Inputs

- Spec sections: `API surface`, `Data model`, `Permissions`, `Failure modes`,
  `Scenarios` (all merge scenarios).
- Plan sections: `Backend → Message-queue package`, `Tests` (repository rows).
- Pattern to mirror: `UpdateContentAndMetadata` guard + tx in
  `repository_sqlite.go:691`; `append`/`applyMetadataUpdates` helpers; the
  memory repo's locking style.

## Risks

- Function size/complexity limits (≤80 lines, ≤50 statements, complexity ≤15)
  — extract the kind gate and the reference-union into helpers.
- goconst: reuse a new `MetadataSenderTaskID` constant for `"sender_task_id"`.
- Reserved lifecycle rows: a reserved-in-flight target must be rejected, never
  merged into.
