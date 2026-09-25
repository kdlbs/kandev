---
created: 2026-09-24
status: implemented
requirements:
  - REQ-INTEGRATIONS-JIRA-DEFAULT-VIEW-001
system_design:
  - ../../specs/integrations/system-design/jira-default-view.md
legacy_specs: []
---

# Implementation Plan: Jira Default View

## Overview

Issue [#3921](https://github.com/kdlbs/kandev/issues/3921) reports that every `/jira` visit starts on **Assigned to me** even when a user always works from the same saved custom-JQL view. `useFilterState` initializes `activeViewId` to `builtin:assigned`, while `useSavedViews` hydrates saved views later without selecting one. Add a per-user default ID, resolve it before the first ticket search, and expose set/clear controls in the existing view picker. Implement the settings contract first, then selection and persistence, then the rendered controls and end-to-end proof.

## Scope

### In scope

- One user-owned default that may identify a built-in or custom Jira view.
- Exact saved-filter and custom-JQL restoration, missing-view fallback, and unchanged no-default behavior.
- Desktop and phone picker actions, translations, public documentation, and focused regression coverage.

### Out of scope

- Active-view URL parameters, bookmarks, and sharing.
- Last-selected-view persistence or defaults for other providers.
- Changing Jira workspace configuration or saved-view query semantics.

## Technical approach

1. Add scalar `jira_default_view_id` to the existing user-settings GET/PATCH path: `apps/backend/internal/user/models/models.go`, `dto/dto.go`, `controller/controller.go`, `service/service.go`, `store/sqlite.go`, and `internal/settingscatalog/defaults.go`. Regenerate `apps/web/lib/settings-discovery/contract.generated.json`. Add the web transport field in `apps/web/lib/types/http-user-settings.ts`. Empty string clears the choice; an omitted field preserves it.
2. Extend `apps/web/components/jira/my-jira/use-saved-views.ts` to hydrate the default and views together, expose readiness and an acknowledged set/clear operation, and clear the default in the same PATCH as default-view deletion. Add a pure resolver for an available default and the legacy **Assigned to me** fallback. Use `apps/web/app/jira/jira-page-client.tsx` to apply the resolved view once, protect manual interaction during hydration, restore custom JQL, and gate `useJiraSearch` until initial selection is known.
3. Add marker and sibling set/clear action to `apps/web/components/jira/my-jira/list-toolbar.tsx`. The selected view and default view are distinct states. Reuse the existing picker and deletion confirmation. Localize new copy in all Jira catalogs and document the user workflow in `docs/public/integrations.md`.

## ASCII UI preview

`UI-01` is the existing `/jira` saved-view picker. The primary row action selects a view; the star action changes the future default. The drawing shows control order and states, not pixel spacing or final icon styling.

Current desktop and phone picker (source: `ListToolbar`):

```text
Views: Assigned to me
  Built in
    [check] Assigned to me
            Unassigned
  Saved
            My open tickets             [Delete]
```

Proposed desktop picker, after **My open tickets** is marked default:

```text
Views: Assigned to me
  Built in
    [check] Assigned to me             [Set default]
            Unassigned                 [Set default]
  Saved
            My open tickets       [star: default] [Delete]
```

Proposed phone picker, using the current inset picker surface:

```text
Views: Assigned to me  [tap]
  Built in
    [check] Assigned to me       [star, 44px]
            Unassigned           [star, 44px]
  Saved
            My open tickets      [star, 44px] [Delete, 44px]
  (one vertical scroll region; actions stay inside the viewport)
```

The visible star identifies the default; its accessible name says **Set ... as default view** or **Clear ... as default view**. A default action leaves the current view unchanged. The phone surface keeps a visible touch target for each action and does not rely on hover. This preview covers `AC-INTEGRATIONS-JIRA-DEFAULT-VIEW-001.5` and `.6`.

## Tests

| Acceptance criteria | Focused evidence |
| --- | --- |
| `.1`, `.7` | Backend GET/PATCH and SQLite round-trip tests; `use-saved-views.test.ts` acknowledged writes, replacement, clear, and failure tests. |
| `.2`, `.3`, `.4`, `.5` | New pure resolver and `jira-page-client` state tests, including exact custom JQL, no-default project key, deleted view, and late hydration after manual selection. |
| `.6` | `list-toolbar.test.tsx` selection isolation, accessible names, and phone hit-target assertions. |

## E2E tests

- `apps/web/e2e/tests/integrations/jira-default-view.spec.ts` (desktop): mark a custom JQL view as default, revisit `/jira`, inspect the query and results, delete it, and verify the next visit falls back to **Assigned to me**. Covers `.1` through `.4`.
- `apps/web/e2e/tests/integrations/mobile-jira-default-view.spec.ts` (`mobile-chrome`): set and clear a default in the phone picker, revisit the page, and verify touch reachability and no horizontal overflow. Covers `.1`, `.2`, `.5`, and `.6`.

## Work orders

- [x] [Task 01: Persist the Jira default preference](task-01-persist-jira-default.md)
- [x] [Task 02: Restore the default Jira view](task-02-restore-jira-default.md)
- [x] [Task 03: Add picker controls and prove the flow](task-03-jira-default-controls.md)

Work orders run sequentially in the primary session. Planning a dependency wave does not authorize delegation.

## Verification results

All three work orders are complete. The user settings contract persists a per-user default view ID. `/jira` restores an available built-in or custom view, including exact custom JQL, before its displayed ticket search, and retains a manual choice made during hydration. The picker provides localized, accessible set/clear actions on desktop and phone. Deleting the active default clears it and returns the current and next visit to **Assigned to me**. Project-status reconciliation waits for readiness keyed to the current workspace and project set and skips structured-status cleanup while saved custom JQL owns the query. Saved-view list mutations serialize, new views publish only after an acknowledged settings write, and the desktop picker keeps its active default star visible without hover. Public Jira documentation explains how to choose or clear a default.

Focused backend tests pass. The six-file Jira web suite passes 33 tests; after review remediation, the focused four-file suite passes 34 tests. Web typecheck and i18n checks pass. Desktop and mobile Playwright scenarios pass, the production web build succeeds, and both public documentation validators pass.

## Risks

- Settings hydrate after the page mounts. Initial search must wait for the resolved target, and late responses must not overwrite a user action.
- Saved views and the default ID share one user-settings document. Deleting the default must clear both fields in one PATCH; failed writes must retain the prior state.
- A custom JQL may refer to projects unavailable in another workspace. The page should preserve the saved query and show the existing Jira search error rather than silently rewrite it.
- The picker is narrow on phones; star and delete targets must remain reachable without horizontal scrolling.
