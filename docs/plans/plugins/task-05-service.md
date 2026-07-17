---
id: task-05
title: Registry + service + lifecycle state machine + Provide
status: pending
wave: 2
depends_on: [task-01, task-02, task-03]
plan: docs/plans/plugins/plan.md
---

# Registry + service + lifecycle state machine + Provide

## Title
The core plugin `Service`: in-memory registry loaded from the FS store, lifecycle
state machine, and a `Provide(...)` constructor following the repo provider pattern.

## Inputs
- Spec `docs/specs/plugins/spec.md` → "State machine" (registered→active→
  disabled→uninstalled, error branch), "Permissions", registration flow.
- Depends on: `internal/plugins/manifest` (task-01), `internal/plugins/store`
  (task-02), `internal/plugins/state` (task-03).
- Provider pattern reference: `internal/jira/provider.go` `Provide(...) (*Service, cleanup, error)`
  and how `internal/backendapp/services.go` `initJiraService` calls it.

## Acceptance
1. `internal/plugins/registry.go`: `Registry` wrapping `map[string]*store.Record`
   with mutex; `Load()` from FS store, `Get`, `List`, `Add`, `Remove`, `SetStatus`.
2. `internal/plugins/service.go`: `Service` with methods `Register(manifest) (*store.Record, store.Credentials, error)`,
   `List()`, `Get(id)`, `UpdateConfig(id, map)`, `Uninstall(id)`, `Enable(id)`,
   `Disable(id)`, `SetStatus(id, status)`, `AuthenticatePlugin(apiKey) (*store.Record, error)`.
   Status transitions enforce the spec state machine (reject invalid transitions
   with a typed error).
3. `internal/plugins/provider.go`: `Provide(cfg, stateDB, secretAdapter, eventBus, log) (*Service, func() error, error)`
   constructing FS store (dir `~/.kandev/plugins`, overridable via cfg/env for tests),
   state store, registry (Load on startup). Health poller + delivery are wired in
   later tasks — expose setters/fields so task-06/07 can attach.
4. Status enum + `Status` type in `internal/plugins/types.go`.

## Files
- `apps/backend/internal/plugins/types.go`
- `apps/backend/internal/plugins/registry.go` + `registry_test.go`
- `apps/backend/internal/plugins/service.go` + `service_test.go`
- `apps/backend/internal/plugins/provider.go` + `provider_test.go`

## Verification
- `go test ./internal/plugins/...` from `apps/backend`
- `make -C apps/backend lint`

## Output contract
Report: state-machine transition table implemented, Provide signature (so wiring
task-09 matches), extension points left for delivery/health. Do not edit
manifest/store/state packages (import only).

## Dependencies
task-01, task-02, task-03.
