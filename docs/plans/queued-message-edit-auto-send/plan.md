---
created: 2026-09-07
status: completed
requirements:
  - REQ-UI-MESSAGE-QUEUE-MANAGEMENT-002
system_design:
  - ../../specs/ui/system-design/message-queue-edit.md
legacy_specs: []
---

# Implementation Plan: Queued Message Edit Auto-send

## Overview

When a user saves a queued message after its active turn has become promptable,
release the edit lease and resume ordinary FIFO delivery if Auto-run was already
enabled. Preserve the server-owned queue policy and make cancellation, failed
save, and Auto-run OFF non-dispatching outcomes.

## Scope

### In scope

- A successful fenced queued-message edit can request a policy-preserving drain
  when its edit lease ends.
- Backend lease, promptability, cancellation, and FIFO reservation races remain
  serialized through existing queue boundaries.
- Browser save completion distinguishes successful save from cancellation and
  sends the post-save dispatch intent only after the save succeeds.
- Desktop and mobile queue editing retain the shared inline composition and gain
  regression coverage for the same save outcome.

### Out of scope

- Changing Auto-run defaults, persistence, or the explicit Send Now contract.
- Dispatching a cancelled or failed edit.
- Sending a non-head edited row ahead of earlier FIFO entries.
- New queue UI, layout, or touch interaction patterns.

## Technical approach

Extend the queued edit end lifecycle with a server-validated
`dispatch_if_auto_run` intent. The queue edit service records whether the live
lease completed a successful update, so cancellation and failed-save cleanup
cannot authorize a dispatch. After validating and releasing the lease, the
queue handler invokes an orchestrator drain that rechecks session promptability,
cancellation ownership, in-flight dispatch state, and the persisted Auto-run
policy without enabling Auto-run. Existing FIFO reservation and queue status
publication remain authoritative.

The browser preserves the current lease and operation-id flow. A successful
`updateQueuedMessage` result marks the edit completion as a save; the editor's
cancel, lease-loss, and error paths omit the dispatch intent. Both desktop and
mobile use the same hook and queue row, so no separate responsive implementation
is introduced.

## Mobile design contract

The existing inline queue panel remains the mobile composition and the queue
list remains its single internal scroll owner. The Save and Cancel actions stay
visible in the existing touch-sized edit row; the primary outcome is unchanged
content, with no new drawer or navigation. Desktop and mobile share lease,
save-result, and post-save dispatch state. The mobile Playwright flow edits and
saves the FIFO head while Auto-run is enabled, then asserts that the entry is
delivered without a second queue action and without document horizontal overflow.

## Tests

- `apps/backend/internal/orchestrator/messagequeue/edit_lease_test.go` covers
  successful-save authorization, cancellation/failed-save denial, Auto-run OFF,
  and lease release before policy-preserving drain.
- `apps/backend/internal/orchestrator/handlers/queue_handlers_edit_test.go`
  covers the end-edit request flag, connection authorization, and the drain
  call only after a successful save.
- `apps/web/hooks/use-queue-edit-protection.test.ts`,
  `apps/web/lib/api/domains/queue-api.test.ts`, and the queue editor tests cover
  save versus cancellation completion and the new request field.

## E2E tests

- Desktop Chromium: extend `apps/web/e2e/tests/chat/message-queue.spec.ts`
  with a queued-head edit whose active turn completes before save.
- Mobile Chrome: extend `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
  with the same save-and-deliver flow using the mobile editor controls.

## Work orders

- [x] [Task 01: Dispatch saved queued edits](task-01-dispatch-saved-queued-edits.md)

## Verification results

- Backend policy, lease, and handler regression tests passed with `go test
  -race -run 'TestDrainQueuedMessageIfAutoRunDoesNotResumePausedQueue|TestEndEditAfterSaveReportsOnlyFinalizedUpdates|TestWsEndEditDispatchesOnlyAfterSuccessfulSave'
  ./internal/orchestrator ./internal/orchestrator/messagequeue
  ./internal/orchestrator/handlers`.
- Frontend queue lifecycle tests passed: 4 files, 125 tests.
- Web TypeScript typecheck passed with `pnpm exec tsc --noEmit`.
- Changed web files passed ESLint with zero errors and zero warnings.
- Desktop and mobile E2E scenarios were added and attempted. Both fixture
  runs stopped during task seeding because `POST /api/v1/tasks` returned HTTP
  500 (`{"error":"request failed"}`), before the queue flow executed.

## Risks

- The end-edit request is a WebSocket contract change and must retain existing
  cancel, disconnect, and stale-lease behavior for older clients.
- A post-save drain may lose a race to another eligible queue trigger; the saved
  row must remain durable and be delivered by that winner or the next trigger.
