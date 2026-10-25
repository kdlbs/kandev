---
id: "01-dispatch-saved-queued-edits"
title: "Dispatch saved queued edits"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-MANAGEMENT-002
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.8
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-002.9
system_design:
  - ../../specs/ui/system-design/message-queue-edit.md
---

# Task 01: Dispatch saved queued edits

## Summary

Make a successful queued-message edit release its lease and request ordinary
FIFO delivery when Auto-run is already enabled and the session is promptable.
Keep cancellation, failed saves, stale leases, and Auto-run OFF from initiating
delivery, while preserving existing queue reconciliation and mobile behavior.

## In scope

- Extend the backend edit-end contract and queue handler for a validated
  post-save dispatch intent.
- Preserve successful-save state on the edit lease and call a policy-preserving
  orchestrator drain after lease release.
- Propagate save-versus-cancel completion through the web queue editor and API.
- Add focused backend/frontend tests plus desktop and mobile queue E2E coverage.

## Out of scope

- Changes to Auto-run persistence or its default value.
- Send Now, queue ordering, merge, remove, or attachment semantics unrelated to
  edit completion.
- New responsive components or alternate mobile navigation.

## Acceptance

- With Auto-run enabled, a queued head edited while its turn is active is sent
  automatically after the save releases the edit lease once the session is
  promptable.
- Auto-run OFF, cancellation, stale/failed saves, and lease-loss cleanup leave
  the saved or existing row pending and do not start a turn from edit cleanup.
- Desktop and mobile save flows use the same request/result semantics and retain
  their current touch targets, scroll ownership, and no-overflow behavior.

## Verification

```bash
cd apps/backend && go test -race ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers ./internal/orchestrator
cd apps && pnpm --filter @kandev/web test -- --run hooks/use-queue-edit-protection.test.ts lib/api/domains/queue-api.test.ts components/task/chat/queued-ghost-list.test.tsx
cd apps/web && pnpm e2e:run tests/chat/message-queue.spec.ts -- --grep "edit.*Auto-run|save.*queued"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts -- --grep "edit.*Auto-run|save.*queued"
```

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/edit_lease_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_edit_test.go`
- `apps/web/lib/api/domains/queue-api.ts`
- `apps/web/hooks/use-queue-edit-protection.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-list.tsx`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`

## Dependencies

None.

## Risks

- The dispatch intent must be server-validated against a successful update on
  the same live lease; trusting a browser boolean would let cancellation resume
  the queue unexpectedly.
- The post-save drain must not enable Auto-run or duplicate a reservation that
  another trigger already owns.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/message-queue-management.md`
- `docs/specs/ui/system-design/message-queue-edit.md`
- Existing queued-message edit fencing implementation and queue auto-run tests.

## Results

Implemented the server-validated save completion path. Successful fenced
updates mark the live lease as dispatch-authorized; the end-edit handler
releases that lease and invokes the policy-preserving orchestrator drain.
Cancellation, failed save, stale lease cleanup, and Auto-run OFF do not invoke
the drain. The web editor now passes save completion separately from cancel
completion, and desktop/mobile E2E coverage was added.

Focused verification passed:

- Backend targeted race tests passed for paused-policy preservation, finalized
  save authorization, and handler dispatch gating.
- Frontend queue lifecycle tests passed: 4 files, 125 tests.
- Web TypeScript typecheck and changed-file ESLint passed.

Desktop and mobile E2E runs were attempted but both stopped during fixture task
creation because `POST /api/v1/tasks` returned HTTP 500
(`{"error":"request failed"}`); neither reached the queue interaction.
