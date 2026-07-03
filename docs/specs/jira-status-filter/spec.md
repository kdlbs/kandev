---
status: approved
created: 2026-07-03
owner: cfl12
---

# Jira Ticket Status Filter

## Why
On the Jira tickets page, the "Status" filter only offers Jira's three built-in
status *categories* (To Do / In Progress / Done), not the real workflow statuses
that appear on tickets (e.g. `In Development`, `Ready for review`). Users read
those real status names on every ticket row, look for them in the filter, and
can't find them — so the filter looks broken and can't express "show me only
tickets that are Ready for review". Reported in issue #1588.

## What
- The filter bar SHALL offer a status filter whose options are the **real Jira
  status names** of the selected project(s), not the three status categories.
- The status filter SHALL be **multi-select**; selecting statuses narrows the
  ticket list to tickets whose status name is one of the selected values.
- The status options SHALL be sourced from the project(s) currently selected in
  the Project filter. When more than one project is selected, the options are the
  **union** of those projects' statuses, de-duplicated by status name.
- When no project is selected, the status filter SHALL be disabled and show a
  hint directing the user to pick a project first (matching the issue's target
  workflow: choose project → statuses populate).
- Selected statuses SHALL be reflected in the composed JQL as
  `status in ("Name", …)` (replacing the previous `statusCategory in (…)`).
- The previous status-category filter is **removed**, per the acceptance criteria
  ("current status filter is renamed or removed").
- Changing the Project selection SHALL drop any selected statuses that are no
  longer available in the new project set (no stale/unselectable statuses persist).
- Fetched statuses per project SHALL be cached for the page session so switching
  project selections back and forth does not refetch.

## Data model
No persistent backend state. One new transport type is introduced.

Backend Go type (in `apps/backend/internal/jira/models.go`), mirrored on the
frontend in `apps/web/lib/types/jira.ts`:

```
JiraStatus
  id             string   Jira status id
  name           string   display name, e.g. "Ready for review"
  statusCategory string   "new" | "indeterminate" | "done" (for badge colour only)
```

`FilterState` (frontend, `apps/web/components/jira/my-jira/filter-model.ts`)
changes its status field from `statusCategories: JiraStatusCategory[]` to
`statuses: string[]` (status names). Saved views persisted in localStorage that
still carry the old `statusCategories` shape MUST load without error — the old
field is ignored and treated as "no status filter".

## API surface
New backend endpoint, following the existing `GET /api/v1/jira/projects` pattern:

- `GET /api/v1/jira/projects/:key/statuses`
  - Response `200`: `{ "statuses": JiraStatus[] }` — flattened + de-duplicated by
    status id across the project's issue types.
  - Errors follow the shared Jira handler mapping: `503`
    `{ "code": "JIRA_NOT_CONFIGURED" }` when not configured; upstream `401/403/404`
    passed through; other failures `500`.

New client method on the `Client` interface / `CloudClient`
(`apps/backend/internal/jira/client.go`, `cloud_client.go`):
- `ListProjectStatuses(ctx, projectKey string) ([]JiraStatus, error)` — calls
  Jira `GET {apiBase}/project/{projectKey}/statuses` (`apiBase` is `/rest/api/3`
  for Cloud, `/rest/api/2` for Server/DC), flattening the per-issue-type status
  arrays and de-duplicating by id. `MockClient` gains a matching seed setter for
  E2E.

New frontend API function (`apps/web/lib/api/domains/jira-api.ts`):
- `listJiraProjectStatuses(projectKey)` → `{ statuses: JiraStatus[] }`.

JQL contract (`filter-model.ts`): the status clause becomes
`status in ("A", "B")` with names quoted/escaped exactly as project keys are today.
Empty selection emits no clause.

## Failure modes
- **Status fetch fails / project has no statuses:** the status filter shows no
  selectable options (disabled with an unobtrusive hint); the ticket list is
  unaffected and no status clause is added to the JQL. Failure is non-fatal and
  logged, consistent with how project loading already degrades on this page.
- **Not configured:** the page already gates on Jira config; the endpoint returns
  `503 JIRA_NOT_CONFIGURED` and the filter simply has no options.

## Scenarios
- **GIVEN** Jira is configured and no project is selected, **WHEN** the user opens
  the Status filter, **THEN** it is disabled and shows a hint to select a project.
- **GIVEN** a project `CLIP` whose workflow includes `In Development` and
  `Ready for review`, **WHEN** the user selects project `CLIP`, **THEN** the Status
  filter lists the real `CLIP` status names including `In Development` and
  `Ready for review` (not To Do / In Progress / Done).
- **GIVEN** project `CLIP` selected, **WHEN** the user checks `Ready for review`,
  **THEN** the composed JQL contains `status in ("Ready for review")` and the
  ticket list shows only tickets in that status.
- **GIVEN** two projects selected with overlapping status names, **WHEN** the user
  opens the Status filter, **THEN** each status name appears once (union,
  de-duplicated).
- **GIVEN** `Ready for review` is selected under project `CLIP`, **WHEN** the user
  deselects `CLIP` and selects a project without a `Ready for review` status,
  **THEN** `Ready for review` is dropped from the active filter and the JQL no
  longer references it.
- **GIVEN** a saved view persisted with the old `statusCategories` field, **WHEN**
  the page loads that view, **THEN** it loads without error and applies no status
  filter.
- **GIVEN** the status endpoint returns an error for the selected project, **WHEN**
  the user opens the Status filter, **THEN** it shows no options and the ticket
  list is still returned by the other filters.

## Out of scope
- Filtering by status *category* (the old behavior) — removed, not preserved.
- Persisting fetched statuses beyond the page session (no backend cache/table).
- Editing or transitioning statuses from the filter (transitions already exist
  elsewhere).
- Assignee/priority/type filter changes.

## Decisions
- When no project is selected, the status filter is disabled and shows a hint to
  pick a project first (confirmed with owner, matches the issue's target
  workflow). It does not fall back to the old category filter.
