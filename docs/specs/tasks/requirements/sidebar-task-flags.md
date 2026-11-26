---
status: active
system: tasks
created: 2026-11-25
owners:
  - Kandev
---

# Sidebar task flags

## Requirements

### REQ-TASKS-SIDEBAR-FLAGS-001: Annotate a sidebar task

**Intent:** A user can mark a task for attention and read the reason while browsing the sidebar.

#### Acceptance criteria

- **AC-TASKS-SIDEBAR-FLAGS-001.1:** Right-clicking a single unflagged task offers **Add flag**. Saving a nonblank note places a small red flag beside that task's title. The current task stays open. A flag note can contain up to 1,024 UTF-16 code units.
- **AC-TASKS-SIDEBAR-FLAGS-001.2:** On desktop, hovering or focusing the flag reveals its note in an editable popover. The user can enter the field and save there. The flag stays visible when the pointer leaves the row.
- **AC-TASKS-SIDEBAR-FLAGS-001.3:** A flagged task offers **Change flag** with two submenu items. **Edit flag** opens the current note. **Remove flag** removes it directly. Removal clears the icon and restores **Add flag**.
- **AC-TASKS-SIDEBAR-FLAGS-001.4:** Every sidebar occurrence shows the same saved flag after refresh. Cancel preserves the saved state. A failed write preserves the draft and offers a retry.
- **AC-TASKS-SIDEBAR-FLAGS-001.5:** A visible task actions button gives touch users the same add, read, edit, and remove actions. Keyboard users can complete the same flow.
- **AC-TASKS-SIDEBAR-FLAGS-001.6:** The Priority menu button uses a ranked-list icon. The flag glyph identifies task notes.
