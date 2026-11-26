---
status: done
id: "01-sidebar-flags"
title: "Add sidebar task flags"
wave: 1
depends_on: []
requirements:
  - REQ-TASKS-SIDEBAR-FLAGS-001
system_design:
  - ../../specs/tasks/system-design/sidebar-task-flags.md
plan: plan.md
acceptance_criteria:
  - AC-TASKS-SIDEBAR-FLAGS-001.1
  - AC-TASKS-SIDEBAR-FLAGS-001.2
  - AC-TASKS-SIDEBAR-FLAGS-001.3
  - AC-TASKS-SIDEBAR-FLAGS-001.4
  - AC-TASKS-SIDEBAR-FLAGS-001.5
---

# Add, read, edit, and remove a sidebar task flag

Implement [REQ-TASKS-SIDEBAR-FLAGS-001 and criteria .1 through .5](../../specs/tasks/requirements/sidebar-task-flags.md) using the [system design](../../specs/tasks/system-design/sidebar-task-flags.md). This work order covers single-task sidebar actions. Bulk actions and board indicators are outside its scope.

1. Extend task migrations, row scans, models, handlers, and the field update in the task repository. Test setting and clearing a flag through the registered update path. Verify that an unrelated concurrent task edit preserves it.
2. Carry the field through task summaries and sidebar selectors. Extend the task switcher menu, item, mobile sheet, and locale resources. Use the invoked task ID throughout. Extend the public sidebar guide with the two menu states.
3. Extend desktop and mobile Playwright coverage. Add a flag to a different task, inspect it, edit it, reload, remove it, and check the menu label. Cover cancellation and failed submission in focused editor tests. Verify keyboard access, touch hit areas, and containment with the phone keyboard open.

## ASCII UI previews

```text
UI-01  Task [flag] -> hover: saved note
       Right-click -> Flag -> [note] [Remove flag] [Cancel] [Save]
UI-02  Task [flag] [...] -> Flag -> bottom drawer
       [note]
       [Remove flag]          [Cancel] [Save]
```

Compare the rendered views with the [plan previews](plan.md). Verify an unflagged row offers Add flag.

## Verification and results

Run from the repository root after implementation:

```sh
make -C apps/backend test
make test-e2e
```

The task now stores and publishes its flag note. Desktop and phone use the same editor and task update path. Both the task and Office table-rebuild migrations preserve existing flags.

Focused verification passed for the desktop and phone Playwright flows, frontend state propagation, editor error recovery, translations, and TypeScript. The backend checks covered registered HTTP and WebSocket updates, SQLite and PostgreSQL persistence, schema replay, and late writes from an older task snapshot. The two full-suite commands above were not run.
