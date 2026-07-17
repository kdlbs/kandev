---
id: "03-sdk-data-accessors"
title: "SDK Go-native data DTOs, Host interface, and author accessors"
status: done
wave: 2
depends_on: ["01-proto-contract"]
plan: "plan.md"
spec: "../../../specs/plugins/spec.md"
adr: "../../../decisions/0043-plugin-host-data-api.md"
---

# Task 03: SDK Go-native data DTOs, Host interface, and author accessors

Extend `pkg/pluginsdk` so authors read kandev data through Go-native typed structs
— proto types never leak past the package boundary, matching the existing
state/secrets pattern in `host.go`/`types.go`.

## Scope
- **Go-native DTOs** in `types.go` mirroring the proto: `Task`, `TaskRepository`,
  `Session`, `SessionCodeStats`, `Workspace`, `Workflow`, `WorkflowStep`,
  `AgentProfile`, `Repository`, plus `Page`/`PageInfo`/filter structs, each with
  `toProto`/`fromProto` helpers alongside the existing ones.
- **Host interface extension** (`host.go`): add data read methods. Prefer grouping
  behind sub-accessors exposed from the `Host` interface — `host.Tasks()`,
  `host.Sessions()`, `host.Workspaces()`, `host.Workflows()`,
  `host.AgentProfiles()`, `host.Repositories()` — each returning a small typed
  accessor (e.g. `TasksAPI.List(ctx, filter, page)`, `TasksAPI.Get(ctx, id)`,
  `SessionsAPI.List(...)`, `SessionsAPI.CodeStats(...)`). Keep write methods off
  the surface this phase (deferred).
- **Client + server conversions**: `grpcHostClient` (plugin side) calls the
  generated data RPCs and converts responses to Go-native; `grpcHostServer`
  (kandev side) dispatches to the Go-native interface. Follow the existing
  `GetState`/`ListState` conversion shape exactly.
- Keep the existing `Host` state/secrets/emit methods intact.

## Acceptance
- Author can call `host.Sessions().List(ctx, filter, page)` and
  `host.Sessions().CodeStats(ctx, filter, page)` (etc.) receiving Go-native
  structs; proto types are not exposed.
- `grpcHostServer` satisfies the generated server interface for the new RPCs;
  `grpcHostClient` satisfies the extended `Host` interface.
- Round-trip conversion unit tests pass.

## Verification
- `cd apps/backend && go test ./pkg/pluginsdk/...`
- `cd apps/backend && go build ./...`

## Files likely touched
- `apps/backend/pkg/pluginsdk/types.go`
- `apps/backend/pkg/pluginsdk/host.go`
- `apps/backend/pkg/pluginsdk/*_test.go`

## Inputs
- Generated stubs from task-01.
- Existing pattern: `pkg/pluginsdk/host.go` (grpcHostClient/grpcHostServer),
  `types.go` (toProto/fromProto, `mapToStruct`/`structToMap`).
- Spec: "Host data API" resource/capability table.

## Dependencies
Task 01 (needs regenerated proto stubs).

## Output contract
Summary, the accessor shape chosen (sub-accessors vs flat methods), files changed,
test result, and status update here + in `plan.md`.
</content>
