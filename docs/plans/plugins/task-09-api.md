---
id: task-09
title: HTTP API + auth middleware + capability checks + backendapp wiring
status: pending
wave: 2
depends_on: [task-05, task-06, task-07, task-08]
plan: docs/plans/plugins/plan.md
---

# HTTP API + auth middleware + capability checks + backendapp wiring

## Title
Expose `/api/plugins/*` (gin), the plugin-facing auth middleware + capability
gating, state/secrets APIs, webhook/UI proxy routes, tool-listing, and wire the
whole package into backendapp.

## Inputs
- Spec `docs/specs/plugins/spec.md` → "API surface" (all endpoints), "Permissions",
  "Security".
- Routing precedent: `internal/jira/handlers.go` `RegisterRoutes(router, dispatcher,
  svc, log)` → `router.Group("/api/...")`. Wiring precedent:
  `internal/backendapp/services.go` `initJiraService` + `provideServices`, and
  `internal/backendapp/helpers.go` `routeParams` (add a `pluginsSvc *plugins.Service`
  field) + `registerRoutes` (call `plugins.RegisterRoutes(...)`), gated on
  `p.features.Plugins` (task-10 adds the flag).

## Acceptance
1. `internal/plugins/handlers.go` `RegisterRoutes(router, svc, log)`:
   - Management (operator): `POST /api/plugins/register`, `GET /api/plugins`,
     `GET /api/plugins/:id`, `PATCH /api/plugins/:id`, `DELETE /api/plugins/:id`,
     `POST /api/plugins/:id/enable`, `POST /api/plugins/:id/disable`.
     Register returns cleartext creds once. List/Get never expose creds.
   - `GET /api/plugins/tools` → aggregate declared tools across active plugins
     (id, name, display_name, description, input_schema). (Listing only — not wired
     into agent sessions.)
   - `GET /api/plugins/:id/ui/*path` and `POST` → UIProxy (task-08). Public to the
     browser session (operator UI); no plugin api-key required.
   - `POST /api/plugins/:id/webhooks/:key` → WebhookProxy (task-08). External;
     validates plugin active + key declared.
2. `internal/plugins/middleware.go`: `PluginAuth(svc)` gin middleware — parses
   `Authorization: Bearer <api_key>`, resolves plugin via `svc.AuthenticatePlugin`,
   sets plugin in context. `RequireCapability(kind, resource)` returns
   `403 {"error":"capability '<kind>:<resource>' not declared"}` when the plugin's
   manifest lacks it. Apply to plugin-facing routes:
   - State API: `GET/POST/DELETE /api/plugins/:id/state`, `GET .../state/list`
     (requires `capabilities.state`), backed by task-03 store, always scoped to the
     authenticated plugin id (a plugin cannot touch another's id → 403 if mismatch).
   - Secrets API: `GET /api/plugins/:id/secrets/:ref` (requires `capabilities.secrets`)
     → resolve via secretadapter.
   - Office write-back used by plugins is OUT OF SCOPE here (api_read/api_write
     office endpoints deferred) — implement state + secrets only; leave a comment
     noting office data endpoints are follow-up.
3. `internal/backendapp` wiring: `initPluginsService(...)` in services.go calling
   `plugins.Provide(...)`; field on `Services` + `routeParams`; `registerRoutes`
   calls `plugins.RegisterRoutes` and starts health monitor + delivery via the
   returned service (respect `addCleanup`). Gate registration on `features.Plugins`.

## Files
- `apps/backend/internal/plugins/handlers.go` + `handlers_test.go`
- `apps/backend/internal/plugins/middleware.go` + `middleware_test.go`
- `apps/backend/internal/plugins/dto.go` (request/response DTOs)
- edits: `apps/backend/internal/backendapp/services.go`,
  `apps/backend/internal/backendapp/helpers.go`

## Verification
- `go test ./internal/plugins/... ./internal/backendapp/...` from `apps/backend`
- `make -C apps/backend build` and `make -C apps/backend lint`

## Output contract
Report: full route table, middleware/capability behavior, wiring points touched in
backendapp, and confirmation the feature gate is respected. This is the last
backend task — ensure `make -C apps/backend build test lint` pass for the package.

## Dependencies
task-05, task-06, task-07, task-08. (Feature flag field from task-10 — if task-10
hasn't landed, add the `Plugins bool` field to FeaturesConfig yourself and note it.)
