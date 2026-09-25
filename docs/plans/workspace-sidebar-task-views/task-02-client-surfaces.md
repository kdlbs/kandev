---
id: "02-client-surfaces"
title: "Integrate scoped desktop and phone views"
status: done
wave: 2
depends_on: ['01-scoped-settings']
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-SIDEBAR-VIEWS-001
acceptance_criteria:
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.1
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.2
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.4
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.5
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.6
system_design:
  - ../../specs/ui/system-design/workspace-sidebar-task-views.md
---

# Task 02: Integrate scoped desktop and phone views

## Summary

Typed client mapping, workspace selectors, actions, hydration/live updates, async journals, scoped overlays, desktop/phone tests and public docs.

## In scope

Typed client mapping, workspace selectors, actions, hydration/live updates, async journals, scoped overlays, desktop/phone tests and public docs. Write failing targeted tests before production changes and preserve
existing sidebar normalization and optimistic-write guarantees.

## Out of scope

Backend persistence design is task 01. No changes to Threads, integration queries,
shared preferences, automatic task colors, or manual task ordering.

## Acceptance

- Every view read/mutation and restoration resolves the active workspace without a global fallback.
- Delayed responses, broadcasts and stale overlay callbacks cannot change another workspace.
- Desktop and phone flows pass targeted tests; scope documentation and package statuses match implementation.

## ASCII UI preview

UI-01: Desktop sidebar after switching from workspace A to workspace B.

```text
[Workspace B v]
Tasks [B's selected view v] [Filter]
  B's matching tasks
Picker: B's saved views | New view
```

UI-02: Phone, app navigation > Task views, workspace B active.

```text
+--------------------------------+
| Task views              [Close] |
| [B view] [B view] [+] [Filter]  |
| B's matching tasks             |
|        (internal scroll)       |
+--------------------------------+
```

The existing workspace selector supplies context; these drawings do not add
labels. Phone drawer controls stay fixed, chips scroll within their row and
only the task body scrolls vertically. For a fresh workspace the selected view
is All tasks. With no valid workspace, no prior workspace views or mutable
editor remain. A switch dismisses an editor/confirmation from the old scope.
These scope and hierarchy rules are required by AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.1,
.4 and .6; spacing is illustrative. Existing localized copy and primitives stay.

Full preview: [plan](plan.md#ascii-ui-preview).

## Verification

Run from repository root. Installation is needed once in a fresh worktree.
Add and run the tests mapped in [plan](plan.md#tests), capturing red then green.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/...)
(cd apps/web && pnpm exec vitest run lib/state/slices/ui/sidebar-workspace-views.test.ts lib/state/slices/ui/sidebar-view-actions.test.ts lib/state/slices/ui/sidebar-view-wire.test.ts lib/state/hydration/hydrator.test.ts lib/ws/handlers/users.test.ts lib/state/slices/workspace/workspace-slice.test.ts hooks/use-sidebar-views-sync.test.ts components/task/sidebar-filter/use-sidebar-view-popover.test.tsx components/task/sidebar-filter/view-manager.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/use-ensure-user-settings.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/workspace-sidebar-views.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-workspace-sidebar-views.spec.ts tests/task/mobile-sidebar-views.spec.ts tests/github/mobile-task-view-access.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-types.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-actions.ts`
- `apps/web/lib/state/slices/ui/sidebar-view-wire.ts`
- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/hooks/domains/sidebar/use-effective-sidebar-view.ts`
- `apps/web/hooks/use-sidebar-views-sync.ts`
- `apps/web/components/app-sidebar/sections/tasks-view-picker.tsx`
- `apps/web/components/task/sidebar-filter/`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/components/confirmation/use-saved-task-view-delete-confirmation.ts`
- `apps/web/e2e/tests/task/workspace-sidebar-views.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-workspace-sidebar-views.spec.ts (new)`
- `docs/public/integrations.md and referenced specification files`

## Dependencies

Task 01 must pass first. Migrate every direct sidebarViews reader discovered with rg, including boot-state types and shared deletion hooks.

## Risks

A global queue, global rollback or full-settings broadcast can cross workspace boundaries. Include duplicated legacy view IDs in race tests. Do not use browser storage as a fallback.

## Parallelism

Sequential. No delegation authorized.

## Inputs

- [Requirements](../../specs/ui/requirements/workspace-sidebar-task-views.md)
- [System design](../../specs/ui/system-design/workspace-sidebar-task-views.md)
- Existing sidebar action tests and `internal/user/service/user_settings_cas_test.go`.
- Existing mobile sidebar and Task views access E2E suites for task 02.

## Results

Implemented workspace selectors, scoped optimistic queues and rollback, shared
boot/HTTP/live mapping with per-entry revision guards, and workspace-bound
editor callbacks. Updated all E2E setup writes to the scoped contract.

- Focused frontend suite: 132 tests passed across 11 files.
- Final corrected fixture regression: 8 tests passed.
- `pnpm run typecheck`: passed.
- Targeted ESLint for changed TypeScript files: passed with no warnings.
- `pnpm run i18n:check`: passed.
- Public-document tests, public-document validation, catalog and specification lint: passed.
- Backend and Vite E2E builds: passed.
- Desktop browser verification: 23 tests passed.
- Phone browser verification: 14 tests passed.
- Desktop and phone screenshots inspected: active workspace collection only, contained controls.

Browser commands after the fresh managed backend/Vite build (run from `apps/web`):

```bash
pnpm e2e:run --host --no-build --project chromium tests/task/workspace-sidebar-views.spec.ts tests/task/sidebar-filter.spec.ts tests/task/sidebar-subtask-state-sort.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-workspace-sidebar-views.spec.ts tests/task/mobile-sidebar-views.spec.ts tests/github/mobile-task-view-access.spec.ts
```

The initial sandboxed browser attempt could not start its backend. Both final runs
used approved local socket access and isolated test fixtures. Go builds used
`GOCACHE=/tmp/kandev-sidebar-go-cache` because the default cache was read-only.

Initial workspace state, migration and stale-response regressions were observed
failing before implementation and now pass.
