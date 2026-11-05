---
id: "02-composer-parity"
title: "Reuse the attachment composer in Office"
status: complete
wave: 2
depends_on:
  - "01-claim-creation-attachments"
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATION-ATTACHMENTS-001
  - REQ-OFFICE-TASK-CREATION-001
acceptance_criteria:
  - AC-TASKS-CREATION-ATTACHMENTS-001.1
  - AC-TASKS-CREATION-ATTACHMENTS-001.2
  - AC-TASKS-CREATION-ATTACHMENTS-001.3
  - AC-TASKS-CREATION-ATTACHMENTS-001.4
  - AC-TASKS-CREATION-ATTACHMENTS-001.5
  - AC-TASKS-CREATION-ATTACHMENTS-001.9
  - AC-OFFICE-TASK-CREATION-001.1
  - AC-OFFICE-TASK-CREATION-001.2
  - AC-OFFICE-TASK-CREATION-001.3
system_design:
  - ../../specs/tasks/system-design/task-creation-attachments.md
  - ../../specs/office/system-design/overview-01.md
---

# Task 02: Reuse the Attachment Composer in Office

## Summary

Use the Kanban composer in Office with the same image/file attachment controls.
Preserve draft identity, display previews, and submit only ready descriptors
through the accepted task creation path.

## In scope

- Extract the existing upload controller for default Kanban and controlled Office use.
- Keep the Office controller above Radix content for same-draft close/reopen.
- Reuse real file input, paste/drop handlers, context chips, thumbnails, and previews.
- Serialize ready descriptors, restore content URLs, and isolate draft generations.
- Guard processing/upload/expiry in both Create Task and its handler.
- Remove duplicate inactive Upload and empty dots controls.
- Preserve Office fields and add desktop/phone plus Kanban regression evidence.

## Out of scope

- New document renderers, blocker picker, or overflow actions.
- Backend ownership or scheduling changes beyond Task 01.
- New standalone upload code or a separate attachment model.

## Acceptance

1. Office offers Kanban file selection, image paste/drop, thumbnails, larger
   image preview, file metadata, upload status, retry, and removal.
   Existing property, assignee, source, and error flows remain functional.
2. Draft close/reopen, reload, context switch, discard, late completion, and
   success obey ownership rules. No bytes enter browser storage. No incomplete
   descriptor can pass the handler or delete a successfully claimed file.
3. Desktop keyboard and phone touch flows pass real upload/create tests.
   Phone details use the existing Drawer, preview returns focus, footer actions
   remain reachable, and no document overflow appears.

## ASCII UI preview

Use the same [plan views](plan.md#ascii-ui-preview) during implementation.
`UI-01` desktop:

```text
For [Assignee v] in [Project v]
+--------------------------------------------------+
| [IMG screenshot.png x] [FILE logs.zip x]           |
| Describe the issue...                            |
| [paperclip: Attach files]                         |
+--------------------------------------------------+
[Todo v] [Medium v]
[Discard Draft]                       [Create Task]
```

`UI-02` phone:

```text
+------------------------------+
| New issue                [X] |
|------------------------------|
| Title / assignee / project   |
| [IMG screenshot.png]     [x] |
| [FILE logs.zip]          [x] |
| Description...               |
| [paperclip]                  |
| [Todo v] [Medium v]           |
|------------------------------|
| [Discard]       [Create Task]|
+------------------------------+
tap chip -> inset details -> Open image -> larger viewer
```

`UI-03` states:

```text
[screenshot.png Uploading...] [x]        Create disabled
[logs.zip Upload failed] [Retry] [x]     Create disabled
```

Order, state, focus, and responsive structure are required. Spacing is illustrative.
The footer clears safe areas. Shared desktop density and 44px touch targets apply.
These views cover the acceptance criteria listed in this work order.

## Implementation notes

Start with `new-task-dialog-attachments.test.tsx`:
`opens the file picker and submits a ready attachment`.
The behavioral red is the missing functional attachment action or dropped
descriptor, not a missing new test ID.

Extract `useFileAttachments` into
`hooks/domains/task/use-task-create-attachments.ts`. Keep the existing
`TaskFormInputs` uncontrolled default and add one optional external controller.
Disable the unused internal controller in controlled mode.
Test both modes with the same upload behavior.

The Office adapter owns description callbacks and ready descriptor storage.
It reads the controller's current snapshot for submission. Start the pending
guard before FileReader/image decoding. Keep the controller alive during
same-draft closure and stop stale work on owner changes.

Use shared draft serialization and preview restoration. Ready descriptor URLs
support reloaded images and uploaded images without a local preview.
Treat rejected claim/expired content feedback as recoverable without clearing
the description or valid siblings. Success transfers ownership and clears only
the accepted draft. Discard releases staged files and invalidates callbacks.

Add controller and Office draft tests for selection/paste, retry/remove,
delayed upload/discard, context changes, restoration, and successful transfer.
Keep existing shared composer and image-preview tests as regression coverage.
Use the live responsive primitives. Add no global CSS selectors.

Add `new-task-attachments.spec.ts` and `mobile-new-task-attachments.spec.ts`
under `e2e/tests/office/`, with shared helpers in `new-task-attachments-helpers.ts`.
Import the existing disposable Office fixtures.
Use actual file bytes, real uploads, and causal HTTP waits.
Share or extract the deterministic clipboard helper pattern without importing
an unrelated scenario or weakening its existing assertions.

Include these scenarios on desktop and phone:

- Select PNG and ZIP, paste another PNG, inspect thumbnail/image and file details.
- Upload fails, Create stays guarded, retry succeeds, and a failed sibling can be removed.
- Plain text paste remains text. Unreadable image paste gives existing feedback.
- Reopen/reload ready draft, discard during an upload, and switch owner during a late result.
- Submit title/project with attachments and no description. Confirm IDs in
  the request and task creation association through Task 01's persisted evidence.
- Assert dots absence with the old SVG-bearing selector, not its missing name.
- Prove the phone Drawer, image focus return, touch targets, and internal scrolling.
  Assert zero horizontal overflow at phone and 767/768px boundaries.

Replace the old mobile test's inert dots tap with supported-action assertions.
Preserve its assignee and project-source checks and the desktop rejection scenario.

## Verification

In a fresh worktree, install dependencies from `apps/` once.
Run the first command red, then run this complete block after implementation:

```bash
(cd apps/web && pnpm test -- app/office/components/new-task-dialog-attachments.test.tsx app/office/components/new-task-draft.test.ts app/office/components/new-task-dialog.test.tsx app/office/components/new-task-dialog-create-payload.test.tsx app/office/components/new-task-bottom-bar.test.ts hooks/domains/task/use-task-create-attachments.test.tsx components/task-create-dialog-selectors.test.tsx components/task-create-dialog-selectors-attachment-lifecycle.test.tsx components/task/chat/image-preview-dialog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/office/new-task-attachments.spec.ts tests/office/new-task-dialog.spec.ts tests/task/create-task-attachment-warning.spec.ts tests/task/preparation-attachments.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/office/mobile-new-task-attachments.spec.ts tests/office/mobile-new-task-dialog.spec.ts tests/task/mobile-preparation-attachments.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `pnpm exec eslint` from `apps/web` with the exact changed TypeScript paths,
including the new hook, adapter, tests, and E2E helpers.
Run the plan's reference preflight. Record discovered test counts and rendered
comparison with `UI-01` through `UI-03`. Browser runs are sequential and rebuild.

## Files likely touched

All paths below start with `apps/web/`:

- `app/office/components/new-task-dialog.tsx`, `new-task-bottom-bar.tsx`, `new-task-draft.ts`.
- New `app/office/components/new-task-attachment-composer.tsx` adapter and Office tests.
- `components/task-create-dialog-selectors.tsx` and its existing attachment lifecycle tests.
- New `hooks/domains/task/use-task-create-attachments.ts` and hook tests.
- Shared descriptor/preview helpers only when necessary, with their existing tests.
- `e2e/tests/office/new-task-attachments.spec.ts`, `mobile-new-task-attachments.spec.ts`,
  `new-task-attachments-helpers.ts`, and the two existing creation suites.
- Locale catalogs only if existing copy cannot express required feedback.

## Dependencies

Task 01. Do not accept browser success as a substitute for its delivery proof.

## Risks

- Unmounting content can lose files or start duplicate uploads.
- Image decoding can race submission before an upload row appears.
- A live local preview can hide failure to restore an authorized content URL.
- Broad composer changes can regress Kanban behavior.

## Parallelism

`sequential`

## Inputs

- [Task requirement](../../specs/tasks/requirements/task-creation-attachments.md).
- [Task design](../../specs/tasks/system-design/task-creation-attachments.md).
- [Office requirement](../../specs/office/requirements/overview.md#req-office-task-creation-001-supported-creation-controls).
- [Office design](../../specs/office/system-design/overview-01.md#new-task-dialog).
- Scoped frontend guidance, `/mobile-parity`, `/tdd`, and `/e2e`.

## Results

Complete. `useFileAttachments` moved to `useTaskCreateAttachments` with an optional external controller. Office owns the controller above `DialogContent`, persists ready descriptors in the workspace/parent draft, restores them on reload, and submits ready IDs through `createTask.attachments`. File processing is published as a synchronous pending admission, and a generation/accepting guard prevents stale processing or upload results from entering a discarded or successor draft. Every collection mutation now notifies the Office owner, so removing a ready file updates storage. Submission freezes file admission, Discard, and close until the request settles; the controller is re-enabled on failure/success. The empty Upload and More options controls are removed. Image chips now fall back to the authorized content URL when no local preview exists.

Verification passed:

- Focused Vitest suites pass; the impacted Office/composer/chat area ran 196 files and 2,149 tests, including processing/discard, removal persistence, and frozen-submission regressions.
- `pnpm exec tsc --noEmit`
- `pnpm run i18n:check`
- `pnpm run i18n:ratchet`
- `pnpm exec eslint <changed TypeScript paths>`
- `pnpm run e2e:sleep-ratchet`
- `pnpm e2e:run --host --shards 1 --project chromium tests/office/new-task-attachments.spec.ts tests/office/new-task-dialog.spec.ts`: 7/7, including removal persistence across reload.
- `pnpm e2e:run --host --shards 1 --project mobile-chrome tests/office/mobile-new-task-attachments.spec.ts tests/office/mobile-new-task-dialog.spec.ts`: 3/3.

The backendapp materialization test proves the production starter receives the ordered image/resource descriptors and their exact bytes remain readable through the claimed attachment registry.
