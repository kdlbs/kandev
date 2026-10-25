---
id: "01-manifest-sdk-contract"
title: "Manifest and SDK contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/requirements/agent-tools.md"
adr: "../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md"
---

# Task 01: Manifest and SDK contract

## Acceptance

- The manifest and normalized runtime descriptor represent every field and
  limit in the spec, reject invalid schemas/surfaces/names, and derive bounded
  deterministic exposed names.
- The plugin protobuf and Go SDK support one context-respecting
  `InvokeAgentTool` RPC with fallback text, optional structured content, and
  `is_error`; the optional `AgentToolPlugin` interface leaves the base `Plugin`
  interface and existing implementations source-compatible.
- Protobuf stubs are regenerated, not edited manually, and focused manifest and
  SDK round-trip tests pass.

## Verification

```bash
make -C apps/backend proto && cd apps/backend && go test -race ./internal/plugins/manifest ./internal/mcp/plugintools ./pkg/pluginsdk
```

## Files likely touched

- `apps/backend/internal/plugins/manifest/manifest.go`
- `apps/backend/internal/plugins/manifest/validate.go`
- `apps/backend/internal/plugins/manifest/manifest_test.go`
- `apps/backend/internal/mcp/plugintools/types.go`
- `apps/backend/internal/mcp/plugintools/types_test.go`
- `apps/backend/internal/mcp/toolschema/schema.go`
- `apps/backend/internal/mcp/toolschema/schema_test.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/proto/kandev/plugin/v1/plugin.pb.go`
- `apps/backend/proto/kandev/plugin/v1/plugin_grpc.pb.go`
- `apps/backend/pkg/pluginsdk/plugin.go`
- `apps/backend/pkg/pluginsdk/types.go`
- `apps/backend/pkg/pluginsdk/serve.go`
- `apps/backend/pkg/pluginsdk/plugin_test.go`
- `apps/backend/pkg/pluginsdk/serve_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task establishes shared manifest, protobuf, generated, and SDK
contracts consumed by every later task.

## Inputs

- Spec sections `What`, `Manifest Contract`, and `Plugin SDK Contract`
- ADR decisions 1, 4, 8, and 9
- Existing `manifest.Validate`, protobuf `service Plugin`, pluginsdk optional
  interface/adapter patterns, and MCP JSON Schema validation behavior

## Risks

- A protobuf field or SDK signature mistake becomes a public compatibility
  burden. Keep request context backend-authored and result content bounded.
- JSON Schema must have one interpretation at install and invocation time.

## Output contract

Report RED tests, exact public fields and defaults, generated files, commands and
results, compatibility notes, and task/plan status updates.

## Results

- RED: `TestParse_AgentToolDeclarationIsRetained` failed because the existing
  manifest parser had no `AgentTools` field.
- Added manifest declarations and validation for names, surfaces, schemas,
  limits, and annotation consistency; added normalized readable provider-safe
  names and catalog snapshots under `internal/mcp/plugintools`.
- Added shared JSON Schema compilation under `internal/mcp/toolschema`.
- Added optional `pluginsdk.AgentToolPlugin` and gRPC request/response
  conversion. The base `Plugin` interface remains unchanged for compatibility.
- Added `InvokeAgentTool` to the plugin protobuf and regenerated stubs with
  `make proto`.
- Verification: `make proto` (pass); `go test -race ./internal/plugins/manifest ./internal/mcp/toolschema ./internal/mcp/plugintools ./pkg/pluginsdk` (pass); focused SDK round trip `go test -race -run 'TestServe_EndToEnd/InvokeAgentTool' ./pkg/pluginsdk` (pass).
- Remaining risk: backend catalog invocation and agentctl registry are not yet
  wired; task 02 begins that work.
