---
id: "03-http-ws-surface"
title: "Repository set HTTP and WebSocket surface"
status: done
wave: 3
depends_on: ["02-service-and-events"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 03: Repository Set HTTP And WebSocket Surface

## Acceptance

- The five REST routes from the spec are registered and return the documented request/response
  shapes, with typed service errors mapped to `400` / `404` / `409` / `422` and no write on any
  failure. Collection routes use `:id` for the workspace, matching the sibling repository routes.
- The same five operations are reachable over the WebSocket dispatcher as
  `repository_set.list|create|get|update|delete`.
- `route_registration_test.go` pins every new method+path and every new WS action, and handler tests
  drive the routes over an httptest router covering the success path and each error code.

## Verification

```bash
cd apps/backend
go test ./internal/task/handlers/... ./internal/backendapp/...
```

## Files likely touched

- `apps/backend/internal/task/handlers/repository_set_handlers.go` (new, modelled on
  `repository_handlers.go` `RegisterRepositoryRoutes:32`, `registerHTTP:38`, `registerWS:67`)
- `apps/backend/internal/task/handlers/repository_set_handlers_test.go` (new)
- `apps/backend/internal/task/handlers/route_registration_test.go` (helpers at `:12-63`)
- `apps/backend/internal/backendapp/helpers.go` (`registerTaskRoutes`, beside
  `RegisterRepositoryRoutes` at `:1064`)

## Dependencies

Task 02 for the service methods and typed errors.

## Inputs

- Spec: API surface, Permissions, Failure modes.
- Patterns: `repository_handlers.go` for the handler/registration split;
  `repository_handlers_test.go:30` for `newRepositoryHTTPTestRouter`.
- gin rejects two different wildcard names at the same path position inside one group, so the
  workspace param must be `:id` here.

## Risks

- Registering the item route as `/repository-sets/:id` alongside a collection route that also binds
  `:id` is fine, but a mismatched param name would fail at router build time rather than in a test;
  the pinning test must actually exercise route construction.
- Update semantics are replace-not-merge for `repository_ids` but merge for `name`/`description`;
  a handler that treats an absent `repository_ids` as an empty list would wipe memberships.

## Output contract

Summary, files changed, tests run with results, blockers, risks, divergence from the plan, and
task/plan status updates.
