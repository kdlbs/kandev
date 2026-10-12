---
id: "02-task-saved-view-surfaces"
title: "Task saved-view surfaces"
status: done
wave: 2
depends_on:
  - "01-shared-confirmation-shell"
plan: "plan.md"
requirements:
  - REQ-UI-SAVED-TASK-VIEW-DELETION-001
acceptance_criteria:
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.1
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.2
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.3
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.4
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.5
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.6
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.7
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.8
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.9
system_design:
  - ../../specs/ui/system-design/saved-task-view-deletion-confirmation.md
---

# Task 02: Task Saved-View Surfaces

## Summary

Connect the shared shell to task-sidebar and Threads saved-view deletion. Keep
their existing guards, callbacks, fallback selection, persistence, rollback,
and post-confirm parent-close behavior unchanged.

## In scope

- Capture the active sidebar view and initiating delete element before opening
  confirmation.
- Keep the fine-pointer sidebar editor popover mounted around its anchored
  confirmation and use inline confirmation in the drawer header actions.
- Carry a captured Threads target through the editor/action components.
- Use an anchored confirmation in desktop Threads and inline confirmation in
  the mobile drawer, closing settings only after explicit confirmation.
- Use the localized visible built-in name for each target.
- Add focused component tests for open, cancel, confirm, guards, target changes,
  and parent-close behavior.

## Out of scope

- Sidebar or Threads store, settings payload, persistence queue, and rollback
  changes.
- Saved-view creation, rename, duplicate, save, discard, query, sort, or column
  behavior.
- Browser-level verification owned by Task 04.

## Acceptance

- Neither task surface invokes its delete action until the named confirmation's
  destructive action is selected, and Cancel/Escape/target changes are no-ops.
- Fine-pointer confirmation preserves the editor and focus boundary; coarse
  confirmation remains inside the existing drawer with 44px actions.
- Last-view guards and each surface's existing confirmed-delete fallback and
  parent-close behavior remain intact.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- \
  components/task/sidebar-filter/view-manager.test.tsx \
  components/task/sidebar-filter/sidebar-filter-popover.test.tsx \
  components/threads/threads-view-editor-actions.test.tsx \
  components/threads/threads-view-controls.test.tsx
pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/components/task/sidebar-filter/view-manager.tsx`
- `apps/web/components/task/sidebar-filter/view-manager.test.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-filter-popover.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-filter-popover.test.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-view-editor.tsx`
- `apps/web/components/threads/threads-view-controls.tsx`
- `apps/web/components/threads/threads-view-controls.test.tsx`
- `apps/web/components/threads/threads-view-editor.tsx`
- `apps/web/components/threads/threads-view-editor-sections.tsx`
- `apps/web/components/threads/threads-view-editor-actions.tsx`
- `apps/web/components/threads/threads-view-editor-actions.test.tsx`

## Dependencies

Task 01 (`SavedTaskViewDeleteConfirmation` and shared copy/boundary contract).

## Risks

- The active view can change while confirmation is open; confirm must use the
  captured target and stale candidate state must close safely.
- Threads intentionally closes its settings surface after delete while the
  sidebar does not. Shared wiring must not erase that distinction.

## Parallelism

`parallel-safe` with Task 03 after Task 01 completes.

## Inputs

- Requirement acceptance criteria `.1` through `.9`.
- System-design sections Surface adapters, Control flow, and Responsive and
  accessibility behavior.
- Existing `SidebarFilterPopover`, `ViewHeaderRow`, `ThreadsViewControls`,
  `ThreadsViewEditor`, and optimistic saved-view actions.

## Results

- Added captured, named confirmation to task-sidebar and Threads deletion.
- Kept desktop editors mounted around anchored confirmation and used inline
  touch actions inside the existing mobile drawers.
- Preserved sidebar and Threads store actions, guards, fallback selection, and
  Threads parent-close behavior.
- Focused task-surface tests passed 18/18; web typecheck passed.
