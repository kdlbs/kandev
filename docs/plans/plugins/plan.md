# Plan: Office Plugin System (Phase 1 + UI iframe proxy)

**Spec:** `docs/specs/plugins/spec.md`
**Status:** in progress
**Owner:** jcfs

## Goal

Implement the plugin system end-to-end for an operator to register an
out-of-process HTTP plugin, have kandev deliver signed events to it, invoke its
tools, proxy external webhooks and iframe UI pages to it, expose plugin-scoped KV
state + secrets, monitor health, and manage it from a Plugins settings page —
all gated behind a new `plugins` feature flag.

## In scope

- Backend `internal/plugins/` package (provider pattern): manifest types +
  validation, filesystem registration store, in-memory registry, lifecycle state
  machine, health poller, event delivery (HMAC + retries + ring buffer), tool
  invocation client, webhook + UI reverse proxies, `plugin_state` SQLite store,
  auth middleware + capability checks, secrets resolution.
- `/api/plugins/*` management + plugin-facing API.
- `plugins` feature flag (profiles.yaml, `FeaturesConfig`, runtimeflags registry,
  frontend `FeatureFlags`).
- Frontend Plugins settings page + plugin detail with sandboxed iframe pages.
- Fixture plugin binary for tests/e2e.
- Playwright e2e.

## SCOPE REVISION — option C (native JS UI plugins)

User chose the full Mattermost-webapp model: plugins ship JS bundles loaded into
the running SPA that register **native** routes/nav/components/reducers (goal: a
plugin can implement `/jira` as a native page). Iframe pages are dropped in favor
of this. The frozen frontend contract is `docs/plans/plugins/PLUGIN-API.md`.
Backend foundations (Wave 1 + task-05 service) are unchanged and reused. Waves
below are superseded by the "Option C waves" section.

Still out of scope: marketplace/index, managed process runtime (kandev spawning
plugins), wiring plugin tools into live agent sessions (client + listing only),
rate limiting, plugin DB namespaces, hot reload, multi-instance, and plugin-JS
sandboxing (workers/realms — documented as future work; v1 loads only active
operator-registered plugins over same-origin proxy).

## Architecture anchors (verified)

- HTTP: gin; `registerRoutes(routeParams)` in
  `apps/backend/internal/backendapp/helpers.go:478`; domain groups follow
  `jira.RegisterRoutes(router, dispatcher, svc, log)` → `router.Group("/api/v1/jira")`.
- Service wiring: `apps/backend/internal/backendapp/services.go` `provideServices(...)`
  with `init<Name>Service(...)` helpers calling per-package `Provide(...)`; result
  fields land on `Services` and `routeParams`.
- Event bus: `internal/events/bus.EventBus` (`Subscribe(subject, handler) (Subscription, error)`,
  wildcard subjects supported); model delivery after
  `internal/gateway/websocket/task_notifications.go` `RegisterTaskNotifications`.
  Event names in `internal/events/types.go` (`events.TaskCreated="task.created"`, etc).
- Health: `internal/integrations/healthpoll` `Prober`/`Poller` (interval configurable).
- Secrets: `internal/secrets` + `internal/integrations/secretadapter.New(store)`.
- Persistence: SQLite; per-package `initSchema()` (see
  `internal/task/repository/sqlite/base_schema.go`). No central migrations.
- UI proxy precedent: `internal/agentctl/server/api/` reverse proxy (header stripping).
- Feature flag pattern: `profiles.yaml` `features:` block →
  `internal/common/config/config.go` `FeaturesConfig` (`config.go:174`) →
  `internal/runtimeflags/registry.go` + `runtimeflags/config.go:31` →
  frontend `apps/web/lib/state/slices/features/types.ts` + `hooks/domains/features/use-feature.ts`.

## Waves

### Wave 1 — backend foundations + fixture (parallel; disjoint files)
- **task-01** manifest types + validation (`internal/plugins/manifest/`)
- **task-02** filesystem registration store + credentials (`internal/plugins/store/`)
- **task-03** `plugin_state` SQLite store (`internal/plugins/state/`)
- **task-04** fixture plugin binary (`apps/backend/cmd/plugin-fixture/`)

### Wave 2 — service, delivery, API, proxies (sequential within package)
- **task-05** registry + service + lifecycle state machine + `Provide` (`internal/plugins/service.go`, `registry.go`, `provider.go`)
- **task-06** event delivery subscriber (HMAC, retries, ring buffer) (`internal/plugins/delivery/`)
- **task-07** health poller integration (`internal/plugins/health.go`)
- **task-08** tool invocation client + webhook & UI reverse proxies (`internal/plugins/client.go`, `proxy.go`)
- **task-09** HTTP API + auth middleware + capability checks + backendapp wiring (`internal/plugins/handlers.go`, `middleware.go`; edits to `backendapp/services.go`, `helpers.go`)

### Wave 3 — feature flag + frontend
- **task-10** `plugins` feature flag (backend + frontend plumbing)
- **task-11** plugins API client + Zustand slice (+ unit tests)
- **task-12** Plugins settings page (list/register/enable/disable/uninstall) + route
- **task-13** plugin detail with sandboxed iframe pages + postMessage bridge

### Wave 4 — e2e + verify
- **task-14** Playwright e2e (register→active, enable/disable, iframe renders, event delivery)
- **task-15** spec update + docs + full `make fmt typecheck test lint` + web typecheck/lint

## Option C waves (ACTIVE)

Contract: `PLUGIN-API.md`. task-12/13 (iframe) superseded by task-19/20.

### Wave A — foundations (parallel; disjoint)
- **task-06** event delivery (unchanged) — `internal/plugins/delivery/`
- **task-16** manifest `ui.bundle`/`ui.styles` fields — `internal/plugins/manifest/`
- **task-10** `plugins` feature flag (backend+frontend plumbing)
- **task-18** frontend Plugin API: globals + registry + host/loader — `apps/web/lib/plugins/`

### Wave B — integration (after A)
- **task-17** backend API + auth/caps + state/secrets + webhook proxy + bundle proxy +
  boot-payload `plugins` + health + backendapp wiring (merges old 07/08/09) —
  needs task-06, task-16, task-10
- **task-19** frontend dynamic integration: routes/nav/slots/ws bridge + PluginSlot —
  needs task-18
- **task-20** plugins management page (list/register/enable/disable) + API client +
  slice — needs task-18, task-17

### Wave C — example + e2e + verify (after B)
- **task-21** example plugin repo (git init; backend + JS bundle + manifest + README) —
  needs PLUGIN-API.md, task-16
- **task-22** Playwright e2e loading the example plugin: native nav item + page +
  slot + WS-driven update — needs task-17, task-19, task-20, task-21
- **task-23** spec + docs update + full verify — needs all

## gRPC transport waves (ACTIVE)

Contract: `GRPC-CONTRACT.md` (frozen). Supersedes the HTTP+HMAC backend transport
from Wave 2 / Option C Wave B's task-17: the plugin **backend** now speaks gRPC over
a kandev-spawned `hashicorp/go-plugin` subprocess instead of HTTP with
API-key/HMAC-signed webhooks. Registration-by-manifest-paste
(`POST /api/plugins/register`, generated `api_key`/`webhook_secret`) is removed
outright — replaced by install-by-URL/upload of a release tarball. task-06
(event delivery semantics), task-16 (manifest `ui.bundle`/`ui.styles`), task-10
(feature flag), and all of Wave A's frontend plugin-host work (task-18/19/20's UI
layer) carry over unchanged; only the backend transport and the install flow change.

- **G1 — proto + SDK.** Author `apps/backend/proto/kandev/plugin/v1/plugin.proto`
  (`Plugin`/`Host` services, all messages per GRPC-CONTRACT.md §3), generate Go
  stubs, and build `apps/backend/pkg/pluginsdk` (§4: `Plugin`/`Host` interfaces,
  `Serve(p Plugin, opts...)`, `UnimplementedPlugin`). No transport wiring yet.
- **G2 — installer package.** Package format + install pipeline (§6):
  checksum verification, optional ed25519 signature check (unsigned → warn),
  manifest validation before any code runs, extraction to
  `~/.kandev/plugins/<id>/<version>/`, and the on-disk install record
  (id/version/install_path/status). No spawning yet.
- **G3 — runtime manager + host service + transport swap + install API + wiring.**
  go-plugin process manager (spawn, handshake §2, `Ping`-based health, crash/backoff
  restart with the state machine in the spec), the `Host` gRPC service
  implementation (GetState/SetState/DeleteState/ListState/RevealSecret/EmitEvent)
  with the capability-gating server interceptor, swap event delivery / tool
  invocation / webhook proxy from HTTP calls to `DeliverEvent`/`InvokeTool`/
  `HandleWebhook` gRPC calls, `POST /api/plugins/install` handler (JSON `{url}` or
  multipart `package`), removal of `POST /api/plugins/register` and all
  api_key/webhook_secret/HMAC code paths, and `backendapp` wiring.
- **G4 — fixture + hello rewrite + frontend install UI.** Rewrite
  `apps/backend/cmd/plugin-fixture/` as a `pluginsdk`-based Go binary (drop the
  HTTP fixture server); rewrite the `kandev-plugin-hello` example repo's backend
  half the same way (frontend bundle unchanged per PLUGIN-API.md §7); replace the
  frontend "Register plugin" flow (manifest paste, credential display) with an
  "Install plugin" flow (URL input + file upload, no credentials shown) per
  PLUGIN-API.md's updated "Loading model"/"Security posture".
- **G5 — e2e + verify.** Playwright coverage for install-from-url, install-by-upload,
  crash → restart → recovery (buffered event flush), and capability-denied
  (gRPC `PermissionDenied`) scenarios; full `make fmt typecheck test lint` plus web
  typecheck/lint/test.

## Verification

- Backend: `make -C apps/backend test`, `make -C apps/backend lint`, `make -C apps/backend build`
- Web: `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test`
- E2E: `cd apps/web && pnpm e2e -g "plugin"`
- Final: `make -C apps/backend fmt` then `make -C apps/backend typecheck test lint`

## Risks / open questions

- Event `payload` shape: bus events carry `event.Data` as struct or
  `map[string]interface{}`. Delivery must normalize to the spec's JSON envelope;
  task-06 defines a `toEnvelope` that marshals `event.Data` under `payload`.
- `workspace_id` is not present on every event; envelope sets it when derivable,
  else omits. Acceptable for Phase 1.
- Sequential-per-plugin delivery must not block the bus goroutine; task-06 uses a
  per-plugin queue + worker.
- Feature-flag default must keep prod OFF; e2e/dev ON so tests run.

**Status 2026-07-16:** all gRPC waves (G1–G5) completed. E2E green (2 specs, 3x repeat no flake); full backend+web gate green. One integration bug found by e2e and fixed (install response unwrapping in plugins-api.ts).
