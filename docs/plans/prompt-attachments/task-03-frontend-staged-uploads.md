---
id: "03-frontend-staged-uploads"
title: "Frontend staged uploads"
status: completed
wave: 3
depends_on: ["01-attachment-storage-api", "02-agent-delivery-and-queue"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prompt-attachments.md"
---

# Task 03: Frontend staged uploads

## Acceptance

1. Every existing task, subtask, new-session, chat, and queue composer accepts
   files up to 100 MiB/100 MiB aggregate, uploads them over HTTP, displays
   pending/uploading/ready/failed state, and submits only ready attachment IDs.
2. Retry, removal, expiry, and reload preserve prompt text and descriptor-only
   draft state; no attachment base64 is written to web storage, task/message
   JSON, or WebSocket payloads, and object URLs are revoked.
3. Desktop and mobile reuse the existing inline composer composition with
   localized copy, contained wrapping, accessible submit-state explanation, and
   visible 44px retry/remove touch targets for failed/incomplete uploads.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- components/task/chat/file-attachment.test.ts components/task/chat/use-chat-input-state.test.ts components/task-create-dialog-selectors.test.tsx components/task/session-dialog-shared.test.tsx lib/local-storage.test.ts lib/api/domains/attachment-api.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web i18n:check
cd apps && pnpm --filter @kandev/web i18n:ratchet
```

## Files likely touched

- `apps/web/lib/api/domains/attachment-api.ts` (new)
- `apps/web/lib/api/domains/attachment-api.test.ts` (new)
- `apps/web/components/task/chat/file-attachment.ts`
- `apps/web/components/task/chat/file-attachment-preview.tsx`
- `apps/web/components/task/chat/use-attachment-file-feedback.ts`
- `apps/web/components/task/chat/use-chat-input-state.ts`
- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-selectors.tsx`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task/session-dialog-shared.tsx`
- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/lib/services/session-launch-service.ts`
- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/local-storage.ts`
- `apps/web/src/locales/en/chat.json`
- `apps/web/src/locales/pseudo/chat.json`
- Focused unit/component tests beside the changed files

## Dependencies

- Task 01 supplies the browser upload/content/delete API.
- Task 02 supplies final task/message/queue descriptor shapes and delivery
  behavior.

## Parallelism

Sequential. It consumes both backend contracts and touches shared composer and
payload types used by the later E2E scenarios.

## Inputs

- Spec: What, API surface, Failure modes, Persistence guarantees, Scenarios
- Plan: Frontend / Upload API and attachment state; Mobile design contract
- Tasks 01-02 results and response/error shapes

## Output contract

Report each migrated composer, persisted draft shape, files changed, exact test
and i18n results, rendered desktop/mobile check evidence if performed, blockers,
risks, and synchronized task/plan status.

## Results

Implemented HTTP staging, upload state/retry/remove handling, descriptor-only task/message/queue payloads, draft persistence filtering, 100 MiB client validation, shared desktop/mobile composer behavior, and workspace resolution for passthrough/session composers. Pending uploads now disable only send controls and expose localized guidance while retry/remove remain available; uploads retry when workspace hydration completes. Focused attachment/task tests passed (82 tests across seven files, plus the pending-upload regression); the full web suite passed with 1,095 files and 8,439 tests (4 skipped). Typecheck, lint, i18n ratchet, and i18n checks passed.
