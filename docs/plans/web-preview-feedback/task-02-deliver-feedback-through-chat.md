---
id: "02-deliver-feedback-through-chat"
title: "Deliver preview feedback through chat"
status: completed
wave: 2
depends_on:
  - "01-persist-and-sync-feedback"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-003
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.6
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
---

# Task 02: Deliver Preview Feedback Through Chat

## Summary

Add preview feedback to every task composer and extend direct and queued message
admission so Send formats, attaches, and consumes the selected task snapshot as
one durable operation.

## In scope

- `preview_feedback_refs` on direct-message and queue contracts, caller replay
  fingerprints, validation errors, and persisted metadata.
- Canonical server formatting for text, rendered DOM anchors and coordinates,
  escaped element HTML, screenshot names, page context, and mixed plan-comment
  submissions.
- One shared atomic envelope for exact plan-comment and preview-feedback
  consumption plus ordinary and screenshot attachment claims.
- Direct, busy-session queue, Send Now, replay, capacity, rollback, and
  concurrent-send coverage.
- One composer context item per task, empty-text submission, selected-session
  routing, submit settlement, and shared pending-panel entry.
- Localized count, conflict, attachment-limit, and send-failure copy.

## Out of scope

- A preview-panel Send or per-item Run control.
- Selection capture and marker rendering.
- Screenshot rasterization and upload UI.

## Acceptance

- Send to an idle or busy selected session persists the same visible prompt and
  attachments and consumes only the frozen exact feedback versions.
- Any stale reference, route, capacity, prompt, or attachment error rolls back
  message or queue admission and preserves both feedback and composer content.
- A lost response or retry with the same unchanged identity resolves to one
  accepted delivery, while feedback added after the snapshot remains pending.

## ASCII UI preview

Use [UI-03 in the plan](plan.md#ui-03-ordinary-composer-delivery). The count chip
is task-owned and opens the shared collection; the existing composer Send button
is the only delivery action.

```text
┌ Context ────────────────────────────────────────────────────────────┐
│ [Preview feedback · 3]  [Plan comments · 1]                       │
├─────────────────────────────────────────────────────────────────────┤
│ Add an optional message...                              [Send]     │
└─────────────────────────────────────────────────────────────────────┘
```

## TDD sequence

1. Add failing canonical-format and direct/queue transaction tests for exact
   refs, runtime text anchors, coordinates, mixed context, attachment transfer,
   rollback, races, and replay; run the focused backend packages RED.
2. Implement the shared admission resolver and repository transactions until
   all focused backend tests pass under `-race`.
3. Add failing composer/context/submission tests for selected-session routing,
   empty text, busy queueing, preserved failures, and post-snapshot additions;
   run them RED.
4. Implement frontend reference submission and settlement, rerun GREEN, then
   simplify only after direct and queue behavior remain identical.

## Verification

```bash
cd apps/backend
go test -race ./internal/task/previewfeedback ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers
cd ../../apps
pnpm --filter @kandev/web test -- --run components/task/chat/chat-context-items.test.ts components/task/chat/use-chat-panel-state.test.tsx hooks/use-message-handler.test.ts lib/comments/format-preview-feedback.test.ts
cd web
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm exec eslint components/task/chat hooks/use-message-handler.ts lib/comments/format-preview-feedback.ts
```

## Files likely touched

- `apps/backend/internal/task/previewfeedback/format*.go`
- `apps/backend/internal/task/repository/sqlite/message_preview_feedback*.go`
- `apps/backend/internal/task/repository/sqlite/attachment*.go`
- `apps/backend/internal/task/service/service_messages*.go`
- `apps/backend/internal/task/handlers/message_handlers*.go`
- `apps/backend/internal/orchestrator/messagequeue/`
- `apps/backend/internal/orchestrator/handlers/queue_handlers*.go`
- `apps/web/components/task/chat-context-items*.ts`
- `apps/web/components/task/chat/use-chat-panel-state*.ts`
- `apps/web/hooks/use-message-handler*.ts`
- `apps/web/lib/comments/format-preview-feedback*.ts`
- `apps/web/lib/api/types/message*.ts`
- `apps/web/src/locales/*/task.json`

## Dependencies

- Task 01 supplies durable rows, exact versions, screenshots claims, snapshots,
  and authorization.

## Risks

- Adding a second transaction around the existing plan-comment resolver would
  create partial consumption; one shared leaf transaction must own both.
- Queue auto-merge must not erase the caller identity or attachment/ref
  fingerprint needed for replay.
- User-controlled page strings must remain quoted user content and cannot break
  Markdown fences or enter system markup.

## Parallelism

`sequential`

## Inputs

- Composer projection, canonical prompt, and atomic acceptance sections in the
  system design.
- Existing task-owned plan-comment admission, queue receipt, and attachment
  claim tests.

## Results

- Added exact `preview_feedback_refs` to direct and queued message contracts,
  replay fingerprints, task/session admission leases, and changed-snapshot
  errors.
- Added one shared transaction for plan comments, preview feedback, ordinary
  staged attachments, screenshot transfers, queue capacity, and durable
  message or queue acceptance.
- Added the canonical server formatter with complete runtime text anchors,
  escaped rendered element HTML, screenshot context, page grouping, and prompt
  bounds.
- Projected the task-owned pending count into every composer and froze visible
  versions on Send without client-formatted duplication or local clearing.
- Verified focused backend packages under the race detector and 51 frontend
  submission, queue, context, and settlement tests; web typecheck and i18n
  validation pass.
