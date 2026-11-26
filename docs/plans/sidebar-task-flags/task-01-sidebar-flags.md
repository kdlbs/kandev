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

The focused commands below passed.

From `apps/backend`:

```sh
go test -trimpath -race ./internal/task/handlers ./internal/backendapp -run 'TestTaskFieldUpdatesRegisteredProtocols|TestMapKanbanTaskStateIncludesTaskLabels' -count=1
go test -race ./internal/persistence/storeconformance -count=1
```

The SQLite and PostgreSQL checks covered flag persistence, upgrade replay, and writes from older task snapshots.

From `apps/web`, with freshly built E2E artifacts:

```sh
pnpm e2e:raw --project=chromium --project=mobile-chrome e2e/tests/task/sidebar-task-flags.spec.ts e2e/tests/task/mobile-sidebar-task-flags.spec.ts
pnpm run typecheck
pnpm run i18n:check
```

The local runs exercised the changed paths. The repository-wide backend and E2E suites run in the [PR checks](https://github.com/kdlbs/kandev/pull/5536/checks). Merge requires passing CI.
