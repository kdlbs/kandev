---
id: "01-sidebar-flag-actions"
title: "Revise sidebar flag actions"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-SIDEBAR-FLAGS-001
system_design:
  - ../../specs/tasks/system-design/sidebar-task-flags.md
acceptance_criteria:
  - AC-TASKS-SIDEBAR-FLAGS-001.1
  - AC-TASKS-SIDEBAR-FLAGS-001.2
  - AC-TASKS-SIDEBAR-FLAGS-001.3
  - AC-TASKS-SIDEBAR-FLAGS-001.4
  - AC-TASKS-SIDEBAR-FLAGS-001.5
  - AC-TASKS-SIDEBAR-FLAGS-001.6
---

# Sidebar flag actions

1. Update TaskFlagItem and TaskPriorityContextMenu. Wire direct removal to the confirmed task update, including pending and error feedback.
2. Share the TaskFlagEditor form with TaskFlagIndicator’s desktop popover. Preserve the touch drawer and existing note validation. Keep drafts while moving between the trigger and editor. Cancel must discard edits. Saving and removal must target the invoked task without navigation.
3. Update all locales, the public sidebar guide, and existing desktop and mobile flag, menu order, and priority icon tests. Verify hover-to-field travel, keyboard editing, retry after failure, direct removal, and both phone submenu actions.

## Interaction previews

Desktop, saved flag:

```text
Task [flag] -> hover or focus -> [Note field] [Cancel] [Save]
Row menu: Change flag > Edit flag
                        Remove flag
          Priority    > ...
```

Phone, saved flag:

```text
Task [flag] [...] -> Change flag > Edit flag   -> note drawer
                                   Remove flag -> remove immediately
```

An unflagged row offers Add flag. The priority icon is a numbered list.

## Verification

From `apps/web`, run `pnpm run typecheck`, `pnpm run i18n:check`, and, after rebuilding E2E assets, `pnpm e2e:raw --project=chromium --project=mobile-chrome sidebar-task-flags.spec.ts task-menu-grouping.spec.ts mobile-sidebar-task-actions.spec.ts`.

The desktop and phone flag flows and desktop menu grouping passed in Playwright. The scenarios cover hover editing, keyboard focus, draft recovery, direct removal, and phone submenu actions. Focused component tests, TypeScript, and localization checks also passed.
