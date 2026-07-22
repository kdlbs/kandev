---
status: shipped
created: 2026-07-22
owner: kandev
---

# Explicit Workflow-Step Completion Signal

## Why

Signal-gated Kanban workflows need a reliable distinction between an agent ending a turn and an agent completing the current workflow step. If the completion tool disappears from an agent client's deferred catalog during an MCP reconnect, finished work remains stuck even though Kandev still expects an explicit signal.

## What

- `step_complete_kandev` is a Kanban task-session capability. It is registered in `ModeTask` and is never registered or advertised in `ModeOffice`, `ModeConfig`, or `ModeExternal`.
- Task-mode registration is stable for the lifetime of the MCP server. A workflow step's `auto_advance_requires_signal` setting controls prompt instructions and transition behavior, not whether the tool exists in the task-mode tool list.
- When the current step requires an explicit signal, the first-turn context tells the agent to call `step_complete_kandev` as its last action after satisfying every requirement.
- The canonical MCP protocol name is `step_complete_kandev`. Clients may expose a runtime-qualified alias such as `mcp__kandev__step_complete_kandev`; Kandev instructions distinguish the canonical name from client-specific qualification instead of claiming that one display form is universal.
- `step_complete_kandev` carries no vendor-specific eager-load metadata. Agents use their normal MCP catalog or tool-search mechanism to discover it in Kanban task mode.
- Creating or loading an ACP session supplies the local Kandev MCP server. After a transient MCP reconnect, the client can list and call `step_complete_kandev` again without a user message or process restart.
- Kandev does not pin the Claude ACP bridge version as part of this feature. Bridge selection retains the existing unversioned package behavior; diagnostics continue to report the version returned during ACP initialization.
- Existing idempotency, clarification barriers, and re-open semantics remain unchanged. ADR 0015's planned manual "Mark complete & advance" fallback is not currently implemented and is outside this reliability fix.

## API Surface

### MCP tool

`ModeTask` exposes:

```json
{
  "name": "step_complete_kandev",
  "arguments": {
    "summary": "string",
    "handoff": "string?",
    "blockers": "string?"
  }
}
```

The tool uses the standard MCP definition without `anthropic/alwaysLoad`. Client-side deferred discovery does not change handler authorization, persistence, idempotency, or workflow transition semantics.

### Mode boundary

| MCP mode | `step_complete_kandev` |
|---|---|
| `ModeTask` | Registered |
| `ModeOffice` | Not registered or advertised |
| `ModeConfig` | Not registered or advertised |
| `ModeExternal` | Not registered or advertised |

## Failure Modes

- If the Kandev MCP transport is temporarily disconnected, the client reports the transport failure and retries its normal connection path. It must not convert the failure into a permanent "tool does not exist" conclusion while the server is reconnecting.
- If a connected client defers the tool from its active context, the agent uses that client's normal tool-search mechanism to resolve the canonical `step_complete_kandev` name.
- If reconnect does not recover, the task stays on the current step. The user can retry the agent or move the task through the normal workflow UI; Kandev never auto-advances from a bare halt on a signal-gated step.
- Unsupported clients ignore the Anthropic metadata and continue using the standard MCP tool definition.
- A failure to load or call the tool in Office mode is expected and must not trigger reconnect remediation because Office does not own this capability.

## Persistence Guarantees

The pending completion signal continues to use `TaskSession.Metadata` as specified by ADR 0015. Tool catalog state is not persisted; it is reconstructed from the session's MCP mode on new session, load, and reconnect. ACP bridge package resolution remains external to session data.

## Scenarios

- **GIVEN** a Kanban task session in `ModeTask`, **WHEN** the client lists or searches tools, **THEN** `step_complete_kandev` is discoverable without vendor-specific eager-load metadata.
- **GIVEN** an Office task session in `ModeOffice`, **WHEN** the client lists tools or receives its first-turn context, **THEN** `step_complete_kandev` is absent.
- **GIVEN** a task-mode client has connected to Kandev MCP, **WHEN** its MCP connection drops and reconnects, **THEN** the client can list and call `step_complete_kandev` without another user message.
- **GIVEN** a signal-gated workflow step, **WHEN** the agent finishes without calling the tool, **THEN** the task remains on the current step and no automatic transition runs.
- **GIVEN** a client that displays fully qualified MCP names, **WHEN** it resolves the completion instruction, **THEN** it can associate canonical `step_complete_kandev` with its qualified runtime alias.
- **GIVEN** Claude ACP initializes, **WHEN** the bridge reports its identity, **THEN** Kandev records the reported bridge name and version in diagnostics without constraining package resolution in this feature.

## Out of Scope

- Exposing `step_complete_kandev` to Office agents.
- Changing completion-signal persistence or transition semantics, or implementing ADR 0015's planned manual fallback UI.
- Adding vendor-specific eager-load metadata; agents are expected to use normal MCP tool discovery.
- Implementing a generic MCP proxy or replacing the ACP bridge.
