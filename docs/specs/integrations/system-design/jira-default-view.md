---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-JIRA-DEFAULT-VIEW-001
---

# Jira Default View System Design

## Purpose and boundaries

The Jira integration owns view selection and restoration. The existing user-settings service provides per-user storage; Jira connection settings remain workspace-scoped. The default references a saved-view identity and does not duplicate filters or JQL.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-JIRA-DEFAULT-VIEW-001` | [Persistence and settings contract](#persistence-and-settings-contract), [Initial selection](#initial-selection), [Picker and mutations](#picker-and-mutations), [Failure and recovery](#failure-and-recovery) |

## Persistence and settings contract

Add `jira_default_view_id` beside `jira_saved_views` in `GET` and `PATCH /api/v1/user/settings`. It is a string containing either a built-in ID such as `builtin:assigned`, a custom saved-view ID, or `""` for no default. An omitted PATCH field leaves the value unchanged. The backend trims an explicitly supplied string; `""` clears it. This field flows through `internal/user` model, DTO, controller, service, SQLite JSON payload, and the settings catalog, then through the web HTTP type. No database migration or separate settings endpoint is needed because the existing user-settings payload stores new fields by JSON name. Only the authenticated user's settings are read or changed.

The frontend treats an ID as effective only when it matches a current built-in view or a normalized custom view in `useSavedViews`. It never trusts a dangling or malformed ID as a query. The current saved-view format and exact `customJql` string remain unchanged. Existing users have an empty default and retain existing landing behavior.

## Initial selection

`useSavedViews` hydrates saved views and the default ID from one user-settings response and exposes readiness. `useFilterState` resolves an initial target after that hydration. A valid custom default supplies its saved filters and `customJql`; a valid built-in supplies its built-in filters. The JQL editor opens for a JQL-backed default, following manual view selection. An explicit default does not inherit the workspace default project key. With no effective default, `initialFilters(defaultProjectKey)` preserves the current **Assigned to me** behavior.

`AuthenticatedView` does not start its ticket search until this initial target is resolved, so a default does not first issue an unrelated **Assigned to me** search. A manual selection or filter edit made before hydration wins over the late settings response. Subsequent default changes do not reinitialize the current view.

## Picker and mutations

The existing `ListToolbar` view rows gain a visible default marker and sibling set/clear action. The primary row button still selects the view. The default action uses an accessible name, pressed state, and localized copy; it never triggers row selection. Fine-pointer controls retain toolbar density, and phone/coarse-pointer controls expose a visible hit target of at least 44px. The existing picker remains the phone entry point and vertical scroll owner.

`useSavedViews` owns default mutations alongside saved-view mutations. Setting or clearing a default persists an acknowledged user-settings PATCH before publishing the new marker. Removing the default custom view sends the updated `jira_saved_views` array and an empty `jira_default_view_id` in the same PATCH. A default mutation in progress blocks conflicting default actions and deletion of that default. Removing the active default selects the legacy fallback on the current page.

## Failure and recovery

If settings hydration fails, the page remains usable with **Assigned to me** and can retry loading settings through the existing hook path. A missing or malformed default ID uses the fallback without writing settings merely because the page opened. If a default mutation fails, retain the prior rendered and persisted marker, surface a localized error, and permit retry. If a combined deletion PATCH fails, restore the prior saved-view list and default marker.

## Verification boundaries

Backend tests cover omitted, set, and clear PATCH semantics and SQLite round-trip. Hook tests cover hydration, default replacement, missing-view fallback, custom JQL, mutation ordering, and failure recovery. Component tests cover desktop and phone row actions. Desktop and mobile Playwright scenarios cover marking a custom default, reloading `/jira`, applying its exact JQL, and deleting it to reach the fallback.
