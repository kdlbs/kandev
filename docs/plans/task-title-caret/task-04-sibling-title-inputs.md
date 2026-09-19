---
id: "04-sibling-title-inputs"
title: "Extend the hook to sibling task-title inputs"
status: done
wave: 4
depends_on: ["03-rename-edit-dialogs"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/title-length-limit.md"
---

# Task 04: Extend the hook to sibling task-title inputs

## Acceptance

- Every remaining task-title input that clamps inside `onChange` now uses
  `useTaskTitleSelectionRestore` and keeps the caret at the 60-character cap:
  - `apps/web/components/task/task-top-bar-title.tsx` (inline breadcrumb title
    editor, `task-title-rename-input`);
  - `apps/web/components/task/new-subtask-form-parts.tsx` (`subtask-title-input`);
  - `apps/web/app/office/components/new-task-dialog.tsx` (title textarea —
    the hook must be wired to the `Textarea` ref, which supports the same
    selection API);
  - `apps/web/app/office/setup/step-task.tsx` (onboarding title input);
  - `apps/web/components/automations/automation-editor-sections.tsx`
    (`taskTitleTemplate` input).
- Behavior is otherwise unchanged: no `maxLength` attributes added, no new
  user-facing copy, clamp semantics unchanged.
- The full verification gate passes: `make fmt`, then `make typecheck test
  lint`.
- The amended spec `docs/specs/tasks/requirements/title-length-limit.md` status flips back
  to `complete` and the plan/task statuses are updated.

## Verification

- `make fmt`
- `make typecheck`
- `make test` (web suite, including `task-title.test.ts` and the new hook
  test)
- `make lint`
- `cd apps/web && pnpm e2e:raw tests/kanban/task-title-caret.spec.ts`
- `cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/task/mobile-task-title-caret.spec.ts`
- `cd apps/web && pnpm e2e:raw tests/github/pr-action-create-task-dialog.spec.ts`
  (guards the `maxlength`-absence contract)

## Files likely touched

- `apps/web/components/task/task-top-bar-title.tsx`
- `apps/web/components/task/new-subtask-form-parts.tsx`
- `apps/web/app/office/components/new-task-dialog.tsx`
- `apps/web/app/office/setup/step-task.tsx`
- `apps/web/components/automations/automation-editor-sections.tsx`
- `docs/specs/tasks/requirements/title-length-limit.md` (status back to `complete`)

## Dependencies

- Waves 1–3 must be complete.

## Parallelism

Sequential. These files share the hook contract and are verified together.

## Inputs

- The hook API from Task 02.
- `new-subtask-form-parts.tsx` and the Office files may control their state
  locally; wire the same `ref`/`clampChange` swap as Task 03. If a textarea
  does not forward a ref the same way the `Input` does, adjust the ref wiring
  only, never the hook behavior.
- The Office surfaces run under the office routes in the SPA; the desktop
  `chromium` E2E suite covers the reported dialogs, and the office inputs are
  covered by typecheck/lint plus the shared unit test for the hook.

## Output contract

Report the changed files, the exact verification commands and results
(`make fmt typecheck test lint` plus the targeted E2E runs), and the updated
task/plan/spec statuses in the same conversation.
