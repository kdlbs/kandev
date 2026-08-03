---
id: "01-shared-sidebar-edit-flow"
title: "Add shared sidebar edit flow"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/sidebar-task-edit.md"
---

# Task 01: Add shared sidebar edit flow

## Acceptance

- A live single-task sidebar menu exposes localized **Edit** alongside **Rename**, while archived rows and multi-task menus do not.
- Desktop, phone, and tablet task lists open the existing `TaskCreateDialog` for the clicked task using its own workflow, step, lifecycle state, description, and primary repository without changing the active route/task.
- Phone selection closes the task drawer before the editor opens; tablet selection retains the sheet behind the editor.

## Verification

Run in Red-Green-Refactor order, observing the focused tests fail before production changes:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- components/task/task-switcher.test.tsx components/task/task-session-sidebar-edit.test.ts components/task/mobile/session-task-switcher-sheet.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-switcher.test.tsx`
- `apps/web/components/task/task-session-sidebar-edit.tsx`
- `apps/web/components/task/task-session-sidebar-edit.test.tsx`
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/task-session-sidebar-switcher-props.ts`
- `apps/web/components/task/task-session-sidebar-dialogs.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.test.tsx`
- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pseudo/common.json`

## Dependencies

None.

## Parallelism

Sequential. The menu contract, shared edit controller, and both responsive hosts share types and files and should land as one vertical slice.

## Inputs

- Spec sections: `What`, `Failure modes`, and all desktop/phone/tablet scenarios.
- Plan sections: `Shared single-task menu contract`, `Shared edit controller and dialog`, `Desktop sidebar wiring`, `Phone and tablet wiring`, and `Mobile design contract`.
- Existing patterns: `useTaskCRUD()` / `KanbanBoardDialogs` for Kanban edit props, `findTaskInSnapshots()` for sidebar target resolution, and `surfaceAction()` for phone-versus-tablet overlay behavior.

## Risks

- Do not initialize the dialog from global `kanban.workflowId`; the clicked task may come from another workflow snapshot.
- Do not duplicate `TaskCreateDialog` validation/submission or add a second cache mutation path.
- New user-facing copy must use i18n at render time.

## Output contract

Report the red test evidence, final files changed, exact commands and outcomes, blockers, responsive behavior, and remaining risks. Reconcile this file's likely-file list with the actual diff, set this task to `done`, and synchronize its checkbox plus verification results in `plan.md`.

## Results

- RED evidence: before the production menu/controller wiring, the focused switcher run reported 2 expected missing-Edit failures and 16 passing tests.
- GREEN evidence: `cd apps && pnpm --filter @kandev/web test -- components/task/task-switcher.test.tsx components/task/task-session-sidebar-edit.test.ts components/task/mobile/session-task-switcher-sheet.test.tsx` passed all 24 tests, including the unresolved-source, missing-workflow, and selection-clear guards added during PR review.
- `cd apps/web && pnpm run typecheck` passed. The i18n checks passed from `apps/web` (`i18n:check` confirmed 1464 referenced keys and pseudo locale sync; `i18n:ratchet` reported zero added or modified violations). The original `cd apps && pnpm run i18n:check` path was corrected because the scripts are owned by `@kandev/web`.
- Targeted ESLint across the changed task/sidebar files passed with no warnings, and `git diff --check` passed.
- The actual implementation spans the shared switcher/context menu, shared edit controller/dialog, desktop sidebar hosts, mobile/tablet sheet host, locale files, and focused unit tests. Phone selection closes the drawer before opening the editor; tablet keeps the sheet mounted behind it. No duplicate task update/cache path was added.
