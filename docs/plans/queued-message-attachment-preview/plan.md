---
spec: docs/specs/tasks/prompt-attachments.md
created: 2026-08-13
status: completed
---

# Implementation Plan: Queued message attachment previews

## Overview

Repair the queue panel's image preview contract after file-backed prompt
attachments were introduced. The frontend queue DTO and renderer will accept
both descriptor-backed attachments and legacy inline data, using the same
authorized content URL fallback already used by persisted chat messages. This
is a frontend-only repair; the backend queue contract already returns
`attachment_id`.

Confirmed root cause: `use-chat-input-state.ts` submits ready uploads as
`attachment_id`, while `QueuedMessage` still requires `data` and
`queued-ghost-message.tsx` always constructs a base64 data URL from it. The
existing queue tests only exercise inline data fixtures.

---

## Frontend

### Queue attachment contract

- Extend `apps/web/lib/state/slices/session/types.ts` so queued attachment
  descriptors allow optional `data` and `attachment_id`, matching the queue
  API and backend `MessageAttachment` JSON shape.
- Update `apps/web/components/task/chat/queued-ghost-message.tsx` to resolve
  `attachment_id` with `attachmentContentUrl()` before falling back to an
  inline MIME-qualified data URL. Do not render an invalid image URL when both
  sources are absent.
- Preserve the existing compact 40x40 thumbnail and dialog/edit interaction;
  thumbnail geometry and crop policy are out of scope.

### Mobile parity

- **Outcome and entry:** desktop and phone queue panels show the same image
  preview content for a queued attachment; the existing queue chip/panel entry
  remains unchanged.
- **Nearest exemplar:** reuse the persisted chat message resolver in
  `apps/web/components/task/chat/messages/chat-message.tsx`; the existing
  `mobile-message-queue-management.spec.ts` remains the closest mobile queue
  surface.
- **Hierarchy and action:** no action or navigation changes. The existing
  thumbnail remains the preview trigger on both pointer types.
- **Presentation and scroll:** no new overlay or layout is introduced; the
  current queue panel remains the scroll owner and the 40x40 thumbnail remains
  within the row.
- **Shared logic:** descriptor resolution and inline fallback are shared by
  the queue display and edit preview through the existing component path.

---

## Tests

- **What:** a queued image with `attachment_id` and no inline data uses the
  authorized content URL, and a legacy inline image still uses its data URL.
  **File:** `apps/web/components/task/chat/queued-ghost-message.test.tsx`.
  **How:** focused React Testing Library regression tests using the existing
  queue fixture and preview dialog behavior.
- **What:** an attachment with neither source does not create a broken image
  element. **File:** the same test file. **How:** focused rendered fallback
  assertion if the implementation exposes this defensive case.

## E2E Tests

No new Playwright test is planned. This repair only normalizes the image source
inside the already-shipped queue row and does not change mobile composition,
touch targets, scrolling, or navigation. The deterministic staged-descriptor
regression belongs in the component test; the existing desktop and mobile
queue-management scenarios continue to cover row mounting and interaction.

---

## Verification Results

- `rtk pnpm --filter @kandev/web test -- --run
  components/task/chat/queued-ghost-message.test.tsx` from `apps`: passed,
  44 tests.
- `rtk pnpm run typecheck` from `apps/web`: passed.
- `rtk git diff --check`: passed.
- No new mobile Playwright test was added because the repair does not change
  composition, touch targets, scrolling, navigation, or viewport behavior.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-queue-attachment-rendering](task-01-queue-attachment-rendering.md) — completed; sequential

Execution remains sequential in the primary conversation.

## Open Questions

None.
