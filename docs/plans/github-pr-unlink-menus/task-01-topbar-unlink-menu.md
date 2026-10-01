---
id: "01-topbar-unlink-menu"
title: "Top-bar unlink menu and shared action"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.1
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.3
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.4
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.5
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.7
system_design:
  - ../../specs/integrations/system-design/github-pr-unlink-menus.md
---

# Task 01: Top-bar unlink menu and shared action

## Summary

Right-clicking the task's GitHub PR control opens `Edit` with an exact unlink
choice for each linked PR. The action reuses the current association mutation
and leaves normal PR navigation and CI hover intact.

## In scope

- Single- and multi-PR top-bar context menu with localized labels.
- Scoped shared unlink action only if needed for card reuse, pending and error
  feedback, focus and overlay interaction.
- Focused component and desktop browser tests.

## Out of scope

- Card and phone menus, GitLab and plugin providers, backend API or schema
  changes.

## Acceptance

1. Right-click and keyboard context-menu activation expose `Edit` and the
   exact PR choices while click, multi-PR selection, and hover retain their
   current behavior.
2. Each choice unlinks only its association; duplicate activation is guarded,
   failure keeps the PR and permits retry, and the final PR can disappear.
3. Desktop browser coverage proves persisted unlink and sibling preservation.

## ASCII UI preview

`UI-01: Desktop top-bar PR control; right-click; one linked PR`
([combined preview](plan.md#ascii-ui-preview))

```text
[ PR #3943 ]  right-click  >  Edit  >  Unlink pull request #3943
```

For multiple PRs, the submenu lists one repository-qualified row per
association. These controls map to `AC-001.1`, `.3`, `.4`, `.5`, and `.7` of
the requirement above.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/github/pr-topbar-button.test.tsx hooks/domains/github/use-task-pr.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run tests/pr/pr-topbar-unlink-context-menu.spec.ts)
```

## Files likely touched

- `apps/web/components/github/pr-topbar-button.tsx`
- `apps/web/components/github/pr-topbar-button.test.tsx`
- `apps/web/hooks/domains/github/use-task-pr.ts` and its test, if action extraction is needed
- `apps/web/src/locales/*/github.json`
- `apps/web/e2e/tests/pr/pr-topbar-unlink-context-menu.spec.ts`

## Dependencies

None. Reuse the existing backend unlink endpoint and WebSocket removal event.

## Risks

- Radix context, dropdown, and hover layers currently share a trigger; check
  event ordering and focus when the final link unmounts it.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-unlink-menus.md)
- [System design](../../specs/integrations/system-design/github-pr-unlink-menus.md)
- Existing `PRTopbarButton`, `MultiPRCIPopover`, and `useTaskPR` tests.

## Results

Implemented the desktop top-bar `Edit` context submenu and extracted the
workspace-scoped association mutation for menu reuse. Targeted tests cover exact
selection, duplicate PR numbers across repositories, final-link removal,
pending duplicate suppression, stale association rejection, failure retention,
and retry.

Validation passed:

- `pnpm exec vitest run components/github/pr-status-refresh-routes.test.tsx hooks/domains/github/use-task-pr.test.tsx hooks/domains/github/use-task-pr-unlink.test.tsx` (24 tests)
- `pnpm run typecheck`
- `pnpm e2e:run tests/pr/pr-topbar-unlink-context-menu.spec.ts`
