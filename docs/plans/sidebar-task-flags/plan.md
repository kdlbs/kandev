---
status: completed
requirements:
  - REQ-TASKS-SIDEBAR-FLAGS-001
system_design:
  - ../../specs/tasks/system-design/sidebar-task-flags.md
---

# Sidebar task flags

Deliver the [requirements](../../specs/tasks/requirements/sidebar-task-flags.md) through the [task design](../../specs/tasks/system-design/sidebar-task-flags.md).

1. Complete the [flag work order](task-01-sidebar-flags.md). Verify partial updates first, then connect the sidebar and test the full interaction.
2. Check a flagged task that appears twice and a target outside the active workflow. Stale summaries and row memoization can otherwise hide successful edits.

## ASCII UI previews

UI-01, desktop sidebar. Right-click opens the menu. The note wraps on hover or focus.

```text
Update release notes  [flag]     Flag
                      |          [ Check links before release ]
                      |          [Remove flag] [Cancel] [Save]
                      +-- Check links before release
```

UI-02, phone task switcher. Use the visible actions button or tap the flag. The drawer body scrolls when the keyboard reduces available space.

```text
Update release notes  [flag] [...]
                v
+--------------------------------+
| Flag                           |
| [ Check links before release ] |
| Remove flag                    |
|              [Cancel]   [Save] |
+--------------------------------+
```

Both views show the saved state. For an unflagged task the menu reads Add flag and the editor starts empty. Spacing and sample text are illustrative.
