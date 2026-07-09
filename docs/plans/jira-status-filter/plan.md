---
spec: docs/specs/jira-status-filter/spec.md
created: 2026-07-03
status: draft
---

# Implementation Plan: Jira Ticket Status Filter

## Overview
Replace the Jira tickets page status-*category* filter with a real-status
multi-select sourced from the selected project(s). Backend gains one read
endpoint that lists a project's statuses; the frontend swaps the filter model
from `statusCategories` to `statuses`, fetches per-project statuses on demand,
and emits `status in (…)` JQL. Order: backend contract first, then frontend
types/API, filter model, UI wiring, then E2E.

---

## Backend

### JiraStatus model (`apps/backend/internal/jira/models.go`)
Add `JiraStatus{ ID, Name, StatusCategory string }` with JSON tags
`id`/`name`/`statusCategory`.

### Client method (`apps/backend/internal/jira/client.go`, `cloud_client.go`)
Add `ListProjectStatuses(ctx, projectKey string) ([]JiraStatus, error)` to the
`Client` interface. `CloudClient` implementation calls
`GET {apiBase}/project/{projectKey}/statuses` via the existing `do()` helper.
The upstream response is `[]{ id, name, statuses: []{ id, name, statusCategory:{ key } } }`
(per issue type). Flatten and de-duplicate by status id; map
`statusCategory.key` → `StatusCategory`.

### Mock client (`apps/backend/internal/jira/mock_client.go`)
Add `statuses map[string][]JiraStatus`, a `SetProjectStatuses(key, []JiraStatus)`
setter, and `ListProjectStatuses` returning the seeded slice. Wire a mock
control route `POST /api/v1/jira/mock/projects/:key/statuses` in
`mock_controller.go` following the existing mock route pattern.

### Service (`apps/backend/internal/jira/service.go`)
Add `ListProjectStatuses(ctx, projectKey)` using `clientFor()`.

### Handler + route (`apps/backend/internal/jira/handlers.go`)
Add `httpListProjectStatuses` returning `{ "statuses": [...] }`, using
`writeClientError`. Register `GET /projects/:key/statuses` in
`RegisterHTTPRoutes`.

---

## Frontend

### Types (`apps/web/lib/types/jira.ts`)
Add `interface JiraStatus { id: string; name: string; statusCategory: JiraStatusCategory }`.

### API client (`apps/web/lib/api/domains/jira-api.ts`)
Add `listJiraProjectStatuses(key, options?)` →
`fetchJson<{ statuses: JiraStatus[] }>('/api/v1/jira/projects/{key}/statuses')`.

### Filter model (`apps/web/components/jira/my-jira/filter-model.ts`)
- `FilterState.statusCategories: JiraStatusCategory[]` → `statuses: string[]`.
- Remove `STATUS_CATEGORY_OPTIONS`.
- `statusClause` emits `status in ("A", "B")` (reuse `quote`).
- Update `DEFAULT_FILTERS` and `filtersEqual`.

### Saved views back-compat (`apps/web/components/jira/my-jira/use-saved-views.ts`)
When hydrating persisted views, coerce any legacy `statusCategories` field to
`statuses: []` so old localStorage entries load without error.

### Status pill (`apps/web/components/jira/my-jira/filter-pills.tsx`)
Rewrite `StatusPill` to accept `options: JiraStatus[]`, `value: string[]`,
`disabled`, and a hint. Disabled + hint when no options (no project selected).
Multi-select by status name.

### Filter bar + page wiring (`filter-bar.tsx`, `app/jira/jira-page-client.tsx`)
Add a hook that fetches statuses for the selected `projectKeys` (union,
de-duped by name), caches per project key for the session, and drops selected
statuses no longer present when the project set changes. Pass options/disabled
into `StatusPill` via `FilterBar`.

---

## Tests

- **Client flatten/de-dupe** — `cloud_client_test.go`: mock `GET /rest/api/3/project/{key}/statuses` returning two issue types with overlapping statuses; assert de-dup by id and category mapping. (spec: union scenario, data model)
- **Handler shape/errors** — `handlers_test.go`: `200 {statuses:[...]}`, and `503 JIRA_NOT_CONFIGURED` when not configured. (spec: API surface, failure modes)
- **Service passthrough** — `service_test.go`: fake client returns statuses. 
- **filtersToJql status clause** — `filter-model.test.ts`: `statuses: ["Ready for review"]` → `status in ("Ready for review")`; empty → no clause. (spec: JQL contract)
- **Saved-view back-compat** — `use-saved-views` test: legacy `statusCategories` view hydrates to `statuses: []` without throwing. (spec: saved views scenario)
- **Stale-status drop** — unit test for the project→status reconciliation helper. (spec: deselect-project scenario)

---

## E2E Tests

- **Status populates from project** — `apps/web/e2e/jira/status-filter.spec.ts`: seed a mock project with statuses `In Development`, `Ready for review`; select the project; open Status; assert real status names appear (not To Do/In Progress/Done). (spec scenario 2)
- **Filter narrows list** — check `Ready for review`; assert only matching seeded tickets remain. (spec scenario 3)
- **Disabled without project** — with no project selected, Status pill is disabled with hint. (spec scenario 1)

---

## Implementation Waves

```
Wave 1:
- [x] [task-01-backend-status-endpoint](task-01-backend-status-endpoint.md)

Wave 2:
- [x] [task-02-frontend-types-api](task-02-frontend-types-api.md)
- [x] [task-03-frontend-filter-model](task-03-frontend-filter-model.md)

Wave 3:
- [x] [task-04-frontend-ui-wiring](task-04-frontend-ui-wiring.md)

Wave 4:
- [x] [task-05-e2e](task-05-e2e.md)
```

Frontend tasks are largely sequential (shared build/type surfaces); task-04
depends on 02 and 03. E2E runs last against the mock Jira client.
