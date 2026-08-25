---
status: active
system: tasks
created: 2026-07-22
owners:
  - kandev
---
# Explicit Workflow-Step Completion Signal Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001: Explicit Workflow-Step Completion Signal

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Signal-gated Kanban and Office workflows need a reliable distinction between an agent ending a turn and an agent completing the current workflow step. If the completion tool disappears from an agent client's deferred catalog during an MCP reconnect, finished work remains stuck even though Kandev still expects an explicit signal.

## What

- `step_complete_kandev` is a task-session capability. It is registered in `ModeTask` and `ModeOffice`, and is never registered or advertised in `ModeConfig` or `ModeExternal`.
- Task-mode and Office-mode registration is stable for the lifetime of the MCP server. A workflow step's `auto_advance_requires_signal` setting controls prompt instructions and transition behavior, not whether the tool exists in either tool list.
- When the current step requires an explicit signal, the first-turn context tells the agent to call `step_complete_kandev` as its last action after satisfying every requirement. Ungated Office steps keep the tool available but do not receive this imperative instruction.
- The canonical MCP protocol name is `step_complete_kandev`. Clients may expose a runtime-qualified alias such as `mcp__kandev__step_complete_kandev`; Kandev instructions distinguish the canonical name from client-specific qualification instead of claiming that one display form is universal.
- `step_complete_kandev` carries no vendor-specific eager-load metadata. Agents use their normal MCP catalog or tool-search mechanism to discover it in Kanban task mode.
- Creating or loading an ACP session supplies the local Kandev MCP server. After a transient MCP reconnect, the client can list and call `step_complete_kandev` again without a user message or process restart.
- Kandev does not pin the Claude ACP bridge version as part of this feature. Bridge selection retains the existing unversioned package behavior; diagnostics continue to report the version returned during ACP initialization.
- Existing idempotency, clarification barriers, and re-open semantics remain unchanged. ADR 0015's planned manual "Mark complete & advance" fallback is not currently implemented and is outside this reliability fix.
- Explicit user-cancel completion is owned by the [Cancelled Turn Completion](workflow-cancelled-turn-completion.md) spec. When a step opts into that policy, the user's cancel action is a human completion decision and may run `on_turn_complete` without an agent-emitted signal; internal cancellation paths remain non-completing.

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
| `ModeOffice` | Registered |
| `ModeConfig` | Not registered or advertised |
| `ModeExternal` | Not registered or advertised |

## Failure Modes

- If the Kandev MCP transport is temporarily disconnected, the client reports the transport failure and retries its normal connection path. It must not convert the failure into a permanent "tool does not exist" conclusion while the server is reconnecting.
- If a connected client defers the tool from its active context, the agent uses that client's normal tool-search mechanism to resolve the canonical `step_complete_kandev` name.
- If reconnect does not recover, the task stays on the current step. The user can retry the agent or move the task through the normal workflow UI; Kandev never auto-advances from a bare halt on a signal-gated step.
- If an Office step requires a signal, a failure to load or call the tool keeps the task on the current step. The user can retry the agent or move the task through the normal workflow UI.

## Persistence Guarantees

The pending completion signal continues to use `TaskSession.Metadata` as specified by ADR 0015. Tool catalog state is not persisted; it is reconstructed from the session's MCP mode on new session, load, and reconnect. ACP bridge package resolution remains external to session data.

## Scenarios

- **GIVEN** a Kanban task session in `ModeTask`, **WHEN** the client lists or searches tools, **THEN** `step_complete_kandev` is discoverable without vendor-specific eager-load metadata.
- **GIVEN** a non-Office Kanban workflow step with an agent profile and `auto_advance_requires_signal=true`, **WHEN** its task resolves the step profile as the runner, **THEN** the task remains in `ModeTask`, its first-turn context instructs normal discovery of `step_complete_kandev`, and the task-mode catalog contains the tool.
- **GIVEN** an Office task session in `ModeOffice`, **WHEN** the client lists tools, **THEN** `step_complete_kandev` is discoverable.
- **GIVEN** an ungated Office workflow step, **WHEN** its first-turn context is generated, **THEN** the tool remains available but the context does not instruct the agent to call it as the final action.
- **GIVEN** a signal-gated Office workflow step, **WHEN** its first-turn context is generated, **THEN** the context instructs the agent to call `step_complete_kandev` as the final action.
- **GIVEN** a task-mode client has connected to Kandev MCP, **WHEN** its MCP connection drops and reconnects, **THEN** the client can list and call `step_complete_kandev` without another user message.
- **GIVEN** a signal-gated workflow step, **WHEN** the agent finishes without calling the tool, **THEN** the task remains on the current step and no automatic transition runs.
- **GIVEN** a signal-gated workflow step that also enables explicit user-cancel completion, **WHEN** the user cancels the active turn, **THEN** the configured completion transition may run without `step_complete_kandev` as defined by the cancelled-turn completion policy.
- **GIVEN** a client that displays fully qualified MCP names, **WHEN** it resolves the completion instruction, **THEN** it can associate canonical `step_complete_kandev` with its qualified runtime alias.
- **GIVEN** Claude ACP initializes, **WHEN** the bridge reports its identity, **THEN** Kandev records the reported bridge name and version in diagnostics without constraining package resolution in this feature.

## Out of Scope

- Changing completion-signal persistence or ordinary halt semantics, or implementing ADR 0015's planned manual fallback UI. Explicit user-cancel behavior is defined separately by the cancelled-turn completion spec.
- Adding vendor-specific eager-load metadata; agents are expected to use normal MCP tool discovery.
- Implementing a generic MCP proxy or replacing the ACP bridge.
