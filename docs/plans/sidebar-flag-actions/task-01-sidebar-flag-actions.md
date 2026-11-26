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

From the repository root, build the E2E fixtures with `make build-backend build-e2e-plugin-package build-e2e-delivery-fixture build-e2e-send-now-fixture` and build the frontend with `make build-web-e2e`. Both builds passed.

From `apps/web`, set `KANDEV_SERVER_HOST=127.0.0.1` for Playwright and supply browser libraries on the host library path.

- `pnpm e2e:raw --project=chromium --project=mobile-chrome e2e/tests/task/sidebar-task-flags.spec.ts e2e/tests/task/mobile-sidebar-task-flags.spec.ts e2e/tests/task/task-menu-grouping.spec.ts` passed all three tests.
- `pnpm e2e:raw --project=chromium e2e/tests/task/sidebar-layout.spec.ts --grep 'shows priority after the title'` passed the desktop priority test.
- `pnpm e2e:raw --project=mobile-chrome e2e/tests/task/mobile-sidebar-task-actions.spec.ts` passed all 17 phone sidebar tests.
- `pnpm exec vitest run components/task/task-flag-indicator.test.tsx components/task/task-switcher-{context-menu,rename-handoff}.test.tsx` passed all 29 cases across the three suites, including keyboard dismissal and draft retention after a breakpoint change.
- `pnpm run typecheck` and `pnpm run i18n:check` passed.

From the repository root, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
