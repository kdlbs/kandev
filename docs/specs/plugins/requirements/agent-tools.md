---
status: draft
system: plugins
created: 2026-08-11
owners:
  - Kandev
---
# Plugin-Contributed Agent Tools Requirements

## Overview

Plugin authors can extend Kandev's UI and data behavior, but task agents cannot invoke plugin-owned operations. Authors currently have to ship and configure a separate MCP server, which duplicates plugin installation, lifecycle, trust, and task-context handling. Kandev needs one supported way for an installed plugin to contribute task-aware tools to agents.

## Requirements

### REQ-PLUGINS-AGENT-TOOLS-001: Plugin-Contributed Agent Tools

**Intent:** Plugin authors can extend Kandev's UI and data behavior, but task agents cannot invoke plugin-owned operations. Authors currently have to ship and configure a separate MCP server, which duplicates plugin installation, lifecycle, trust, and task-context handling. Kandev needs one supported way for an installed plugin to contribute task-aware tools to agents.

#### Acceptance criteria

- **AC-PLUGINS-AGENT-TOOLS-001.1:** A managed plugin package (`runtime.type: binary`) may declare zero or more agent tools in its manifest. Each declaration has a plugin-local name, description, input JSON Schema, optional output JSON Schema, supported task surfaces, and optional MCP annotations. Legacy remote HTTP plugins cannot declare agent tools because they do not implement the managed gRPC runtime.
- **AC-PLUGINS-AGENT-TOOLS-001.2:** Only active plugins contribute tools. Registered, disabled, errored, and uninstalled plugins contribute none.
- **AC-PLUGINS-AGENT-TOOLS-001.3:** Plugin tools are exposed through the existing task-aware Kandev MCP server. Plugins do not run or register a second MCP server.
- **AC-PLUGINS-AGENT-TOOLS-001.4:** Kandev derives a readable provider-safe MCP name as `kandev_<plugin-id-slug>_<local-name>`, where the plugin ID slug replaces punctuation with underscores. The manifest does not choose an arbitrary global MCP name. If the readable name exceeds the provider limit, Kandev truncates the slug and appends a short stable hash suffix. Install validation still rejects a final-name collision.
- **AC-PLUGINS-AGENT-TOOLS-001.5:** A declared tool may target `kanban-task`, `office-task`, or both. Plugin tools are not exposed to `configuration` or `external` MCP surfaces in this version.
- **AC-PLUGINS-AGENT-TOOLS-001.6:** Installing, upgrading, enabling, disabling, degrading, recovering, or uninstalling a plugin recomputes the authoritative plugin-tool catalog.
- **AC-PLUGINS-AGENT-TOOLS-001.7:** Running MCP servers replace their effective tool registry atomically. A changed registry emits one `notifications/tools/list_changed` notification; the agent process, task session, and MCP server do not restart.
- **AC-PLUGINS-AGENT-TOOLS-001.8:** A client that does not honor `tools/list_changed` sees the new catalog after its next MCP reconnect or task-session restart. Kandev does not terminate a healthy agent process solely to force discovery refresh.

## Migrated source detail

## Why

Plugin authors can extend Kandev's UI and data behavior, but task agents cannot
invoke plugin-owned operations. Authors currently have to ship and configure a
separate MCP server, which duplicates plugin installation, lifecycle, trust,
and task-context handling. Kandev needs one supported way for an installed
plugin to contribute task-aware tools to agents.

Decision: [ADR-2026-08-11-plugin-tools-through-kandev-mcp](../../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md).

Implementation plan: [Plugin-Contributed Agent Tools](../../../plans/plugin-agent-tools/plan.md).

## What

- A managed plugin package (`runtime.type: binary`) may declare zero or more
  agent tools in its manifest. Each declaration has a plugin-local name,
  description, input JSON Schema, optional output JSON Schema, supported task
  surfaces, and optional MCP annotations. Legacy remote HTTP plugins cannot
  declare agent tools because they do not implement the managed gRPC runtime.
- Only active plugins contribute tools. Registered, disabled, errored, and
  uninstalled plugins contribute none.
- Plugin tools are exposed through the existing task-aware Kandev MCP server.
  Plugins do not run or register a second MCP server.
- Kandev derives a readable provider-safe MCP name as
  `kandev_<plugin-id-slug>_<local-name>`, where the plugin ID slug replaces
  punctuation with underscores. The manifest does not choose an arbitrary
  global MCP name. If the readable name exceeds the provider limit, Kandev
  truncates the slug and appends a short stable hash suffix. Install
  validation still rejects a final-name collision.
- A declared tool may target `kanban-task`, `office-task`, or both. Plugin tools
  are not exposed to `configuration` or `external` MCP surfaces in this version.
- Installing, upgrading, enabling, disabling, degrading, recovering, or
  uninstalling a plugin recomputes the authoritative plugin-tool catalog.
- Running MCP servers replace their effective tool registry atomically. A
  changed registry emits one `notifications/tools/list_changed` notification;
  the agent process, task session, and MCP server do not restart.
- A client that does not honor `tools/list_changed` sees the new catalog after
  its next MCP reconnect or task-session restart. Kandev does not terminate a
  healthy agent process solely to force discovery refresh.
- Tool invocation is a single request to the already supervised plugin process.
  Kandev supplies immutable invocation ID, task ID, session ID, workspace ID,
  and MCP surface context derived from the running session. Those values are not
  accepted from tool arguments.
- The plugin returns required fallback text, optional structured JSON content,
  and an `is_error` value. Structured content must conform to the declared
  output schema when one is present.
- Tool calls have a 30-second deadline, propagate cancellation, are never
  automatically retried, and return at most 1 MiB across fallback text and
  structured content.
- Manifest validation limits each plugin to 16 tools, local names to 32
  characters, descriptions to 1,024 bytes, and each serialized schema to
  64 KiB. Input and output schemas must compile and have an object root.
- MCP annotations are hints, not authorization. Omitted annotations use
  conservative defaults: `readOnlyHint=false`, `destructiveHint=true`,
  `idempotentHint=false`, and `openWorldHint=true`.
- Declaring an agent tool grants only discovery and invocation of that RPC. It
  grants no additional Host API capability; existing `api_read`, `api_write`,
  `state`, `secrets`, `agent_invoke`, and `auth` gates remain authoritative.
- Discovery is not an authorization boundary. Every invocation revalidates the
  active plugin record, exact declared tool, allowed surface, backend-bound
  task/session pair, caller scope, and input schema before calling the plugin.
- Plugin tool calls are recorded in structured logs with invocation ID, plugin
  ID, local tool name, task ID, session ID, duration, and outcome. Arguments,
  structured output, secret values, and plugin configuration are not logged.

## Manifest Contract

```yaml
agent_tools:
  - name: add_tag
    description: Add an existing tag to the current task.
    surfaces: [kanban-task]
    input_schema:
      type: object
      properties:
        tag_id:
          type: string
      required: [tag_id]
      additionalProperties: false
    output_schema:
      type: object
      properties:
        task_id:
          type: string
        tag_id:
          type: string
      required: [task_id, tag_id]
      additionalProperties: false
    annotations:
      read_only_hint: false
      destructive_hint: false
      idempotent_hint: true
      open_world_hint: false
```

`name` must match `^[a-z0-9][a-z0-9_]{0,31}$`. `surfaces` must contain one
or both supported task surfaces without duplicates. `input_schema` is required;
`output_schema` and `annotations` are optional. Unknown fields continue to
follow the manifest's existing compatibility rules.

An install or upgrade fails before changing the active installation when its
declarations are invalid, when two declarations produce the same exposed name,
or when a declaration conflicts with a tool name reserved by another installed
plugin version. A failed package change leaves the previous installed version
and its tools unchanged.

## Plugin SDK Contract

Plugins that contribute declared tools implement the optional `AgentToolPlugin`
interface:

```go
type AgentToolPlugin interface {
    InvokeAgentTool(context.Context, *AgentToolRequest) (*AgentToolResult, error)
}
```

The author-facing types are:

```go
type AgentToolRequest struct {
    InvocationID string
    Name         string
    Arguments    map[string]any
    Context      AgentToolContext
}

type AgentToolContext struct {
    TaskID      string
    SessionID   string
    WorkspaceID string
    Surface     string
}

type AgentToolResult struct {
    Text              string
    StructuredContent map[string]any
    IsError           bool
}
```

The base `Plugin` interface remains unchanged. The SDK's gRPC adapter checks
whether the implementation satisfies `AgentToolPlugin` and returns gRPC
`Unimplemented` otherwise. `UnimplementedPlugin.InvokeAgentTool` also returns
`Unimplemented`, so both embedding and non-embedding existing implementations
remain source-compatible. Kandev calls the method only for a declaration present
in the active manifest.

## Runtime Catalog

The runtime catalog is a sorted snapshot containing a process-generation ID,
monotonic revision, and the complete set of active plugin tool descriptors.
The generation ID changes on backend startup. A running agentctl accepts a new
generation and, within one generation, ignores snapshots older than its applied
revision. This prevents delayed refresh requests from restoring stale tools.

Launch and resume carry the complete current snapshot. Live plugin lifecycle
changes push a replacement snapshot to every active execution. Agentctl filters
the snapshot by its backend-owned MCP surface before constructing the effective
registry.

Catalog snapshots are runtime state. The declarations and plugin lifecycle
status remain persisted in the installed plugin record; no new database entity
is introduced.

## Permissions

- Installing, upgrading, enabling, disabling, or uninstalling a plugin retains
  the existing operator/admin permission boundary.
- Any task agent whose backend-owned surface matches an active declaration may
  discover the tool. There is no per-workspace or per-agent-profile enablement
  setting in this version.
- Agentctl injects its bound task and session IDs into the backend request. An
  agent cannot select another task or session through arguments.
- The backend applies the same in-session MCP identity scoping used by built-in
  task tools and verifies that the bound session belongs to the bound task.
- Plugins remain privileged installed code. The tool declaration does not
  sandbox the plugin process or reduce capabilities already granted by its
  manifest.

## Failure Modes

- Invalid tool declarations reject install or upgrade without replacing the
  previous installed plugin.
- A catalog refresh failure is logged per execution and does not roll back the
  plugin lifecycle change. Calls are still rejected against current backend
  state even if a client temporarily retains stale discovery. Launch, resume,
  or the next catalog change repairs the runtime catalog.
- A call discovered immediately before disable, uninstall, or degradation
  fails closed when backend revalidation observes the new state.
- Disabling, uninstalling, or replacing the plugin process cancels calls still
  executing in that process. Registry replacement alone does not cancel calls
  to unrelated plugins.
- An unavailable or crashed plugin returns an MCP tool error. Kandev does not
  retry because the operation may have produced side effects before failure.
- Invalid arguments are rejected before plugin invocation. Backend validation
  repeats agentctl's shared MCP schema validation as defense in depth.
- A plugin result that is missing fallback text, exceeds the result limit, or
  violates its declared output schema becomes an MCP tool error.
- Plugin panics or transport failures follow the existing supervised plugin
  error/recovery lifecycle; recovery republishes the tool catalog.

## Persistence Guarantees

- Tool declarations survive restart as part of the installed manifest record.
- Plugin active/disabled/error status follows the existing plugin lifecycle
  persistence contract.
- Runtime catalog generation, revision, and per-agentctl applied snapshots do
  not survive restart. They are rebuilt from installed plugin records.
- In-flight calls are not resumed after backend, agentctl, agent, or plugin
  process restart.

## Scenarios

- **GIVEN** an active plugin declaring a valid Kanban tool, **WHEN** a Kanban
  task agent initializes MCP, **THEN** `tools/list` includes the derived tool
  name, description, schemas, and conservative or declared annotations.
- **GIVEN** the same plugin tool targets only `kanban-task`, **WHEN** an Office,
  Configuration, or External MCP client lists tools, **THEN** that tool is not
  present.
- **GIVEN** a running compatible task agent, **WHEN** an operator enables a
  plugin with a matching tool, **THEN** the MCP connection receives one
  `tools/list_changed` notification and the next `tools/list` includes it
  without restarting the agent process.
- **GIVEN** a running task agent has discovered a plugin tool, **WHEN** the
  plugin is disabled before invocation, **THEN** the call fails without
  entering the plugin and the refreshed list excludes the tool.
- **GIVEN** a valid call, **WHEN** the plugin executes it, **THEN** the plugin
  receives the exact arguments plus Kandev-bound invocation, task, session,
  workspace, and surface context and the agent receives fallback text and any
  valid structured content.
- **GIVEN** an agent supplies another task or session ID as ordinary tool
  arguments, **WHEN** it invokes the tool, **THEN** those values do not replace
  the context bound by Kandev.
- **GIVEN** invalid arguments, **WHEN** the agent invokes a plugin tool, **THEN**
  Kandev returns a schema-validation error and the plugin receives no RPC.
- **GIVEN** a plugin call times out or its process crashes, **WHEN** the request
  completes, **THEN** the agent receives one MCP error and Kandev does not retry
  the call.
- **GIVEN** two catalog snapshots arrive out of order, **WHEN** agentctl has
  already applied the newer revision from the same generation, **THEN** it
  ignores the older snapshot and does not restore removed tools.
- **GIVEN** a client ignores `tools/list_changed`, **WHEN** the plugin catalog
  changes, **THEN** Kandev keeps the agent running and backend invocation still
  enforces the current catalog; reconnecting exposes the current list.

## Out of Scope

- A plugin-hosted MCP server or transparent federation of arbitrary MCP
  resources, prompts, roots, sampling, or elicitation.
- Plugin tools on Configuration or External MCP surfaces.
- Per-workspace, per-workflow, per-agent-profile, or per-tool enablement UI.
- User approval workflows beyond annotations interpreted by the connected MCP
  client.
- Streaming tool results, progress notifications, binary/image/audio content,
  embedded resources, or resumable calls.
- Automatic retries or exactly-once execution guarantees.
- Sandboxing installed plugin binaries or native UI bundles.
