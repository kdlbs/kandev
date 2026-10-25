---
id: "01-host-submenu-contract"
title: "Host: task menu action submenu contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-001
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-002
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-003
acceptance_criteria:
  - AC-PLUGINS-TASK-MENU-SUBMENUS-001.1
  - AC-PLUGINS-TASK-MENU-SUBMENUS-001.2
  - AC-PLUGINS-TASK-MENU-SUBMENUS-001.3
  - AC-PLUGINS-TASK-MENU-SUBMENUS-001.4
  - AC-PLUGINS-TASK-MENU-SUBMENUS-001.5
  - AC-PLUGINS-TASK-MENU-SUBMENUS-001.6
  - AC-PLUGINS-TASK-MENU-SUBMENUS-002.1
  - AC-PLUGINS-TASK-MENU-SUBMENUS-002.2
  - AC-PLUGINS-TASK-MENU-SUBMENUS-002.3
  - AC-PLUGINS-TASK-MENU-SUBMENUS-002.4
  - AC-PLUGINS-TASK-MENU-SUBMENUS-002.5
  - AC-PLUGINS-TASK-MENU-SUBMENUS-003.1
  - AC-PLUGINS-TASK-MENU-SUBMENUS-003.2
  - AC-PLUGINS-TASK-MENU-SUBMENUS-003.3
system_design:
  - ../../specs/plugins/system-design/plugin-task-menu-submenus.md
---

# Task 01: Host task menu action submenu contract

- **Acceptance:**
  1. `TaskMenuActionRegistration.items?(context)` is optional and synchronous in
     the SDK and the web re-export; `run` stays required; a host that predates the
     field renders the flat item (AC-PLUGINS-TASK-MENU-SUBMENUS-001.1).
  2. A usable `items()` renders the action as a one-level submenu: `label` is the
     unselectable trigger, children render in order under the same context, the
     entry's own `disabled` reaches every child, and a child's own `disabled`
     holds (AC-PLUGINS-TASK-MENU-SUBMENUS-001.2).
  3. Every unusable result — absent, `null`, non-array, empty, a throw, a
     thenable, or only-unusable children — falls back to the flat `run` item, with
     the defect reported once per action and kind
     (AC-PLUGINS-TASK-MENU-SUBMENUS-001.3,
     AC-PLUGINS-TASK-MENU-SUBMENUS-001.4).
  4. A child is read once inside the boundary's guard into a snapshot: blank or
     non-string `id`/`label`, a non-callable `run`, a `disabled` that is neither
     boolean nor `null`, and an `icon` that is none of name/component/element/
     `null` all drop that child; a repeated `id` keeps the first occurrence; a
     throwing getter or a Proxy fails only that child
     (AC-PLUGINS-TASK-MENU-SUBMENUS-001.5).
  5. No malformed registration can fail a render: an unusable action `label` or
     `id` contributes no entry, the defect report never throws on a `Symbol`,
     null-prototype or throwing-accessor registration, a `primary`-group submenu
     whose entries are all unusable leaves the native `Edit` item flat, and the
     key encoder neither throws on a lone surrogate nor collides for any two
     plugin-authored ids (AC-PLUGINS-TASK-MENU-SUBMENUS-002.1 .. 002.4).
  6. Icon resolution admits `memo`/`forwardRef` component objects, excludes
     `lazy` and elements for non-menu surfaces, resolves own keys only, and keeps
     the menu-only element tolerance documented
     (AC-PLUGINS-TASK-MENU-SUBMENUS-002.5).
  7. Submenu children of a `primary` action reach the command palette and the
     sidebar's task commands, one command each, carrying the trigger label as
     `context`; `edit` stays card-only; a caller that forces the flat `Edit` form
     never reuses a prebuilt entry set
     (AC-PLUGINS-TASK-MENU-SUBMENUS-003.1 .. 003.3).
- **Verification:**
  - `cd apps/web && ./node_modules/.bin/tsc --noEmit --incremental false`
  - `cd apps/web && pnpm vitest run components/plugins/task-menu-actions.test.tsx components/kanban-card-edit-submenu.test.tsx components/kanban-card-menu-items.test.tsx components/kanban-card-menu-grouping.test.tsx components/kanban-card-plugin-context.test.tsx lib/kanban/task-actions-menu-entries.test.ts lib/plugins/sdk-contract.test.ts lib/plugins/icons.test.ts components/task/task-switcher-context-menu.test.tsx components/task-command-choices.test.tsx components/task-commands.test.tsx`
  - `cd apps/web && ./node_modules/.bin/eslint --max-warnings 0 <changed files>`
  - `cd apps/web && pnpm i18n:check`
  - `python3 scripts/list-docs.py validate`
  - `node .github/scripts/pr-docs.cjs` (CI's documentation-coverage evaluator)
- **Files likely touched:**
  - `apps/packages/plugin-sdk/src/index.ts`
  - `apps/web/lib/plugins/types.ts`
  - `apps/web/lib/plugins/icons.ts` (+ test)
  - `apps/web/components/plugins/task-menu-actions.ts` (+ test)
  - `apps/web/components/kanban-card-edit-submenu.tsx` (+ test)
  - `apps/web/components/kanban-card-menu-items.tsx` (+ test)
  - `apps/web/components/kanban-card-menu.tsx`
  - `apps/web/components/task-command-choices.tsx` (+ test)
  - `apps/web/components/task-command-items.tsx` (+ test)
  - `apps/web/lib/kanban/task-actions-menu-entries.ts` (+ test)
  - `apps/web/lib/plugins/sdk-contract.test.ts`
  - `apps/web/AGENTS.md`, `docs/plans/plugins/PLUGIN-API.md`, `docs/public/plugins-authoring.md`
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:**
  - Requirements: docs/specs/plugins/requirements/plugin-task-menu-submenus.md
  - System design: docs/specs/plugins/system-design/plugin-task-menu-submenus.md
  - Existing submenu entry model and renderers: `apps/web/components/kanban-card-menu-items.tsx`
  - Plugin contract documentation: `docs/plans/plugins/PLUGIN-API.md`, `docs/public/plugins-authoring.md`
  - Consumer expectations: the plugin repository's `ui/bundle.js` registration and its tests
- **Output contract:** summary, files changed, exact verification commands with
  results, the adversarial review rounds with their dispositions, task status →
  `done`, plan checkbox update.
