---
id: "03-sidebar-unlink-menu"
title: "Sidebar row unlink menu and guide"
status: done
wave: 3
depends_on:
  - "01-topbar-unlink-menu"
  - "02-card-unlink-menu"
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

# Task 03: Sidebar row unlink menu and guide

## Summary

The sidebar task row shown in the request gets an `Edit` submenu with an exact
unlink choice for each linked GitHub PR. The row's visible dots control offers
the same path on a phone. The public guide explains all new entry points.

## In scope

- Row-scoped PR records, on-open hydration, and shared unlink action.
- Sidebar Edit submenu that keeps Rename and Duplicate at root level and hides
  single-task unlink from reduced multi-selection menus.
- Desktop and phone browser coverage and `docs/public/sessions-and-review.md`.

## Out of scope

- Kanban card menu, GitLab and plugin provider unlink, backend API/schema/event
  contract changes.

## Acceptance

1. Desktop row right-click and phone row dots expose exact association choices
   under Edit, without changing row selection, drag, or navigation.
2. An unresolved summary shows a disabled loading row; success removes only
   the selected association, and failure retains a retryable choice.
3. Desktop and phone browser flows prove the row result across reload, phone
   hit targets and containment; the public guide describes the new paths.

## ASCII UI preview

`UI-04: Sidebar task row; right-click or visible dots; one linked PR`
([combined preview](plan.md#ascii-ui-preview))

```text
Task title [PR]                 ...
  Edit > Edit task
         Unlink pull request #3943
  Rename
  Duplicate
```

`UI-05: Phone sidebar task row; visible dots; one linked PR`

```text
Task title [PR]               [ ... ]  tap
  bottom menu: Edit > Edit task
                        Unlink pull request #3943
```

On phone, dots is a visible 44-pixel target, menu rows are at least 44 pixels,
and the bottom menu owns vertical scroll. These views map to `AC-001.2`
through `.7` of the requirement above.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/task-switcher-context-menu.test.tsx components/task/task-switcher.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/task/pr-unlink-sidebar-menu.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-pr-unlink-sidebar-menu.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-switcher-context-menu-items.tsx`
- `apps/web/components/task/task-switcher-context-menu.test.tsx`
- `apps/web/src/locales/*/task.json` or `integrations.json`
- `apps/web/e2e/tests/task/pr-unlink-sidebar-menu.spec.ts`
- `apps/web/e2e/tests/task/mobile-pr-unlink-sidebar-menu.spec.ts`
- `apps/backend/internal/task/statussummary/projector.go` and its regression test
- `docs/public/sessions-and-review.md`

## Dependencies

Task 01's shared mutation; Task 02's menu and browser patterns.

## Risks

- Sidebar Edit, Rename, and Duplicate are separate root items today; only
  Edit should become a submenu for linked PRs.
- Sidebar rows can be selected in bulk; never unlink a PR from another task.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-unlink-menus.md)
- [System design](../../specs/integrations/system-design/github-pr-unlink-menus.md)
- `TaskItemWithContextMenu`, `SingleEditGroup`, and existing sidebar E2E flows.

## Results

Implemented the sidebar task-row Edit submenu with exact association labels,
on-open hydration, a disabled loading entry, and the shared scoped unlink
action. Rename and Duplicate remain at the root. Reduced multi-selection menus
omit the row-specific action. The phone row dots open the same menu surface.

Updated `docs/public/sessions-and-review.md` with the top-bar, task-row, and card
entry points and the effect of unlinking an association.

Verification passed:

- 108 focused Vitest tests across nine files.
- Frontend typecheck, i18n check, and new-copy ratchet.
- Production Vite build.
- Three desktop E2E tests for the top bar, card, and sidebar row.
- Two mobile E2E tests for the card and sidebar row, including target sizing,
  containment, and persistence after reload.
- Public-doc tests (62) and validation of all 47 published pages.
- `git diff --check`.

Targeted ESLint found no code-quality errors. One existing max-lines warning
remains in `task-switcher-context-menu.test.tsx`; that test file already
exceeded the configured limit before this work.

## Review follow-up

The status-summary projector now consumes the existing PR-deleted event and
reloads active task associations. It compares the reloaded projection with the
persisted summary, including after a cold projector restart. The row and card
PR icon also suppresses a stale compact summary while a local deletion
tombstone records that the final association is gone. The shared unlink action
checks its captured workspace ID and context generation after the HTTP request;
a late response cannot reset a newly loaded workspace cache. The public guide
now documents desktop top-bar right-click unlink. Unrelated locale rewrites
were restored, leaving only the new GitHub translation key entries.

Review follow-up validation passed: 144 focused Vitest tests across eleven
files, `go test ./internal/task/statussummary`, frontend typecheck and i18n
checks, the production E2E build, three desktop E2E tests and two mobile E2E
tests, specification and public-doc validation, and `git diff --check`.
