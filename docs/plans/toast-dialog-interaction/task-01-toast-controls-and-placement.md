---
id: "01-toast-controls-and-placement"
title: "Dismiss toasts and keep modal actions clear"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TOAST-INTERACTION-001
acceptance_criteria:
  - AC-UI-TOAST-INTERACTION-001.1
  - AC-UI-TOAST-INTERACTION-001.2
  - AC-UI-TOAST-INTERACTION-001.3
  - AC-UI-TOAST-INTERACTION-001.4
  - AC-UI-TOAST-INTERACTION-001.5
system_design:
  - ../../specs/ui/system-design/toast-interaction.md
---

# Task 01 · Dismiss toasts and keep modal actions clear

## Summary

Expose a localized Dismiss button on each shared toast. Render toasts inside
the active modal, in a row that keeps the modal's actions visible.

## In scope

- Use the shared icon `Button` sizing and a localized accessible name such
  as “Dismiss notification”. Add the key to each supported `common.json`.
- Reuse one optional outlet in Dialog, AlertDialog, Sheet, and Drawer. Use
  the primitive's mount and unmount lifecycle to select the latest outlet.
  Place the image review outlet after `ReviewHeader`.
- Call `dismissToast(id)` from the button. Keep state in the provider.
- Test with the two `system.update_available` events and the shared controls.

## Acceptance

1. Each toast can be dismissed independently. Keyboard activation, the phone
   hit area, both action types, and the existing expiry behavior work.
2. The active modal owns the controls for the visible toasts. Dismissal leaves
   the dialog usable. A nested modal and its parent each expose one instance
   while active. The viewport resumes ownership after the last modal closes.
3. With two update notices present, the question review actions fit and work
   on desktop and phone. The phone form is visible and the bottom bar stays
   in the viewport. The toast region scrolls when it exceeds its height limit.
4. Enter on Dismiss or an action shall preserve native activation without
   reaching dialog-level selection handlers. The modal shall expose one
   polite live region before the first toast and retain that node as its text
   changes.
5. At a 667 by 390 phone viewport with two notices, the complete review
   bottom bar and its touch targets shall remain within the viewport.
6. `AlertDialogContent asChild` shall support one slotted child with the toast
   outlet nested inside it.

## ASCII UI preview

See the [full previews in the plan](plan.md#ascii-ui-preview). UI-01 applies to
the viewport stack. In UI-02 and UI-03, the notification row is inside the
active modal. The phone layout uses a bounded single column.

```text
UI-01  Workbench     Runtime update [Review] [X]

UI-02  Desktop       Review images       [Submit] [Close]
                                 Runtime update     [X]
                     Image stage and answer form

UI-03  Phone         Review images       [Submit] [Close]
                     Question tabs
                     Runtime update [Review runtime] [X]
                     Image stage and scrolling answer form
                     [Flip] [Next] [Image pair]
```

UI-01 covers `AC-UI-TOAST-INTERACTION-001.1`, `AC-UI-TOAST-INTERACTION-001.2`,
and `AC-UI-TOAST-INTERACTION-001.5`. UI-02 and UI-03 cover
`AC-UI-TOAST-INTERACTION-001.3` and `AC-UI-TOAST-INTERACTION-001.4`.

## Verification

First add the desktop regression **update toasts keep review actions clear**.
At 1600 by 1440 CSS pixels, wait for both update toasts and their entry motion.
Compare their rendered bounds with Submit and Close before querying Dismiss.
Confirm the overlap assertion fails on the original code.

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium tests/chat/question-image-review.spec.ts -- --grep 'update toasts keep review actions clear'
```

Verify these paths after the fix.

- In `toast-provider.test.tsx`, dismiss a loading toast while an actionable
  toast remains. Activate the other toast's action.
- In the provider test, transfer a timed toast into a modal, then into its
  nested modal and back. Check for a single copy at each step. Advance the
  clock to its original expiry after closing the modals.
- Keep an empty polite live region inside an open modal before creating the
  first toast. Check that the same node announces the toast text and remains
  outside hidden accessibility subtrees.
- Queue multiple toast messages, including identical text. Check they are
  announced in order and that repeated text creates a separate text mutation.
- Render `AlertDialogContent asChild` around one section and check that a
  toast appears within that section without a Slot error.
- In Chromium, keep the runtime link present while dismissing the release
  toast with Tab and Enter. Check that the dialog stays open. Use Close and
  check focus returns to **Review images**.
- Open the Recent Task Switcher from the command palette, focus Dismiss in a
  visible toast, and press Enter. Confirm the toast dismisses while the
  switcher stays open and the selected task URL does not change.
- In `mobile-chrome`, measure both dimensions and viewport containment of
  Dismiss using `boundingBox()`. Open and close the review's nested More
  drawer with a live toast. Tap Dismiss and then Submit while an update
  toast is still present. Check answer delivery and no horizontal overflow.
  At 667 by 390, assert the full bottom bar and all its hit targets fit.
  Measure the form and toast region while both notices are present. Use a
  controlled clock if the assertions require more than the default duration.
  Capture desktop and phone screenshots with two toasts.

```bash
pnpm --dir apps/web exec vitest run components/toast-provider.test.tsx
pnpm --dir apps/web run test --shard=1/2 --reporter=dot
pnpm --dir apps/web exec vitest run components/github/github-connection-dialog.test.tsx components/task/task-session-sidebar-rename-ownership.test.tsx components/task/mobile/use-mobile-task-rename-ownership.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium tests/chat/question-image-review.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium tests/task/recent-task-switcher.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/toast-provider.tsx`
- `apps/web/components/toast-provider.test.tsx`
- `apps/packages/ui/src/modal-toast-outlet.tsx` (new)
- `apps/packages/ui/src/dialog.tsx`
- `apps/packages/ui/src/alert-dialog.tsx`
- `apps/packages/ui/src/sheet.tsx`
- `apps/packages/ui/src/drawer.tsx`
- `apps/web/components/task/chat/question-review/question-review-dialog.tsx`
- `apps/web/src/locales/*/common.json`
- `apps/web/e2e/tests/chat/question-image-review.spec.ts`
- `apps/web/e2e/tests/chat/mobile-question-image-review.spec.ts`
- `apps/web/e2e/tests/chat/question-image-review-helpers.ts`

## Dependencies

Use Playwright Chromium and the existing image review fixtures. In a fresh
worktree, install dependencies with `pnpm --dir apps install --frozen-lockfile`.

## Risks

Radix uses the React event tree to classify inside interactions. Keep the
rendered controls in the primitive's React tree. Check that a button press
activates only that button while the dialog retains its focus trap.

## Parallelism

Sequential.

## Inputs

- [Toast interaction requirements](../../specs/ui/requirements/toast-interaction.md)
- [Toast interaction system design](../../specs/ui/system-design/toast-interaction.md)
- `openReviewQuestions`, `reviewScript`, and `seedReviewImages` from the
  image review fixture.
- `installRuntimeUpdateFixture` from
  `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`.

## Results

Added localized, independently dismissible controls and routed the shared toast
stack into the latest mounted modal outlet. Toast state, callbacks, and timers
remain in `ToastProvider`. The question image review outlet sits below its
header; the phone stage can shrink while the answer form and bottom bar remain
available. Added provider coverage and desktop/mobile browser regressions.

Addressed four static review findings and queued toast announcement feedback.
Each toast announcement is delivered in order, including when consecutive
notices have identical text. Enter on toast controls no longer reaches
the Recent Task Switcher's task selection handler, while native activation is
preserved. One polite live region exists before the first toast in the active
accessibility scope. The phone layout yields height at 667 by 390, and
`AlertDialogContent asChild` keeps its outlet inside the single slotted child.
Existing modal tests now inspect toast messages in the active outlet and count
the drawer's content scroller separately from the toast scroller.

Verification passed:

- `pnpm --dir apps/web exec vitest run components/toast-provider.test.tsx` — 11 passed.
- `pnpm --dir apps/web run test --shard=1/2 --reporter=dot` — 1,754 files and 15,610 tests passed.
- The grouped modal-outlet regression tests — 35 passed.
- `pnpm --dir apps/web run typecheck`
- `pnpm --dir apps/web run lint`
- `pnpm --dir apps/web run i18n:check`
- `pnpm --dir apps/web run build:vite`
- `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium tests/chat/question-image-review.spec.ts` — 10 passed.
- `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts` — 5 passed.
- `scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium tests/task/recent-task-switcher.spec.ts` — 4 passed.
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
