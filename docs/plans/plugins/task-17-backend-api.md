---
id: task-17
title: Backend HTTP API + proxies + bundle + boot payload + health + wiring
status: done
wave: B
depends_on: [task-05, task-06, task-16, task-10]
plan: docs/plans/plugins/plan.md
---

# Backend HTTP API + proxies + bundle + boot payload + health + wiring

## Title
The full backend surface for native-UI plugins: management + plugin-facing API,
auth/capability middleware, state + secrets, webhook proxy, **JS bundle proxy**,
boot-payload active-plugin list, health monitor, and backendapp wiring.

## Inputs
- Spec `docs/specs/plugins/spec.md` (API surface, permissions, security).
- `PLUGIN-API.md`: boot payload `plugins: [{id,name,bundleUrl,styleUrls?}]`,
  `bundleUrl=/api/plugins/{id}/bundle`; bundle proxied to plugin's `ui.bundle`.
- Reuse: task-05 `Service` (Provide, Registry(), Get, List, Enable/Disable,
  AuthenticatePlugin, RevealWebhookSecret, StateStore(), SetDeliverer,
  SetStatus/UpdateLastHealthCheck), task-06 `delivery.Deliverer`, task-16 manifest
  `ui.bundle`.
- Routing precedent: `internal/jira/handlers.go` `RegisterRoutes(router, dispatcher,
  svc, log)`. Reverse-proxy + header stripping precedent:
  `internal/agentctl/server/api/` (read its AGENTS.md).
- Wiring precedent: `internal/backendapp/services.go` `initJiraService` +
  `provideServices`; `helpers.go` `routeParams` + `registerRoutes`. Boot payload
  producer: `internal/webapp/payload.go` (`BootPayload`) — add active plugins.

## Acceptance
1. `internal/plugins/handlers.go` `RegisterRoutes(router, svc, deliverer, log)`:
   - Management: `POST /api/plugins/register`, `GET /api/plugins`, `GET/:id`,
     `PATCH/:id`, `DELETE/:id`, `POST/:id/enable`, `POST/:id/disable`. Register
     returns cleartext creds once.
   - `GET /api/plugins/tools` (aggregate declared tools; listing only).
   - `GET /api/plugins/:id/bundle` + `GET /api/plugins/:id/ui/*` → reverse proxy to
     plugin base_url (bundle → `ui.bundle` path). Strip iframe-blocking headers on
     `ui/*`; bundle served with `Content-Type: text/javascript`. Only for active plugins.
   - `POST /api/plugins/:id/webhooks/:key` → webhook reverse proxy (active + key declared).
2. `internal/plugins/client.go`: `InvokeTool(ctx, record, tool, input, callCtx)`
   (30s) + `Health(ctx, record)`; `internal/plugins/proxy.go`: the reverse proxies.
3. `internal/plugins/middleware.go`: `PluginAuth(svc)` (Bearer api_key → plugin
   identity) + `RequireCapability(kind, resource)` (403 with capability name).
   Plugin-facing: state API (`/api/plugins/:id/state*`, needs `state` cap, always
   scoped to authed plugin id), secrets API (`/api/plugins/:id/secrets/:ref`, needs
   `secrets` cap → secretadapter). Office read/write endpoints deferred (comment).
4. `internal/plugins/health.go`: `HealthMonitor` 30s/5s, 3 fails → error + emit
   `plugin.health.degraded` bus event, recovery → active + `deliverer.Flush(id)`,
   updates `last_health_check`.
5. Boot payload: `internal/webapp/payload.go` gains `Plugins []ActivePluginPayload`
   (`{ID,Name,BundleURL,StyleURLs}`) populated from active plugins that have
   `ui.bundle`. Gate on `features.plugins`. Add the mapping wherever office data is
   mapped into the payload; add a getter on the plugins Service like
   `ActiveUIPlugins() []Record`.
6. Wiring: `initPluginsService` in services.go calling `plugins.Provide(...)` with
   `secretadapter.New(secretsStore)` as the vault; construct `delivery.Deliverer`,
   `svc.SetDeliverer(d)`, start `HealthMonitor` and deliverer via `addCleanup`;
   fields on `Services` + `routeParams`; `registerRoutes` calls
   `plugins.RegisterRoutes(...)` gated on `p.features.Plugins`.

## Files
- `apps/backend/internal/plugins/handlers.go`, `middleware.go`, `client.go`,
  `proxy.go`, `health.go`, `dto.go` (+ `_test.go` each)
- edits: `internal/backendapp/services.go`, `internal/backendapp/helpers.go`,
  `internal/webapp/payload.go`

## Verification
- `go test ./internal/plugins/... ./internal/backendapp/... ./internal/webapp/...` (apps/backend)
- `make -C apps/backend build lint`

## Output contract
Full route table, proxy header behavior, boot-payload shape (so task-18/19 match),
wiring points. Ensure `make -C apps/backend build test lint` pass. This closes the backend.

## Dependencies
task-05 (done), task-06, task-16, task-10.
