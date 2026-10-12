---
id: "01-harden-queued-message-edit-fencing"
title: "Harden queued message edit fencing"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-MANAGEMENT-002
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.1
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.2
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.3
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.4
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.5
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.6
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.7
system_design:
  - ../../specs/ui/system-design/message-queue-edit.md
---

# Task 01: Harden queued message edit fencing

## Summary

Complete the queued-message edit contract on top of the target-lease
implementation at `b2fbe6d42`. Ensure automatic queue delivery holds only the
selected row, and reject stale, cross-connection, cross-session, expired, and
non-idempotent updates without corrupting queue or attachment state.

## In scope

- Backend lease acquisition, renewal, expiry, release, target revision, and
  operation-id fencing.
- Reservation and all queue mutation paths that can drain, remove, reorder,
  merge, transfer, or replace a leased target.
- WebSocket connection identity and authorization binding.
- Attachment claim/release ordering and entity-reference replacement.
- Frontend lease lifecycle, retry-stable operation IDs, session switching, and
  authoritative queue reconciliation.
- Focused backend/frontend unit tests plus desktop Chromium and mobile-chrome
  Playwright coverage.

## Out of scope

- New queue settings or changes to Auto-run defaults.
- Editing non-user provenance rows.
- Multi-process lease persistence.
- Broad lint, formatting, or project-wide test-suite changes.

## Acceptance

- Automatic delivery continues for non-target entries while the selected entry
  remains pending, and opening or cancelling an edit never toggles Auto-run.
- Only the live lease owner can save or release the target; stale revisions,
  operation-ID hash mismatches, session changes, expiry, and competing views
  fail closed and reconcile without overwriting newer state.
- Attachment and entity-reference replacement is atomic from the queue's
  perspective, with no leaked new claims or released retained attachments on
  rejected saves. Committed saves publish their authoritative status even when
  superseded-attachment cleanup initially fails; the authorized cleanup
  obligation is owned, cancellable, and reconciles removal and session
  transfer before retrying after the edit lease ends.

## Verification

```bash
cd apps/backend && go test -race ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers
cd apps && pnpm --filter @kandev/web test -- --run hooks/use-queue-edit-protection.test.ts lib/api/domains/queue-api.test.ts components/task/chat/queued-ghost-list.test.tsx
cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/message-queue.spec.ts -- --grep "queue editor|queued message edit"
cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/chat/mobile-message-queue-management.spec.ts -- --grep "edit queued message"
```

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/gateway/websocket/client.go`
- `apps/web/lib/api/domains/queue-api.ts`
- `apps/web/hooks/use-queue-edit-protection.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/components/task/chat/queued-ghost-list.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`

## Dependencies

None.

## Risks

- A queue mutation that bypasses the service admission lock can race lease
  invalidation or allow a target to be consumed after an edit begins.
- The shared desktop shell and mobile browser use the same web queue surface;
  E2E setup must keep the Auto-run backlog deterministic.

## Parallelism

`sequential`

## Inputs

- [Queued Message Editing system design](../../specs/ui/system-design/message-queue-edit.md).
- [Queued message management requirements](../../specs/ui/requirements/message-queue-management.md).
- Existing target-lease implementation and its focused tests at `b2fbe6d42`.
- Existing desktop and mobile queue Playwright specs.

## Results

Implemented target-scoped queue edit fencing and frontend lifecycle hardening:

- Preserved backend lease, revision, connection, session, operation, reservation, mutation, attachment, and entity-reference fencing.
- Prevented stale session completion callbacks from releasing replacement leases.
- Bound row save and cancel completion callbacks to the acquired edit identity, including same-session replacement.
- Reconciled superseded attachment cleanup inside the lease update's session admission boundary, before queued state can transfer.
- Detached attachment cleanup read paths from canceled request contexts so committed edits, rejected saves, and post-deletion cleanup still release claims.
- Removed edit revision state with entry, session, task, transfer, restore, merge, dequeue, and successful ordinary Send Now claim lifecycle invalidation.
- Released an active lease when its queued target disappears, using the same session-fenced completion path.
- Invalidated the target lease after FIFO coalescing replacement while holding session admission.
- Restored pending Send Now FIFO claims under the source session's admission lock, validating claim session identity before repository access.
- Ignored stale overlapping renewal successes by lease generation when available, and by renewal sequence when compatibility responses omit generations.
- Retried failed superseded-attachment finalization on an identical operation replay using the original pre-update attachment set.
- Ignored out-of-order or incomplete successful renewals so a newer lease generation cannot be replaced by an older response.
- Added operation-hash mismatch, FIFO replacement and head-rebase, target-removal, same-session completion, attachment cleanup ordering, cleanup retry, and cancellation, Send Now lifecycle, malformed claim identity and admission, physical head/tail selection, restore ordering after deletion and transfer, renewal ordering with and without generations, revision lifecycle, mobile save, and desktop/mobile session replacement coverage.

Focused verification:

- `cd apps/backend && go test -race ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers`: passed.
- Frontend focused tests passed: 4 files, 122 tests.
- `cd apps/web && pnpm --filter @kandev/web run typecheck`: passed.
- `cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/message-queue.spec.ts -- --grep "queue editor|queued message edit"`: passed, 2 tests.
- `cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/chat/mobile-message-queue-management.spec.ts -- --grep "edit queued message"`: passed, 3 tests.
