---
spec: docs/specs/plugins/requirements/agent-tools.md
created: 2026-08-11
status: in_progress
---

# Implementation Plan: Plugin-Contributed Agent Tools

## Overview

Add a bounded manifest and SDK contract first, then implement the authoritative
plugin catalog and gRPC invocation path. Extend the typed MCP profile with
dynamic plugin descriptors, teach agentctl to atomically register proxy tools,
and wire launch plus live catalog refresh through the existing runtime. Finish
with an end-to-end managed-plugin fixture and public authoring documentation.

---

## Backend

### Manifest, transport, and SDK contracts

- Extend `apps/backend/internal/plugins/manifest/manifest.go` with
  `AgentTools []AgentTool`, `AgentToolAnnotations`, surface constants, and the
  exact YAML/JSON fields from the spec.
- Extend `apps/backend/internal/plugins/manifest/validate.go` with per-plugin
  count, name, surface, description, exposed-name length, annotation, schema
  size, object-root, and JSON Schema compilation checks. Extract the reusable
  compiler from `internal/mcp/server/tool_argument_validation.go` into
  `internal/mcp/toolschema` so install-time and invocation-time validation have
  one interpretation.
- Derive readable exposed names in `internal/mcp/plugintools` from a slugged
  plugin ID plus the validated local name. Keep the complete canonical name
  compatible with restrictive agent tool-name grammars, using a short stable
  hash suffix only when the readable form exceeds the length limit.
- Add `apps/backend/internal/mcp/plugintools/types.go` for the normalized runtime
  descriptor and catalog snapshot (`generation`, `revision`, sorted tools).
  Keep this package independent of plugin storage and agentctl.
- Add `InvokeAgentTool` plus request, bound context, and result messages to
  `apps/backend/proto/kandev/plugin/v1/plugin.proto`; regenerate protobuf and
  gRPC stubs with `make -C apps/backend proto`.
- Extend `apps/backend/pkg/pluginsdk/plugin.go`, `types.go`, and `serve.go` with
  an optional `AgentToolPlugin` interface. Keep the base `Plugin` interface
  unchanged, make the gRPC adapter return `Unimplemented` when the optional
  interface is absent, and expose a thin context-respecting `RemotePlugin`
  client.

### Authoritative catalog and invocation

- Add catalog derivation to `apps/backend/internal/plugins/service.go` (or a
  focused `agent_tools.go`): return a canonical snapshot from installed records,
  include only `StatusActive`, reserve names across installed declarations, and
  increment a process-generation/revision identity only when effective content
  changes.
- Validate cross-plugin exposed-name collisions before install/upgrade commits
  in the existing service transaction path so a failed replacement preserves
  the previous version and catalog.
- Add `Service.InvokeAgentTool(ctx, pluginID, localName, arguments, boundContext)`
  beside `InvokeWebhook`. Re-read the current record, require `StatusActive`,
  find the exact declaration, validate surface/input, enforce the 30-second
  deadline and 1 MiB result limit, call `RemotePlugin.InvokeAgentTool` once, and
  validate output before returning a transport-neutral result.
- Publish a nonblocking catalog-change signal after install, upgrade, enable,
  disable, error, recovery, and uninstall persistence succeeds. Do not invoke
  refresh callbacks while the plugin service mutex or runtime supervision
  callback is blocked.
- Add structured invocation logs containing identifiers, duration, and outcome
  only; never log arguments or results.

### Dynamic MCP registry in agentctl

- Extend `internal/mcp/profile.Context` with the canonical plugin-tool snapshot;
  normalization deep-copies and sorts descriptors and includes them in profile
  equality.
- Add `Server.SetPluginTools(snapshot)` and incorporate a `plugin-tools` group
  into `profileToolGroups`. Filter definitions to `kanban-task` and
  `office-task`, construct tools with `mcp.NewToolWithRawSchema`, attach output
  schema and annotations, and bind each handler closure to plugin ID/local name.
- The dynamic handler forwards only arguments plus server-bound task/session
  identity through a new `mcp.invoke_plugin_tool` WebSocket action. It maps the
  backend's fallback text, structured content, and `is_error` response to one
  `mcp.CallToolResult`.
- Add `PUT /api/v1/mcp/plugin-tools` and the matching runtime agentctl client.
  Accept a new generation, ignore stale same-generation revisions, skip
  equivalent effective registries, and rely on one `SetTools` call to emit one
  list-changed notification.
- Preserve existing shared top-level argument validation for dynamic schemas.
  A malformed runtime descriptor must fail closed without replacing the last
  valid registry.

### Backend dispatch and lifecycle propagation

- Add `ActionMCPInvokePluginTool` to `apps/backend/pkg/websocket/actions.go` and
  register a handler in `apps/backend/internal/mcp/handlers` with a narrow
  plugin-tool service interface.
- The handler takes plugin/tool identity from the agentctl closure, task/session
  identity from the bound MCP server payload, and workspace/surface from
  persisted backend state. Apply `mcp/scope` identity and task-session pair
  authorization before plugin lookup; repeat schema and declaration checks in
  the plugin service.
- Inject the current catalog snapshot when
  `orchestrator/executor.resolveTaskSessionMCPProfile` builds launch/resume
  profiles. Keep configuration and external surfaces empty.
- Add a lifecycle method that replaces plugin tools on every live agentctl
  execution. Wire a backendapp catalog refresher/coalescer to the plugin service
  change signal; serialize snapshots, continue past per-execution failures, and
  leave persisted plugin state authoritative.
- Ensure launch/resume and the next live change repair a failed refresh. Do not
  restart an agent solely because a client may ignore list-changed.

### Managed fixture and full-path proof

- Extend the runtime fixture plugin to declare and implement an echo/context
  agent tool and a controlled error/timeout case.
- Add an integration test that starts the managed fixture, creates the task MCP
  server over the real backend request bridge, lists the dynamic tool, invokes
  it, and verifies bound context and result mapping.
- Cover live enable/disable or active/error/recovery replacement with an
  initialized MCP client and assert exactly one list-changed notification per
  effective change, no notification for a stale/equivalent snapshot, and
  fail-closed invocation after removal.

---

## Tests

- **What:** Manifest limits, surface rules, schema compilation, conservative
  annotation defaults, deterministic names, and collision rejection.
  **File:** `internal/plugins/manifest/manifest_test.go`,
  `internal/plugins/service_test.go`. **How:** table-driven unit tests plus a
  failed-upgrade preservation test.
- **What:** SDK/protobuf invocation conversion and source-compatible
  `UnimplementedPlugin`. **File:** `pkg/pluginsdk/plugin_test.go`,
  `pkg/pluginsdk/serve_test.go`. **How:** in-memory go-plugin gRPC round trip.
- **What:** Active-only canonical catalog revisions and bounded, non-retried
  invocation with schema/result enforcement. **File:**
  `internal/plugins/agent_tools_test.go`. **How:** fake runtime plus controlled
  remote plugin tests.
- **What:** Surface-filtered atomic registry replacement, annotations, stale
  revision rejection, schema enforcement, and one list-changed notification.
  **File:** `internal/mcp/server/plugin_tools_test.go`. **How:** initialized
  mcp-go session with a recording backend client.
- **What:** Agentctl live control transport preserves built-in surface,
  capabilities, and providers while replacing plugin descriptors. **File:**
  `internal/agentctl/server/api/mcp_plugin_tools_test.go`,
  `internal/agent/runtime/agentctl/client_mcp_plugin_tools_test.go`. **How:** HTTP
  handler/client contract tests.
- **What:** Backend invocation rejects stale discovery, mismatched task/session,
  unsupported surface, and invalid input before entering the plugin. **File:**
  `internal/mcp/handlers/plugin_tools_test.go`. **How:** scoped dispatcher tests
  with a recording plugin-tool service.
- **What:** Launch/resume carry the complete snapshot and live changes reach all
  active executions without one failure blocking others. **File:**
  `internal/orchestrator/executor/executor_mcp_mode_test.go`,
  `internal/agent/runtime/lifecycle/manager_mcp_plugin_tools_test.go`,
  `internal/backendapp/plugin_tool_refresher_test.go`. **How:** resolver,
  lifecycle client, and coalescing refresher tests.
- **What:** Managed plugin to MCP end-to-end discovery, invocation, bound
  context, output mapping, and hot removal. **File:** a focused integration test
  under `internal/backendapp` or `internal/plugins/runtime`. **How:** real fixture
  subprocess and MCP JSON-RPC transport with bounded timeouts and teardown.

## Verification Results

- Implemented and verified manifest/SDK, plugin catalog/invocation, dynamic MCP
  registry, backend dispatch, and the managed fixture package.
- Focused race suites pass for `internal/plugins`, `internal/mcp/server`,
  `internal/mcp/handlers`, `internal/agent/runtime/lifecycle`,
  `internal/agent/runtime/agentctl`, `internal/agentctl/server/api`,
  `internal/backendapp`, and `pkg/pluginsdk`.
- `go test ./...` reaches unrelated existing filesystem/task-service failures
  in this environment; no plugin/MCP package failures were reported.
- A live backend on port `18101` has the fixture package installed and active.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Manifest and SDK contract](task-01-manifest-sdk-contract.md)

Wave 2 (parallel candidates after Task 01; user authorization required):

- [x] [Task 02: Plugin catalog and invocation](task-02-plugin-catalog-invocation.md)
- [x] [Task 03: Agentctl dynamic MCP registry](task-03-agentctl-dynamic-registry.md)

Wave 3:

- [x] [Task 04: Backend dispatch and runtime propagation](task-04-runtime-propagation-backend-dispatch.md)

Wave 4:

- [ ] [Task 05: Managed fixture end-to-end proof](task-05-end-to-end-fixture.md)

Wave 5:

- [x] [Task 06: Plugin agent-tool documentation](task-06-authoring-docs.md)

The default implementation order is sequential in the primary conversation.
Wave labels do not authorize subagents.

## Open Risks

- MCP clients vary in whether they honor `tools/list_changed`; conformance must
  be checked against supported clients, but backend authorization cannot depend
  on refresh behavior.
- Plugin lifecycle callbacks run on supervision paths that must not block.
  Catalog propagation needs a serialized, nonblocking coordinator.
- The public manifest/protobuf/SDK contract is additive but durable. Generated
  stubs, authoring docs, fixture packages, and minimum Kandev version guidance
  must land together.
