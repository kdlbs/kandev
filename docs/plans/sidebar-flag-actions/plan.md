---
status: completed
requirements:
  - REQ-TASKS-SIDEBAR-FLAGS-001
system_design:
  - ../../specs/tasks/system-design/sidebar-task-flags.md
---

# Sidebar flag actions

Complete the [sidebar interaction work order](task-01-sidebar-flag-actions.md) against the [requirements](../../specs/tasks/requirements/sidebar-task-flags.md) and [design](../../specs/tasks/system-design/sidebar-task-flags.md). Reuse the existing flag update contract. Verify keyboard focus after a submenu closes and touch access to both actions.

## Interaction previews

Desktop, saved flag:

```text
Task [flag] -> hover or focus -> read-only note
Row menu: Change flag > Edit flag   -> [Note field] [Cancel] [Save]
                        Remove flag -> remove immediately
          Priority    > ...
```

Phone, saved flag:

```text
Task [flag] -> tap -> read-only note drawer
Task [...]  -> Change flag > Edit flag   -> note editor drawer
                              Remove flag -> remove immediately
```

An unflagged row offers Add flag. The priority icon is a numbered list.

## Verification results

Desktop and phone tests passed for read-only previews and menu-only editing. The [work order](task-01-sidebar-flag-actions.md#verification) lists the checks and results.
