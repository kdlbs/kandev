---
created: 2026-09-15
status: completed
requirements:
  - REQ-UI-WORKSPACE-SIDEBAR-VIEWS-001
system_design:
  - ../../specs/ui/system-design/workspace-sidebar-task-views.md
legacy_specs: []
---

# Implementation Plan: Workspace Sidebar Task Views

## Overview

Deliver personal workspace-scoped views with one-time legacy migration. Implement
and test the backend contract first, then integrate all client surfaces. Work is
sequential in the primary session. The user authorized implementation after the
design-package handoff.

## Scope

Include scoped persistence, migration, projection, every sidebar-view mutation,
workspace restoration, failure isolation and desktop/phone evidence. Exclude
Threads, integration saved queries, shared views, automatic colors and task order.

## Technical approach

Follow the [design](../../specs/ui/system-design/workspace-sidebar-task-views.md).
Use a typed per-workspace user-settings map, one-entry PATCH and existing CAS.
Keep backend normalization authoritative and migrate the frontend from a global
slice to selectors over workspace entries. Capture workspace identity across
async operations and overlay callbacks. No new dependency or feature flag.

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

## Tests

- AC .1/.4: backend sidebar workspace tests and `sidebar-view-actions.test.ts`
  cover independent CRUD, names, limits, defaults and reference normalization.
- AC .2/.5: `hydrator.test.ts`, `users.test.ts`, `workspace-slice.test.ts` and
  action tests cover boot/reload, revision ordering, per-workspace queues,
  concurrent writes, failed create followed by selection, and A-to-B-to-A races.
- AC .3: new `sidebar_workspace_views_test.go` in user store/service covers
  restart, repeated migration, preserved fields, existing scoped entries,
  empty accounts and failed workspace enumeration or CAS.
- AC .6: popover/manager tests cover editor and deletion-confirmation scope.

## E2E tests

New `tests/task/workspace-sidebar-views.spec.ts` (`chromium`) and
`tests/task/mobile-workspace-sidebar-views.spec.ts` (`mobile-chrome`) create two
workspaces, edit A, switch to B, create/select/edit B, return to A and reload.
Assert names, active view and filtering, not visibility alone (AC .1/.2/.4/.6).
Test stale editor/confirmation dismissal and phone drawer focus/containment.
Use existing mobile sidebar/task-view access suites as regressions. Backend
migration and controlled async failure matrices remain deterministic unit tests.
Managed E2E runner builds fresh artifacts and uses isolated fixtures; follow
causal-wait helpers and do not add fixed sleeps or worker overrides.

## Work orders

- [x] [Task 01: Persist and migrate scoped views](task-01-scoped-settings.md)
- [x] [Task 02: Integrate scoped desktop and phone views](task-02-client-surfaces.md)

## Verification commands

Run each work order's subset first. The complete package checks are:

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

## Documentation impact

Updated the task-view paragraph in
`docs/public/integrations.md` (how-to) with personal workspace scope and migration.
Reconciled the 50-view/global-persistence wording in
`docs/specs/ui/requirements/sidebar-view-creation.md` and relevant sidebar
creation companion artifacts discovered by catalog. Existing completed plans
retain their historical results and link to this continuation.

## Verification results

Design validation on 2026-09-15:

- `python3 scripts/list-docs.py validate`: passed (272 decisions, 936 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed. New package files also checked for trailing whitespace.
- Catalog discovers the requirement and system design.

Implementation verification on 2026-09-15:

- Backend user and backendapp suites passed, including service concurrency under the race detector.
- Focused frontend suite passed: 132 tests across 11 files.
- Targeted ESLint, localization, public-document tests and validators passed.
- Final TypeScript check passed.
- Desktop browser coverage passed: 23 tests (workspace isolation, sidebar filters, subtask state sorting).
- Phone browser coverage passed: 14 tests (workspace isolation, mobile sidebar views and app-navigation access).
- Desktop and phone screenshots inspected; controls remain contained and only the active workspace's collection is displayed.
- Task-level results record commands and remaining evidence.

## Risks

- Old open clients must refresh before editing views after migration.
- Copied workspace-ID filters can legitimately match no tasks in other workspaces.
- Pending writes and broadcast snapshots need per-workspace reconciliation.
- Migration must use an authorized workspace list, including in auth-disabled mode.
