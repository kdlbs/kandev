---
id: "04-host-data-impl"
title: "kandev-side Host data RPC implementation + capability gating + wiring"
status: done
wave: 3
depends_on: ["01-proto-contract", "02-session-loc-aggregation", "03-sdk-data-accessors"]
plan: "plan.md"
spec: "../../../specs/plugins/spec.md"
adr: "../../../decisions/0043-plugin-host-data-api.md"
---

# Task 04: kandev-side Host data RPC implementation

Implement the Go-native Host data interface (from task-03) inside
`internal/plugins`, backed by the service layer, with per-resource capability
gating, and wire the new service dependencies through `plugins.Service`.

## Scope
- Implement each read method on `pluginHost` (or a sibling type in
  `internal/plugins/`) satisfying the extended `pluginsdk.Host` interface:
  1. **Gate:** check `manifest.CanRead("<resource>")`; on miss return
     `permissionDenied("api_read:<resource>")` (extend the existing helper —
     `status.Errorf(codes.PermissionDenied, "capability '%s' not declared", ...)`).
  2. **Service call:** tasks/workspaces/repositories/sessions → `taskSvc`
     methods (e.g. `ListTaskSessions`, workspace + repository listers); workflows/
     steps → workflow service (`ListStepsByWorkflow`, workflow lister); agent
     profiles → agent settings store; code stats → the task-02 aggregation
     service method. Never call a repository directly from the handler.
  3. **Map:** explicit internal model → Go-native DTO mapping (RFC3339 strings,
     optional/NULL handling). Session DTO must carry `acp_session_id` (from
     session metadata / `executors_running` fallback, as the source plugin did).
- Apply the v1 scoping rule: global reads, filters narrow results, ephemeral
  tasks excluded unless requested. Leave a single scoping hook per ADR 0043(a).
- Implement opaque-cursor pagination (server-defined cursor encoding + max cap).
- Write RPC handlers return `codes.Unimplemented`.
- **Wiring:** add the new service deps to `plugins.Service`, `plugins.NewService`,
  and `Provide`; thread them into `hostForPlugin` so each spawned `pluginHost`
  can serve data RPCs. Confirm the new RPCs are served over the existing Host
  broker connection (`runtime/manager.go` spawn path, `registerHostServer`).

## Acceptance
- A plugin declaring `api_read:sessions` gets `ListSessions` +
  `ListSessionCodeStats` results via the Host channel; one without it gets
  `PermissionDenied` naming `api_read:sessions`.
- All read RPCs map internal models to DTOs with no proto/domain-struct leakage;
  write RPCs return `Unimplemented`.
- `make -C apps/backend proto` not required here (done in task-01); package builds
  and tests pass.

## Verification
- `cd apps/backend && go test ./internal/plugins/...`
- `cd apps/backend && go build ./...`

## Files likely touched
- `apps/backend/internal/plugins/host.go`
- `apps/backend/internal/plugins/service.go` (deps, `NewService`, `Provide`,
  `hostForPlugin`)
- possibly a new `apps/backend/internal/plugins/host_data.go`
- `apps/backend/internal/plugins/host_test.go`

## Inputs
- Extended `Host` interface + Go-native DTOs from task-03.
- Aggregation service method from task-02.
- Existing gating pattern: `internal/plugins/host.go` (`permissionDenied`,
  capability checks), `service.go` `hostForPlugin`, `manifest.CanRead`/`CanWrite`.
- Service methods: `internal/task/service/service_sessions.go`,
  `internal/workflow/service/service.go`, `internal/agent/settings/store`.
- Spec: "Host data API"; ADR 0043 (service-layer reads, DTO discipline, scoping).

## Dependencies
Tasks 01, 02, 03.

## Output contract
Summary, per-resource service method used for each RPC, capability-gating result,
wiring changes, `acp_session_id` sourcing note, test result, and status update
here + in `plan.md`.
</content>
