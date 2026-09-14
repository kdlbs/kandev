---
created: 2026-09-02
status: completed
requirements:
  - REQ-UI-MESSAGE-QUEUE-MANAGEMENT-002
system_design:
  - ../../specs/ui/system-design/message-queue-edit.md
legacy_specs: []
---

# Implementation Plan: Queued Message Edit Fencing

## Overview

Harden the queued-message editor so automatic delivery continues for every
other pending entry while the selected entry remains protected. The work first
closes backend lease and reservation races, then aligns the browser lifecycle,
attachment ownership, and desktop/mobile coverage with the fenced contract.

## Scope

### In scope

- Target-bound edit leases with server connection fencing, revision checks, and
  idempotent operation replay.
- Reservation, removal, reorder, merge, transfer, expiry, disconnect, and queue
  replacement behavior for a leased target.
- Attachment and entity-reference replacement rollback behavior.
- WebSocket/store reconciliation after stale, cross-session, and concurrent
  responses.
- Desktop and mobile user-flow coverage for editing during Auto-run.

### Out of scope

- Editing agent-, workflow-, or server-origin entries.
- Changes to queue capacity, merge compatibility, or Send Now semantics except
  where those operations must respect an active target lease.
- Multi-user queue permissions beyond existing session authorization.
- A separate desktop-native queue UI; the desktop shell exercises the shared web
  surface.

## Technical approach

The queue service remains the source of truth and uses its existing per-session
admission boundary. Lease state is keyed by session and entry, stores the
server-issued lease ID, connection binding, expiry/generation, target revision,
and the last operation hash/result. Policy-aware automatic reservation skips only
when its current visible head is the leased target. Queue replacement and
session transfer invalidate affected leases under the same admission boundary.

Queue handlers validate the lease preconditions before claiming staged
attachments, and release only claims made by a rejected operation. A committed
replacement publishes its authoritative status before any transient
superseded-attachment cleanup failure is surfaced internally. The original
cleanup set retains authorization context, follows removal or session
transfer, and is owned by a cancellable retry lifecycle that waits for lease
termination before retrying under session admission. Browser updates retain one
operation ID through request retries and discard responses
that no longer belong to the active session or target. Disconnect and session
switch cleanup is lease-fenced so delayed callbacks cannot end a newer lease.

## Tests

- `apps/backend/internal/orchestrator/messagequeue/edit_lease_test.go` covers
  target-only reservation blocking, expiry, renewal, idempotency, stale
  revisions, transfer/replacement, and lease interaction with other queue
  mutations.
- `apps/backend/internal/orchestrator/handlers/queue_handlers_edit_test.go`
  covers connection binding, authorization, precondition failures, and
  attachment claim/release behavior.
- `apps/web/hooks/use-queue-edit-protection.test.ts` and
  `apps/web/lib/api/domains/queue-api.test.ts` cover stable operation IDs,
  session changes, renewal expiry, stale cleanup, and error mapping.
- `apps/web/components/task/chat/queued-ghost-list.test.tsx` covers the editor
  staying on the selected target while other rows remain actionable.

## E2E tests

- Desktop `chromium`: extend `apps/web/e2e/tests/chat/message-queue.spec.ts`
  with editing during an Auto-run backlog and stale-session reconciliation.
- Mobile `mobile-chrome`: extend or add
  `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts` with the
  touch edit/save path, target retention, and no horizontal overflow.
- The shared web surface is the desktop-app queue path; no second native desktop
  Playwright project is required.

## Work orders

- [x] [Task 01: Harden queued message edit fencing](task-01-harden-queued-message-edit-fencing.md)

## Verification results

Focused verification completed:

- Backend race tests passed: `go test -race ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers`, including physical head/tail lease selection, restore and transfer position ordering, FIFO head-rebase high-water persistence, failed superseded-attachment cleanup replay, attachment-finalization ordering, cancellation-detached cleanup for edits and deletion, Send Now edit-state cleanup, claim admission and identity validation, and edit-revision lifecycle coverage. The queue edit protection hook passed 11 focused tests, including generation-free overlapping renewal ordering.
- Frontend focused tests passed: 4 files, 122 tests. The documented command's `web/` prefixes are package-relative and produced `No test files found`; the equivalent paths from `apps/web` passed.
- Web typecheck passed: `pnpm --filter @kandev/web run typecheck`.
- Desktop Chromium queue-editor E2E passed: 2 tests matched the documented grep.
- Mobile Chrome queued-message edit E2E passed: 3 tests matched the documented grep.

## Risks

- Lease state is process-local; a multi-process deployment would require a
  durable lease store or single-writer routing, which is outside this change.
- Existing queue mutation paths may bypass the service admission helper and
  need careful synchronization before they can safely invalidate leases.
