---
id: "01-queue-attachment-rendering"
title: "Render queued attachment IDs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/prompt-attachments.md"
---

# Task 01: Render queued attachment IDs

## Acceptance

1. A queued image descriptor with `attachment_id` and no inline `data` renders
   from `/api/v1/attachments/<id>/content` in both thumbnail and full-size
   preview contexts for callers authorized to read the owning task.
2. Legacy inline image descriptors continue to render from their MIME-qualified
   data URL, and missing image sources do not produce a broken `<img>` URL.
3. Existing queue thumbnail dimensions, dialog behavior, edit-mode read-only
   behavior, and desktop/mobile row interaction remain unchanged.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/queued-ghost-message.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/components/task/chat/queued-attachment-row.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message.test.tsx`
- `apps/backend/internal/task/service/attachment_service.go`
- `apps/backend/internal/task/service/attachment_service_lifecycle_test.go`
- `apps/backend/internal/backendapp/orchestrator.go`

## Dependencies

None.

## Parallelism

Sequential. The type, renderer, and regression test form one shared frontend
contract.

## Inputs

- `docs/specs/tasks/requirements/prompt-attachments.md`, queued preview scenario
- `docs/plans/queued-message-attachment-preview/plan.md`
- Existing source resolution in
  `apps/web/components/task/chat/messages/chat-message.tsx`

## Output contract

Report the final source-resolution behavior, files changed, exact test and
typecheck results, any mobile verification limitation, and synchronized task
and plan status.

## Results

- RED: `rtk pnpm exec vitest run components/task/chat/queued-ghost-message.test.tsx`
  from `apps/web` failed as expected with 2 failures: staged descriptors became
  `data:image/png;base64,undefined`, and source-less images rendered a preview
  trigger.
- GREEN: `rtk pnpm --filter @kandev/web test -- --run
  components/task/chat/queued-ghost-message.test.tsx` from `apps` passed with
  44 tests.
- Typecheck: `rtk pnpm run typecheck` from `apps/web` passed.
- Backend authorization regression: `rtk go test -v ./internal/task/service -run
  'TestAttachment(OpenClaimedAllowsAuthorizedTaskReader|OpenClaimedAuthorizesByBinding|OpenReturnsBytesAndScopes)' -count=1`
  passed, 3 tests.
- Formatting: `rtk git diff --check` passed.
- Mobile verification: no new Playwright test was needed because the repair
  only changes attachment source resolution inside the existing row; the
  existing `mobile-message-queue-management.spec.ts` remains the relevant
  mobile surface.
- Cleanup: no temporary diagnostic or capture artifacts were created.
