---
status: draft
system: agents
requirements:
  - REQ-AGENTS-MCP-DISCOVERY-001
  - REQ-AGENTS-MCP-DISCOVERY-002
---

# MCP tool discovery guidance system design

## Purpose and boundaries

The Agents system owns compact runtime instructions for Kandev tool discovery.
The MCP registry remains authoritative for callable operations and schemas.
This change uses the existing typed profile and canonical tool-name contracts.
It does not add a search tool or a client-name switch.

The Canvases system owns the
[canvas authoring and task preset](../../canvases/system-design/agent-authored-web-apps.md#agent-authoring-guidance).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-MCP-DISCOVERY-001` | Discovery instructions |
| `REQ-AGENTS-MCP-DISCOVERY-002` | Essential guidance and capability projection |

## Discovery instructions

`apps/backend/config/prompts/kandev-context.md` keeps the `KANDEV MCP TOOLS`
marker and trusted task/session placeholders. The introductory text identifies
the subsequent list as selected workflow guidance.

Proposed shared discovery text:

> These instructions list selected Kandev tools, not the complete MCP catalog.
> For Kandev operations, use the tools exposed by the `kandev` server.
> If the needed tool is already callable, use its current schema.
> Otherwise, use your client's native tool search or discovery when available.
> Search for `kandev` plus the operation, such as `canvas`, or the known canonical tool name.
> If search is unavailable, inspect the MCP tools available in your client.
> Use the exact callable name and schema that the client exposes.
> Canonical names end in `_kandev`; clients can display a server-qualified alias.
> An omitted entry here does not mean that the tool is unavailable.
> If discovery cannot find a required tool, report that limitation before substituting another result.

Known names are search terms, not instructions to fabricate a tool invocation.
The prompt does not prescribe `ToolSearch`, `tool_search`, or any provider API.
Discovery is conditional on a missing callable tool, so eager clients avoid
redundant searches. Previously discovered definitions remain usable during the
same turn unless the client reports a catalog change or an unavailable tool.

## Essential guidance and capability projection

The compact template retains these behavior-critical sections:

| Section | Treatment |
| --- | --- |
| Task/session identity | Preserve trusted placeholders and marker. |
| Delegation | Preserve explicit authorization and persistent-task/session boundaries. |
| Questions | Preserve the current capability-selected tool and complete hard barrier. |
| Title | Preserve the pending-title condition and first-action instruction. |
| Completion | Preserve the step gate, discovery hint, and final-action semantics. |
| Autopilot | Preserve parent-question selection and root behavior. |
| Plans | One short line for create/get/update and preserving user edits. |
| Rich output | One short routing rule; obtain schemas and examples from tool discovery. |
| Coordinator controls | Preserve the gated interrupt/stop/restart rules. |
| Canvas | One gated create/read/write/publish workflow, owned by Canvases. |

Remove standalone descriptions for routine task/workspace/workflow/session
lists, generic updates, plan deletion, and walkthrough CRUD. Keep their tools
registered. Keep task creation and session names only where authorization rules
need them. Rename `Available tools` to `Essential workflow guidance`.

The raw template currently has 3,482 UTF-8 bytes. Target at most 2,800 bytes
after compaction, including the discovery text and optional placeholders.
Record rendered sizes for an ordinary task and a canvas-enabled task too. The
rendered budgets are separate because capability sections and runtime values
expand the reusable template at render time.
Byte counts measure prompt size without adding a model-specific tokenizer.

`sysprompt.KandevContextOptions` adds an explicit canvas-guidance boolean.
Its zero value omits canvas guidance. The caller derives the value from the
same resolved MCP profile that determines `CapabilityCanvas` registration.
Prompt text and user metadata cannot enable that capability.

The executor already owns `resolveTaskSessionMCPProfile` and
`withCanvasCapability`. Expose a narrow read-only profile query if the
orchestrator needs it. Reuse this resolver instead of storing another flag in
the orchestrator or reproducing its surface rules.

Wire the projection through `wrapCreatedSessionPrompt`,
`applyLaunchPromptContext`, and the workflow auto-start/context-reset path.
Compute the workflow projection once for both recorded and dispatched text.
Retain the existing Office and passthrough branches. Configuration and
automation profiles do not acquire canvas guidance from a global flag alone.

## Failure and recovery

A profile resolution error retains existing launch error behavior. The prompt
must not advertise capabilities from a guessed profile. Missing tools produce
an accurate limitation report after the client-specific discovery options are
exhausted. Discovery never authorizes a mutation outside the user's request.

Existing conversations and stored prompts remain historical records. Normal
launch/reset injection receives the new template. This change does not send
unsolicited messages or restart live sessions.

## Verification

Tests in `internal/sysprompt` cover discovery, essential invariants, the canvas
boolean, compact size, and removal of routine catalog entries. Assertions check
instructions and conditions rather than a complete prose snapshot.

Producer tests in `internal/orchestrator` cover task start, prepared-session
start, and context reset with canvas capability present and absent. They also
cover excluded surfaces and agreement between recorded and dispatched text.
MCP profile/server tests prove that prompt compaction does not remove tools.

Deterministic tests prove instruction delivery and tool exposure. They cannot
prove that every external model will obey prose on every turn.

## Related decisions

- [Typed MCP tool profiles](../../../decisions/2026-08-08-mcp-tool-profiles.md)
- [Canonical MCP tool names](../../../decisions/2026-08-31-agent-aware-mcp-tool-names.md)

No new ADR is required. This change improves guidance within those boundaries.
