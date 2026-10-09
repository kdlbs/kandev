---
id: "02-draft-preservation"
title: "Preserve screenshot draft comments"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-002.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.4
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
---

# Task 02: Preserve screenshot draft comments

## Summary

Keep the typed comment and PNG through successful upload and rejected creation.
Retry the same logical capture without another upload or accidental draft reset.

## In scope

- Stable logical draft identity and controller-owned comment state.
- Upload enrichment, save settlement, duplicate-save prevention, and retry.
- Closure and reopening of the form while the capture controller remains mounted.
- Protection against a late response clearing a replacement draft.
- Shared behavior for desktop Popover and touch Drawer.

## Out of scope

- Persistent browser drafts, reload recovery for unsaved captures, or a new API.
- General screenshot rasterization, route capture, and resource-cleanup repairs.
- Multi-draft create-idempotency changes covered by another existing PR comment.

## Root cause and reproduction

`DraftEditor` resets its local comment on every `capture.draft` change.
`useSavePreviewDraft` replaces that object after upload to attach the server ID.
The effect clears the comment before feedback creation succeeds.

Mount the real capture controller with the editor. Enter a comment, resolve the
upload, and reject create. The current editor shows an empty field even though
the hook retains the PNG. A hook-only test cannot prove comment preservation.

## Implementation approach

1. Add the real-controller component regression before production changes.
2. Keep comment state with the capture draft owner in `use-preview-capture.ts`.
3. Assign a stable local identity when a new capture becomes a draft.
4. Retain that identity and comment when upload adds `attachmentId`.
5. Replace the object-identity reset effect with controlled editor state.
6. Clear only the matching draft after successful create or explicit discard.
7. Guard terminal callbacks with the saved draft identity or generation.
8. Disable repeated save and conflicting replacement actions while save is pending.

Keep the existing API payload and task-scoped saved collection unchanged.
Do not delete a claimed image while settling a successful create.
Do not make panel dismissal equivalent to discard. A new capture or explicit
discard resets the comment. A mere Popover or Drawer remount does not.

## Acceptance

1. After upload succeeds and create fails, the editor retains the comment,
   PNG preview, attachment ID, and visible error on desktop and touch.
2. Retry uploads no second image and sends the retained comment. Successful
   creation clears only that draft, while explicit discard starts cleanly.
3. Repeated save cannot create parallel requests. A late response cannot clear
   a newer draft or a draft for another task/source.

## ASCII UI preview

Use [UI-R1 in the plan](plan.md#ui-r1-capture-draft-after-create-failure).
The same form uses a desktop Popover and a touch Drawer.

```text
Before: [PNG] [empty comment]                         [Save disabled]
After:  [PNG]
        [Keep this typed feedback                                 ]
        [Localized create error                                   ]
                                                 [Discard] [Retry]
```

The thumbnail, comment, error, and action order remain stable during retry.
Use a localized live status for upload and error state. Keep the action name
stable while busy. The touch Drawer retains one scroll owner and safe-area space.

## Tests

- Extend `use-preview-capture.test.ts` with deferred upload/create responses.
- Extend `preview-feedback-controls.test.tsx` with the real hook and mocked API
  boundary. Do not replace the controller with a fixed draft for this regression.
- Cover upload failure, create failure, retry success, explicit discard,
  close/reopen, repeated save, and delayed completion after replacement.
- Assert one upload, stable create payload, retained comment, and one saved row.
- Task 03 adds the shared desktop/mobile E2E scenario after this fix.

## Verification

Run from the repository root. For a fresh worktree, first install dependencies
with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-preview-capture.test.ts components/task/inspector/preview-feedback-controls.test.tsx hooks/domains/comments/use-preview-feedback.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/use-preview-capture.ts components/task/inspector/preview-feedback-controls.tsx)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

The real-controller test supplies behavioral RED. Record both pointer-mode
cases. Keep public copy in existing localization keys where possible.

## Files likely touched

- `apps/web/hooks/use-preview-capture.ts`
- `apps/web/hooks/use-preview-capture.test.ts`
- `apps/web/components/task/inspector/preview-feedback-controls.tsx`
- `apps/web/components/task/inspector/preview-feedback-controls.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json` if copy changes

## Dependencies

None technically. Execute after Task 01 in the planned sequence.
Task 03 depends on this editor ownership before extraction.

## Risks

- Object identity is not logical draft identity.
- Clearing after an awaited call without a generation check can erase a successor.
- Resource release during a state updater can run more than once in StrictMode.
  Do not add cleanup side effects to state updaters.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md).
- [Design: Logical capture draft](../../specs/tasks/system-design/web-preview-feedback.md#logical-capture-draft).
- Existing `use-preview-capture.test.ts` deferred-operation fixtures.
- `apps/web/components/task/inspector/preview-feedback-controls.tsx`: `DraftEditor`.

## Results

Implemented. `use-preview-capture.ts` now owns a logical draft identity and
controlled comment state. Upload metadata enriches the existing screenshot
draft, while create failure retains the PNG, comment, and attachment ID. Only
successful create or explicit discard settles the matching draft, with
generation guards preventing late work from clearing a replacement.

The real controller regression covers upload success, create failure, retained
comment and image, retry without a second upload, and successful cleanup. The
focused frontend suites passed with 62 tests, and web typecheck, scoped lint,
localization checks, and both desktop and mobile preview E2E paths passed.
