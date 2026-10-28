---
id: "03-public-docs"
title: "Update public guidance and the screenshot"
status: complete
wave: 3
depends_on:
  - "02-composer-parity"
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATION-ATTACHMENTS-001
  - REQ-OFFICE-TASK-CREATION-001
acceptance_criteria:
  - AC-TASKS-CREATION-ATTACHMENTS-001.1
  - AC-TASKS-CREATION-ATTACHMENTS-001.2
  - AC-TASKS-CREATION-ATTACHMENTS-001.3
  - AC-OFFICE-TASK-CREATION-001.1
system_design:
  - ../../specs/tasks/system-design/task-creation-attachments.md
  - ../../specs/office/system-design/overview-01.md
---

# Task 03: Update Public Guidance and the Screenshot

## Summary

Document the supported creation attachment flow and show the implemented dialog.
Keep the current public screenshot path and its existing references.

## In scope

- Add concise Attach files/paste/preview/retry guidance to `docs/public/office-tasks.md`.
- State the shared count/size limits and the distinction between image and file previews.
- Replace `docs/screenshots/office-new-task.png` from real disposable Office data.
- Remove temporary capture code and owned runtime data after capture.

## Out of scope

- New marketing media or landing assets.
- New UI, file formats, or generic archive/PDF rendering promises.
- Pixel editing or DOM changes that conceal controls during capture.

## Acceptance

1. Public creation instructions describe only implemented file, paste, preview,
   retry/remove, and submission behavior. Limits match the shared contract.
2. The screenshot shows the working composer, image thumbnail/file chip,
   property row, and primary action. Empty More options is absent.
3. Existing image references and public-documentation validators pass.

## ASCII UI preview

The screenshot follows `UI-01` from the [plan](plan.md#ascii-ui-preview):

```text
[IMG screenshot.png x] [FILE logs.zip x]
Description...
[paperclip: Attach files]
[Todo v] [Medium v]
[Discard Draft]                       [Create Task]
```

Phone behavior follows `UI-02`, with existing touch details and image preview.
This task documents those compositions without changing the UI.

## Capture procedure

Use a temporary spec at
`apps/web/e2e/tests/office/office-new-task-doc-capture.spec.ts`.
Import the existing Office fixture and create a disposable `Customer portal` project.
Open New Task at 1280 by 900 and use the current account-recovery example.
Attach a small synthetic screenshot PNG and a harmless diagnostic text/ZIP file.
Wait for real upload readiness, then capture `office-new-issue-dialog` with
Playwright's locator screenshot API. Write to the existing repository PNG path.
Use Task 02's corrected task branch and a fresh managed build.

Inspect the image and confirm that labels, thumbnails, and actions are legible.
Remove the temporary spec in success and failure paths.
Keep the guide a focused how-to page, with no implementation details.

## Verification

Create the temporary spec before the first command. Run from the repository root:

```bash
(cd apps/web && pnpm e2e:run --host --project chromium tests/office/office-new-task-doc-capture.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/public/office-tasks.md docs/screenshots/office-new-task.png docs/plans/office-new-task-supported-controls apps/web/e2e/tests/office/office-new-task-doc-capture.spec.ts
```

Run the plan's reference preflight. The final status must show no temporary spec.
Record the image dimensions, source revision, and public validator results.

## Files likely touched

- `docs/public/office-tasks.md`
- `docs/screenshots/office-new-task.png`
- This work order and `plan.md`, for execution results.

Temporary only: `apps/web/e2e/tests/office/office-new-task-doc-capture.spec.ts`.

## Dependencies

Task 02 and its completed product checks.

## Risks

- A stale build or old screenshot can advertise the wrong attachment controls.
- A capture against a shared developer instance can expose unrelated data.

## Parallelism

`sequential`

## Inputs

- [Task requirement](../../specs/tasks/requirements/task-creation-attachments.md).
- [Task design](../../specs/tasks/system-design/task-creation-attachments.md).
- [Office requirement](../../specs/office/requirements/overview.md#req-office-task-creation-001-supported-creation-controls).
- [Office design](../../specs/office/system-design/overview-01.md#new-task-dialog).
- `/docs-maintainer`, `/simple-english`, and `/e2e`.

## Results

Complete. `docs/public/office-tasks.md` now describes file selection, image paste and drop, thumbnails, image and file previews, upload state, retry/removal, the Create guard, draft retention, discard cleanup, and the shared 10-file/100 MiB limits.

`docs/screenshots/office-new-task.png` was recaptured from disposable Office data at 1280x900 with real uploads: one image thumbnail, one file chip, the property row, and the Create Task action. The image is 896x440. The temporary capture spec was removed.

Verification passed:

- `node --test scripts/validate-public-docs.test.mjs`: 63/63.
- `node scripts/validate-public-docs.mjs`: 54 published pages.
- `git diff --check`.
- `git status` shows no temporary capture spec.
