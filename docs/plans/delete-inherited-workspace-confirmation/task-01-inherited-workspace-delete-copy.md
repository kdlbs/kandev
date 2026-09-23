---
id: "01-inherited-workspace-delete-copy"
title: "Correct inherited workspace delete confirmation"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.15
  - AC-TASKS-RUNTIME-CLEANUP-001.3
system_design:
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 01: Correct inherited workspace delete confirmation

## Summary

Make single-task delete confirmation accurately describe a child task that
shares its parent's materialized workspace. The summary will show only the
agent-session stop effect plus the parent-workspace preservation note, with
explicit caller context and a task-store fallback for callers that provide only
the task ID.

## In scope

- Extend the single-task cleanup summary options and inherited-parent branch.
- Add the dialog prop, store lookup fallback, and five specified call-site
  values.
- Add the English and five translated catalog values.
- Add summary and dialog regressions, including explicit prop and fallback
  behavior.

## Out of scope

- Backend task cleanup or worktree lifecycle behavior.
- Bulk cleanup summary changes.
- Confirmation layout, touch sizing, focus, or callback changes.
- New Playwright coverage for an unchanged mobile composition.

## Acceptance

- `getCleanupSummary(_, { sharesParentWorkspace: true })` returns exactly one
  agent-session effect and exactly one inherited-parent workspace note, with no
  worktree or branch deletion copy.
- A rendered delete dialog shows the preserved parent worktree, branch, and
  files when the explicit prop is true or when the task ID resolves to
  `inherit_parent` in the app store.
- Existing executor-specific single and bulk copy, deletion callbacks, and
  consent behavior remain unchanged for all other tasks.

## ASCII UI preview

See [UI-01 in the plan](plan.md#ui-01-inherited-parent-workspace-delete-confirmation).
The dialog keeps the existing desktop and phone composition; only the cleanup
effects and supporting note change for inherited-parent tasks.

```text
Cleanup
* Any running agent sessions will be stopped.
This task shares its parent's workspace. The parent task's worktree,
branch, and files are not touched.

                         Cancel  Delete
```

## Verification

```bash
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm test components/task/task-cleanup-summary.test.ts components/task/task-delete-confirm-dialog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run lint)
```

## Files likely touched

- `apps/web/components/task/task-cleanup-summary.ts`
- `apps/web/components/task/task-cleanup-summary.test.ts`
- `apps/web/components/task/task-delete-confirm-dialog.tsx`
- `apps/web/components/task/task-delete-confirm-dialog.test.tsx`
- `apps/web/components/task/task-actions-menu-dialogs.tsx`
- `apps/web/components/kanban-card-menu.tsx`
- `apps/web/components/task/task-management-surface.tsx`
- `apps/web/app/tasks/columns.tsx`
- `apps/web/app/tasks/tasks-list-view.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/task.json`

## Dependencies

None.

## Risks

- Callers with stale or missing task projections must retain the generic or
  executor-based summary through the dialog fallback.
- Traditional Chinese catalogs are generator-managed and must be updated with
  `pnpm run i18n:zh-hant`.
- The repository's existing Japanese catalog gaps may keep the full i18n check
  red after this task; report the exact pre-existing failures.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/confirmation-warning-hierarchy.md`, especially
  `AC-UI-TASK-CLEANUP-CONFIRMATION-001.15`.
- `docs/specs/ui/system-design/confirmation-warning-hierarchy.md`.
- `docs/specs/tasks/requirements/runtime-cleanup.md`,
  `AC-TASKS-RUNTIME-CLEANUP-001.3`.
- `docs/specs/tasks/system-design/runtime-cleanup.md`.
- `apps/web/AGENTS.md`.
- `.agents/skills/tdd/SKILL.md` and `.agents/skills/mobile-parity/SKILL.md`.

## Results

Implemented the inherited-parent cleanup branch, explicit task-aware caller
context, task-store fallback, supported locale values, and regression coverage.
The focused test files pass with 56 tests. Typecheck, lint, Vite build, and
diff validation pass. The scoped Traditional Chinese task catalog generation
passes. The full i18n check still reports the 32 pre-existing Japanese missing
keys, and the full Traditional Chinese generator still reports two pre-existing
simplified Chinese values in `zh-hk/workflows` and `zh-tw/workflows`.
