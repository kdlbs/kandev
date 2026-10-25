---
id: "02-add-edit-dialog-dependencies"
title: "Add dependencies to Edit task"
status: done
wave: 2
depends_on:
  - "01-enable-detail-editing"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001
acceptance_criteria:
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.1
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.2
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.3
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.4
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.5
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.7
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.8
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.9
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.10
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.11
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.12
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.13
  - AC-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001.14
system_design:
  - ../../specs/tasks/system-design/task-dependency-detail-editing.md
---

# Task 02: Add dependencies to Edit task

## Summary

Add a searchable dependency field to the shared Edit task dialog. Stage changes
until Update and submit the complete predecessor draft through the replacement
route from Task 01.

## In scope

- Load the edited task's confirmed dependency projection.
- Search non-archived tasks in the edited task's workspace.
- Show current predecessors as selected and exclude the edited task.
- Keep picker changes in dialog form state until Update.
- Keep dependencies unchanged on Cancel.
- Keep the dialog open and preserve the draft after cycle or request errors.
- Keep the field available when the edited task has started.
- Keep the task-detail dependency chip read-only.
- Add localized copy in all required catalogs.
- Add API client, component, submit, desktop E2E, and mobile E2E coverage.

## Out of scope

- Changes to the Office task-properties picker.
- Inline dependency mutation from the task-detail chip.
- Multi-task dependency editing.
- Changes to `start_when_unblocked`.

## Acceptance

- Every existing Edit task entry point opens the same dependency field.
- Update submits the desired predecessor set and Cancel sends no replacement.
- Cycle and request errors preserve confirmed dependencies and permit a retry.
- Desktop, phone, and tablet paths provide the same user outcome.

## TDD sequence

1. Add failing API client and dialog state tests.
2. Add failing submit tests for update, cancel, and error retention.
3. Add failing desktop and mobile Playwright scenarios.
4. Implement the smallest shared picker, state, and submit changes.
5. Run the focused tests and refactor with all tests green.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- components/task-create-dialog.test.tsx components/task-create-dialog-submit.test.tsx components/task-edit-dialog-dependencies.test.tsx lib/api/domains/task-dependencies-api.test.ts hooks/domains/task/use-task-edit-dialog-dependencies.test.ts components/task-create-dialog-footer.test.ts
pnpm --filter @kandev/web run typecheck
pnpm run lint
cd web
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run --host --no-build --project chromium tests/task/create-task-dependency-selector.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-create-task-dependency-selector.spec.ts
cd ../..
git diff --check
```

## Files likely touched

- `apps/web/lib/api/domains/task-dependencies-api.ts`
- `apps/web/lib/api/domains/task-dependencies-api.test.ts`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-setup.ts`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task-create-dialog-submit.test.tsx`
- `apps/web/components/task-create-dialog-dependencies.tsx`
- `apps/web/components/task-edit-dialog-dependencies.tsx`
- `apps/web/components/task-edit-dialog-dependencies.test.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/zh-hk/task.json`
- `apps/web/src/locales/zh-tw/task.json`
- `apps/web/src/locales/pseudo/task.json`
- `apps/web/e2e/tests/task/task-dependencies.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`

## Dependencies

Task 01 supplies the atomic replacement route.

## Risks

- Task-field and dependency updates use separate HTTP requests. A dependency
  error must reload confirmed task fields while it keeps the dialog open.
- Candidate search responses can arrive out of order.
- A nested popover must remain inside the full-screen phone dialog.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/task-dependency-detail-editing.md`
- `docs/specs/tasks/system-design/task-dependency-detail-editing.md`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog-dependencies.tsx`
- `apps/web/components/task/task-session-sidebar-edit.tsx`
- `apps/web/lib/api/domains/task-dependencies-api.ts`

## Results

Implemented the shared Edit task dependency field and the typed replacement
flow. The field loads confirmed predecessors, searches non-archived workspace
tasks, retains draft selection through Cancel or request errors, and appears in
the existing desktop and mobile Edit entry points. Cycle errors keep the dialog
open with the draft available for correction. The task-detail dependency chip
remains read-only.

Validation passed:

- 68 focused web tests passed across 6 files.
- TypeScript typecheck, full web lint, i18n completeness, and i18n ratchet passed.
- The desktop managed E2E suite passed with 2 tests.
- The mobile managed E2E suite passed with 2 tests.
- Capture-enabled desktop and mobile E2E runs produced inspected, compressed
  PR assets with a complete manifest.
- `git diff --check` passed.
