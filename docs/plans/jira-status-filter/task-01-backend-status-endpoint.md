---
id: "01-backend-status-endpoint"
title: "Backend: list project statuses endpoint"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/jira-status-filter/spec.md"
---

# Task 01: Backend list-project-statuses endpoint

Add the read path that returns a project's real Jira statuses.

## Acceptance
- New `JiraStatus{ID,Name,StatusCategory}` type and a `Client.ListProjectStatuses(ctx, key)` method implemented on `CloudClient` (calls `GET {apiBase}/project/{key}/statuses`, flattens per-issue-type statuses, de-dupes by id, maps `statusCategory.key`).
- `MockClient` supports seeding statuses and a mock control route seeds them; `Service.ListProjectStatuses` and handler `GET /api/v1/jira/projects/:key/statuses` return `{ "statuses": [...] }`.
- Not-configured returns `503` `JIRA_NOT_CONFIGURED`; upstream 401/403/404 pass through via `writeClientError`.

## Verification
- `cd apps/backend && go test ./internal/jira/...`
- `make -C apps/backend lint`

## Files likely touched
- `apps/backend/internal/jira/models.go`
- `apps/backend/internal/jira/client.go`
- `apps/backend/internal/jira/cloud_client.go`
- `apps/backend/internal/jira/mock_client.go`
- `apps/backend/internal/jira/mock_controller.go`
- `apps/backend/internal/jira/service.go`
- `apps/backend/internal/jira/handlers.go`
- Tests: `cloud_client_test.go`, `handlers_test.go`, `service_test.go`

## Inputs
- Spec: Data model, API surface, Failure modes.
- Plan: Backend section.
- Patterns: mirror `ListProjects`/`httpListProjects` and the `do()` helper; mock route pattern in `mock_controller.go`.

## Dependencies
None.

## Output contract
Report: summary, files changed, tests run + results, blockers, risks, and set `status: done` + tick the plan checkbox.
