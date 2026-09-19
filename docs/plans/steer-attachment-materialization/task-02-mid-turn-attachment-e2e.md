---
id: "02-mid-turn-attachment-e2e"
title: "Cover mid-turn attachment delivery"
status: complete
wave: 1
depends_on:
  - "01-materialize-steer-attachments"
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.6
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 02: Cover Mid-Turn Attachment Delivery

## Summary

Add one Playwright scenario that uploads a file while a turn is still
generating and asserts the agent receives the file contents.

## In scope

- Add `apps/web/e2e/tests/chat/mid-turn-attachment.spec.ts`.
- Start a turn that keeps generating long enough to send a second message.
- Upload and send an attachment during that turn.
- Assert the agent resolves the file contents, not an empty reference.

## Out of scope

- Backend changes, which are Task 01.
- Queued-message delivery and delivery-mode coverage, already covered by
  `apps/web/e2e/tests/chat/attachment-delivery-mode.spec.ts`.

## Acceptance

- The scenario fails before Task 01 because the attachment resolves empty.
- The scenario passes after Task 01.
- The shared helper keeps the mock turn open for 60 seconds by default, which
  leaves setup headroom under CI load before the steer is submitted.

## Verification

```bash
(cd apps/web && pnpm run lint)
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
(cd apps/web && pnpm e2e:run tests/chat/mid-turn-attachment.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/chat/mid-turn-attachment.spec.ts`
