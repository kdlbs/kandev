---
id: "02-card-unlink-menu"
title: "Card unlink menu and phone path"
status: done
wave: 2
depends_on:
  - "01-topbar-unlink-menu"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.2
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.3
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.4
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.5
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.6
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.7
system_design:
  - ../../specs/integrations/system-design/github-pr-unlink-menus.md
---

# Task 02: Card unlink menu and phone path

## Summary

The card's existing `Edit` menu lists exact GitHub PR unlink choices in both
desktop menu variants and the phone dots menu.

## In scope

- Scoped card association reads and on-demand hydration for summary-only
  cards; non-actionable loading state while exact rows are unavailable.
- `Edit` submenu composition with native unlink and existing plugin actions.
- Desktop card and phone card Playwright coverage and locale values.

## Out of scope

- Sidebar row menus, new PR persistence or backend contract, bulk selection,
  MR/provider unlink.

## Acceptance

1. The card context and dots menus show the same per-association choices; the
   phone dots path needs no gesture hidden from touch users.
2. Empty, loading, multi-PR, plugin-contributed Edit, stale-context, pending,
   and failure states keep the correct task and association boundary.
3. Desktop and phone browser tests show the selected association disappears
   across reload, the phone targets are at least 44 pixels, and page width
   stays contained.

## ASCII UI preview

`UI-02: Desktop card; right-click or dots; two linked PRs`
([combined preview](plan.md#ascii-ui-preview))

```text
Card  [PR 2]  ...   >   Edit > Edit task
                               Unlink acme/web #3943
                               Unlink acme/api #3943
                               (existing plugin actions)
```

`UI-03: Phone card; visible dots; two linked PRs`

```text
Card  [PR 2]  [ ... ]  tap  >  Edit > Unlink acme/web #3943
                                         Unlink acme/api #3943
```

Rows have a 44-pixel phone hit target and no horizontal page overflow. During
exact-record hydration, replace choices with a disabled loading row. These
views map to `AC-001.2` through `.7` of the requirement above.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/kanban-card-edit-submenu.test.tsx components/kanban-card-menu-items.test.tsx components/kanban-card-menu-grouping.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/kanban/pr-unlink-card-menu.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-pr-unlink-card-menu.spec.ts)
```

## Files likely touched

- `apps/web/components/kanban-card-menu.tsx`
- `apps/web/components/kanban-card-edit-submenu.tsx`
- `apps/web/components/kanban-card-menu-items.tsx`
- Relevant colocated menu tests
- `apps/web/src/locales/*/kanban.json` or `integrations.json`
- `apps/web/e2e/tests/kanban/pr-unlink-card-menu.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-pr-unlink-card-menu.spec.ts`

## Dependencies

Task 01 supplies the shared scoped unlink action and menu feedback pattern.

## Risks

- A card may initially have only compact PR status. Never use its PR number
  as an association ID.
- The `Edit` submenu can already contain plugin actions; keep their order and
  visibility stable.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-unlink-menus.md)
- [System design](../../specs/integrations/system-design/github-pr-unlink-menus.md)
- `KanbanCardActions`, `KanbanCardContextMenu`, and mobile task-priority E2E.

## Results

Passed: 48 focused Vitest tests, frontend typecheck, and `pnpm run i18n:check`.
Desktop `pr-unlink-card-menu.spec.ts` and `mobile-pr-unlink-card-menu.spec.ts`
passed. The phone test verified both the dots and unlink row hit targets, the
selected association after reload, and no horizontal document overflow.
