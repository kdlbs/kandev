---
id: "03-agentctl-dynamic-registry"
title: "Agentctl dynamic MCP registry"
status: done
wave: 2
depends_on: ["01-manifest-sdk-contract"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/agent-tools.md"
adr: "../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md"
---

# Task 03: Agentctl dynamic MCP registry

## Acceptance

- Typed MCP profiles carry plugin catalog snapshots and agentctl registers only
  declarations matching Kanban or Office surfaces with exact schemas,
  annotations, and bound proxy handlers.
- Live control accepts new generations/newer revisions, ignores stale or
  equivalent snapshots, replaces tools atomically, and emits exactly one
  `tools/list_changed` notification per effective registry change.
- Dynamic calls pass only arguments plus agentctl-bound task/session identity to
  the backend; malformed descriptors fail without replacing the last valid
  registry.

## Verification

```bash
cd apps/backend && go test -race ./internal/mcp/profile ./internal/mcp/server ./internal/agentctl/server/api ./internal/agent/runtime/agentctl
```

## Files likely touched

- `apps/backend/internal/mcp/profile/profile.go`
- `apps/backend/internal/mcp/profile/profile_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/plugin_tools_test.go`
- `apps/backend/internal/mcp/server/tool_argument_validation.go`
- `apps/backend/internal/mcp/toolschema/schema.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/mcp_plugin_tools_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/agent/runtime/agentctl/client_mcp_plugin_tools_test.go`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 02 after Task 01. This task owns MCP server and agentctl
transport files; Task 02 owns plugin service/catalog files.

## Inputs

- Spec sections `What` and `Runtime Catalog`
- ADR decisions 2, 3, 5, and 6
- Existing `SetProfile`, `SetProviders`, isolated `assembleTools`, shared schema
  compiler from Task 01, and initialized-session notification tests

## Risks

- Registry replacement must preserve built-in surface/capability/provider tools.
- Some MCP clients ignore list-changed; server correctness must not depend on a
  client refresh.

## Output contract

Report RED notification/stale-revision evidence, registry composition rules,
wire payload, commands and results, and task/plan status updates.

## Results

- Added atomic `SetPluginTools` replacement, exact raw-schema registration,
  MCP annotations, proxy handlers, and lazy catalog synchronization before
  `tools/list` so changes apply without restarting agentctl.
- Replacements use mcp-go `SetTools`, which emits one `tools/list_changed`
  notification for initialized clients. Same-generation stale revisions are
  ignored.
- Verification: `go test -race ./internal/mcp/profile ./internal/mcp/server
  ./internal/agentctl/server/api ./internal/agent/runtime/agentctl` passed.
