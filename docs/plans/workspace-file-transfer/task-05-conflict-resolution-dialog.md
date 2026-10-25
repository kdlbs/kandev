---
id: "05-conflict-resolution-dialog"
title: "Upload conflict resolution dialog"
status: done
wave: 4
depends_on:
  - "04-web-upload-client"
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.3
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.5
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 05: Upload conflict resolution dialog

## Summary

The decision surface. When the preflight from task 04 reports conflicts, this shows the person
exactly what already exists and collects a choice per file before anything is written.

## Scope

- `apps/web/components/task/file-upload-conflict-dialog.tsx` listing every conflicting destination
  path with **Replace**, **Keep both**, and **Skip** per file.
- An apply-to-all control that sets one resolution across every remaining conflict.
- Confirm returns the resolution map to the hook; cancel reports cancellation, which uploads nothing.
- The conflict list scrolls within the dialog rather than growing it, since a folder upload can
  produce many conflicts.
- Copy in all five locales, with conflict counts using `count` and `_one` / `_other` rather than a
  concatenated plural.

## Exclusions

- The preflight call and the upload loop. Task 04 owns those; this binds to the state it exposes.
- Entry points and drag and drop.

## Acceptance

- Every conflict reported by the preflight is listed, and a resolution can be set per file or applied
  to all remaining at once.
- Confirming returns exactly the chosen resolutions, with **Skip** removing that file from the batch.
- Cancelling results in no upload request being sent for any file in the selection.

## Verification

Write the dialog assertions first and confirm they fail before the production change. Then:

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/file-upload-conflict-dialog.test.tsx hooks/use-file-upload.test.ts
cd apps/web && node ../node_modules/typescript/bin/tsc --noEmit && pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/task/file-upload-conflict-dialog.tsx` and its test
- `apps/web/hooks/use-file-upload.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`

## Dependencies

Task 04.

## Parallelism

Sequential.

## Inputs

- Requirements: `REQ-UI-WORKSPACE-FILE-TRANSFER-004`.
- System design: `Components and responsibilities > Frontend`, `Control flow`.
- Plan: `Frontend > Conflict resolution dialog` and the mobile design contract.
- Existing patterns: `file-delete-confirmation.tsx` as the nearest destructive-choice dialog in this
  panel.

## Risks

- **Replace** is destructive and irreversible; there is no version history behind it. It must not be
  the pre-selected default, and the copy must make the consequence plain.
- A long conflict list must scroll inside the dialog, or the confirm control leaves the viewport on a
  phone.
- Do not translate any value compared with `===`; the resolution tokens are persisted decisions, not
  copy.

## Output contract

Report the dialog's resolution contract, files changed, exact commands and results, then mark this
task `done` and update its checkbox in `plan.md`.

## Results

- `components/task/file-upload-conflict-dialog.tsx` lists every conflicting destination with a
  per-file **Keep both / Replace / Skip** group and an apply-to-all row. The list scrolls inside the
  dialog (`max-h-64 overflow-y-auto`) so a folder upload with many conflicts keeps the footer
  reachable.
- **Keep both is the default.** Replace is irreversible with no version history behind it, so it is
  never pre-selected.
- Eight `task:uploadConflict*` keys in all five locales plus pseudo, with the title using
  `_one`/`_other` on `count`. The three resolution tokens are `// i18n-exempt` wire values.

**Design change during implementation:** the first cut used a Radix `Select` per row. Radix Select
is portal-based and effectively undrivable in jsdom without `@testing-library/user-event`, which is
not a dependency here. Replaced with a segmented `Button` group carrying `aria-pressed`, which is
simpler, keeps all three options visible at a glance for a three-way choice, and is directly
testable. A test asserts the `aria-pressed` state so the active choice stays exposed to assistive
technology.

### Commands

```
pnpm --filter @kandev/web test -- components/task/file-upload-conflict-dialog.test.tsx   7 passed
node ../node_modules/typescript/bin/tsc --noEmit                                          clean
pnpm run i18n:check                                                                       6/6 gates
pnpm run lint                                                                             0 problems
```
