---
created: 2026-09-22
status: complete
requirements:
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
legacy_specs: []
---

# Implementation Plan: Preserve inherited parent workspace copy

## Overview

Correct single-task delete confirmation copy when a subtask uses
`workspaceMode: "inherit_parent"`. The existing cleanup model keys only on the
executor and therefore tells users that the child's worktree and branch will be
deleted, even though task cleanup preserves the borrowed parent environment.
The implementation updates the shared summary model, the delete dialog's
explicit and task-store-derived context, all specified task-aware callers, and
the supported locale catalogs.

The work is one sequential vertical slice. The existing backend cleanup
contract remains unchanged; this package makes the confirmation reflect it.

## Scope

### In scope

- Add the inherited-parent cleanup summary branch and localized note.
- Resolve inherited-parent state explicitly at task-aware call sites and through
  the dialog's task-id fallback.
- Preserve the existing bulk cleanup model, deletion callbacks, layout, focus,
  and consent behavior.
- Update English, Portuguese, Simplified Chinese, Traditional Chinese, and
  Japanese task catalogs.
- Add summary and rendered-dialog regression coverage.

### Out of scope

- Backend cleanup ownership, reference counting, or worktree deletion logic.
- Bulk delete copy or bulk workspace-mode inference.
- A new confirmation surface or responsive layout change.
- Public documentation.

## Technical approach

1. Extend `getCleanupSummary` with an optional `CleanupSummaryOptions` object.
   The inherited-parent branch returns the existing session-stop translation
   and a new parent-workspace note before the executor-specific branch runs.
2. Add `sharesParentWorkspace` to `TaskDeleteConfirmDialogProps`. For single
   deletes, use the prop when present and otherwise resolve the task with
   `findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks)`
   through `useAppStore`. Bulk deletes continue using grouped executor copy.
3. Pass the task's workspace mode from the five specified callers. Other
   callers remain protected by the dialog fallback when they provide a task ID.
4. Add the new key to all six task catalogs. Run the existing Traditional
   Chinese generator after updating `zh-cn` and verify catalog parity.
5. Add failing summary and dialog tests, then implement the smallest production
   change and rerun the focused tests before the full task checks.

## ASCII UI preview

### UI-01: Inherited-parent workspace delete confirmation

Entry point: delete action for a child task with a worktree executor and
`workspaceMode = inherit_parent`. Desktop and phone use the existing centered
`AlertDialog`; on phones the existing body scrolls while the footer remains
reachable. The copy hierarchy is the same at both sizes.

Current behavior:

```text
+----------------------------------------------+
| Delete task                                  |
| Delete "Child task". This action cannot be  |
| undone.                                      |
|                                              |
| Cleanup                                      |
| * The task's git worktree and its branch    |
|   will be deleted.                           |
|   Your main repo and other branches are not |
|   affected.                                  |
| * Any running agent sessions will be stopped.|
|                              Cancel  Delete  |
+----------------------------------------------+
```

Corrected behavior:

```text
+----------------------------------------------+
| Delete task                                  |
| Delete "Child task". This action cannot be  |
| undone.                                      |
|                                              |
| Cleanup                                      |
| * Any running agent sessions will be stopped.|
| This task shares its parent's workspace. The |
| parent task's worktree, branch, and files   |
| are not touched.                             |
|                              Cancel  Delete  |
+----------------------------------------------+
```

Structural requirements: keep the named delete outcome, one effects list, one
supporting note, existing focus and consent behavior, and existing action
reachability. ASCII spacing is illustrative, not a pixel contract. The
corrected view covers `AC-UI-TASK-CLEANUP-CONFIRMATION-001.15`.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-TASK-CLEANUP-CONFIRMATION-001.15` | `apps/web/components/task/task-cleanup-summary.test.ts` verifies the exact structured branch; `apps/web/components/task/task-delete-confirm-dialog.test.tsx` verifies rendered parent-worktree preservation and task-id fallback. |
| `AC-TASKS-RUNTIME-CLEANUP-001.3` | The same rendered copy regression reflects the existing shared-environment cleanup contract. |

## E2E tests

No new Playwright test is required. This is a content and state-normalization
change inside the existing confirmation surface. The mobile composition,
interaction, and geometry remain unchanged; the focused rendered component
test covers the new branch, and the existing mobile confirmation contract
continues to apply.

## Work orders

- [x] [Task 01: Correct inherited workspace delete confirmation](task-01-inherited-workspace-delete-copy.md)

## Verification results

- Focused summary, dialog, and task-menu tests passed: 56 tests across 3 files.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run build:vite`, and `git diff --check` passed.
- The scoped Traditional Chinese task catalog generator passed.
- `pnpm run i18n:check` remains blocked by 32 pre-existing missing Japanese keys
  in `executors` and `task`; the new inherited-workspace key is present in all
  six supported catalogs.
- The full Traditional Chinese generator remains blocked by two pre-existing
  simplified Chinese values in `zh-hk/workflows` and `zh-tw/workflows`.

## Risks

- A missing task in the store must keep the current executor-based fallback so
  unrelated dialogs do not lose cleanup information.
- Bulk delete must continue using executor grouping and must not infer one
  workspace mode for several tasks.
- The repository currently reports unrelated missing Japanese catalog keys in
  `pnpm run i18n:check`; the final result must distinguish that baseline issue
  from any key introduced by this change.
