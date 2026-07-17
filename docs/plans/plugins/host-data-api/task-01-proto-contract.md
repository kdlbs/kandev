---
id: "01-proto-contract"
title: "Host data proto contract + regenerated stubs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../../specs/plugins/spec.md"
adr: "../../../decisions/0043-plugin-host-data-api.md"
---

# Task 01: Host data proto contract + regenerated stubs

Fold the Host data RPCs and DTOs from `docs/plans/plugins/HOST-DATA-API.proto`
into the frozen plugin proto and regenerate Go stubs. This is the contract every
later task compiles against.

## Scope
- Read RPCs: `ListTasks`, `GetTask`, `ListWorkspaces`, `ListWorkflows`,
  `ListWorkflowSteps`, `ListAgentProfiles`, `ListRepositories`, `ListSessions`,
  `ListSessionCodeStats`.
- Deferred write RPCs (declared in proto, not implemented later this phase):
  `CreateTask`, `UpdateTask`, `CreateComment`.
- DTOs + `Page`/`PageInfo`/filter messages from the draft.
- Decide placement: add RPCs to the existing `service Host` (recommended — reuses
  the single broker connection and the capability interceptor) rather than a new
  `service HostData`. Record the choice in a proto comment.
- Verify DTO fields against the real shapes in
  `apps/backend/pkg/api/v1/{task,workflow,agent,workspace}.go`; keep RFC3339
  string timestamps and `optional` nullables per ADR 0043.

## Acceptance
- `apps/backend/proto/kandev/plugin/v1/plugin.proto` contains the read + deferred
  write RPCs and DTOs; existing `Host`/`Plugin` RPCs are unchanged.
- `make -C apps/backend proto` regenerates `plugin.pb.go` / `plugin_grpc.pb.go`
  with no manual edits, and the tree builds.

## Verification
- `make -C apps/backend proto`
- `cd apps/backend && go build ./...`

## Files likely touched
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/proto/kandev/plugin/v1/plugin.pb.go` (generated)
- `apps/backend/proto/kandev/plugin/v1/plugin_grpc.pb.go` (generated)

## Inputs
- Spec: "Host data API" section, capability table.
- Draft: `docs/plans/plugins/HOST-DATA-API.proto`.
- DTO shapes: `apps/backend/pkg/api/v1/{task,workflow,agent,workspace}.go`.
- ADR 0043 (DTO discipline, conventions).

## Dependencies
None.

## Output contract
Summary of RPC placement decision, files changed (incl. regenerated stubs),
`make proto` + build result, any DTO field deviations from the draft, and status
update here + in `plan.md`.
</content>
