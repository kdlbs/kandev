---
requirements:
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-001
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-002
  - REQ-PLUGINS-TASK-MENU-SUBMENUS-003
system_design:
  - ../../specs/plugins/system-design/plugin-task-menu-submenus.md
created: 2026-09-23
status: draft
---

# Implementation Plan: Plugin task menu submenus

## Overview

`registerTaskMenuAction` contributes exactly one selectable item per plugin
action, so a plugin whose natural contribution is a short list of frequent
choices (the Tags plugin's recently used tags) has nowhere to put them. This plan
lands the host half of a one-level submenu contract: an optional
`items(context)` beside the required `run`, a single defensive boundary that
reads plugin values, an entry key that is injective for any plugin-authored
string, and flattening that makes `primary` children reachable from the command
palette and the sidebar's task commands.

The plugin-facing half lives in its own repository and consumes this contract; it
is not part of this plan. Rendering needs no new component, because the menu entry
model already supports submenus and both renderers recurse.

Implementation is entirely in `apps/web` plus the plugin SDK type and two
documents. There is no backend, state, persistence or protocol change: a host that
predates `items` ignores the field, and `run` keeps its existing behavior, so the
contract is additive for every shipped plugin.

## Work orders

| Work order | Scope | Status |
| --- | --- | --- |
| [task-01-host-submenu-contract.md](task-01-host-submenu-contract.md) | The contract, the boundary, keys, icon resolution, palette flattening and their tests | done |

## Design notes

- `KanbanCardMenuEntry` already models `kind: "submenu"` for the built-in Move
  to / Priority / Link menus, and `ContextEntry`/`DropdownEntry` recurse into
  `children`, so cards, the shared task-row menu, the preview/detail menus and the
  palette all inherit plugin submenus from one entry builder.
- The contract's fallback rule is what keeps one implementation valid on two host
  generations: a host predating `items` reads only `label`/`icon`/`run`, and a
  build whose `items()` yields nothing usable renders that same flat item.
- Plugin bundles are plain JavaScript, so the boundary is written for values the
  registration types do not describe: a throwing getter, a Proxy, a thenable, a
  `Symbol` id, a lone surrogate, an element where a component is expected.
- Keys are identity, not display: the palette uses an entry key as both a React
  key and cmdk's value, which is why injectivity is a requirement rather than a
  nicety.

## Verification

- `cd apps/web && ./node_modules/.bin/tsc --noEmit --incremental false`
- `cd apps/web && pnpm vitest run components/plugins/task-menu-actions.test.tsx components/kanban-card-edit-submenu.test.tsx components/kanban-card-menu-items.test.tsx components/kanban-card-menu-grouping.test.tsx components/kanban-card-plugin-context.test.tsx lib/kanban/task-actions-menu-entries.test.ts lib/plugins/sdk-contract.test.ts lib/plugins/icons.test.ts components/task/task-switcher-context-menu.test.tsx components/task-command-choices.test.tsx components/task-commands.test.tsx`
- `cd apps/web && ./node_modules/.bin/eslint --max-warnings 0 <changed files>` and `pnpm i18n:check`
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`
- The consumer plugin's own suite (`node --test ui/bundle.test.js` in its
  repository) exercises the declared `items`/`run` pair against this contract.
