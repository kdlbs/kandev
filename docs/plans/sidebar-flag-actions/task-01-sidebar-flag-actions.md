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

1. Keep direct removal in Change flag and preserve pending and error feedback from the confirmed task update. Keep the ranked-list priority icon.
2. Show the saved note in read-only previews on hover or focus on desktop and on tap on phones. Launch the editable field from Add flag or Change flag → Edit flag. Keep note validation, cancel and retry behavior, and focus return. Saving and removal target the invoked task while the current task stays open.
3. Update the public guide and existing desktop and mobile flag tests. Verify that the previews expose read-only text, keyboard users can reach the editor from the menu, and both phone submenu actions still work. Cover retry after failure and direct removal.

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

## Verification

Build the E2E fixtures with `make build-backend build-e2e-plugin-package build-e2e-delivery-fixture build-e2e-send-now-fixture` and build the frontend with `make build-web-e2e` from the repository root.

Run the following checks from `apps/web`. Use `KANDEV_SERVER_HOST=127.0.0.1` and the host browser libraries for Playwright.

- `pnpm e2e:raw --project=chromium --project=mobile-chrome e2e/tests/task/sidebar-task-flags.spec.ts e2e/tests/task/mobile-sidebar-task-flags.spec.ts`
- `pnpm exec vitest run components/task/task-flag-indicator.test.tsx components/task/task-switcher-{context-menu,rename-handoff}.test.tsx`
- `pnpm run typecheck` and `pnpm run i18n:check`

From the repository root, run `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

Both desktop and phone flag scenarios passed after rebuilding the frontend. All 29 component tests in the three suites above passed. TypeScript, localization, catalog validation, specification lint, and the diff check passed.
