# ADR-2026-08-08-autopilot-mcp-question-capabilities: Use One Question Capability Per Task Session

**Status:** accepted
**Date:** 2026-08-08
**Area:** backend, agentctl, protocol
**Related ADR:** [ADR-2026-08-08-task-autopilot-contract](2026-08-08-task-autopilot-contract.md)
**Related spec:** [Task Autopilot Mode](../specs/tasks/requirements/autopilot-mode.md)

## Context

Task-mode MCP currently uses one `disableAskQuestion` switch. It can hide the
operator question tool, but it cannot express why the tool is hidden or select a
different question owner. Adding `ask_parent_question_kandev` beside the existing
tool would make both tools visible unless every registration path applies the same
condition.

Both question tools expose overlapping schemas and instructions. Showing both
consumes model context and lets an autopilot agent choose the wrong owner. A
top-level autopilot task also has no parent to ask.

## Decision

1. The backend derives one question capability for each task session and transports
   it as one enum-like value. It must not transport independent booleans that can
   expose both tools.
2. The capability matrix is:

   | Task | Direct parent | MCP question capability |
   |---|---|---|
   | Normal | Any | `ask_user_question_kandev` |
   | Autopilot | Present | `ask_parent_question_kandev` |
   | Autopilot | Absent | None |

3. The MCP server registers exactly the tool selected by the capability. It never
   registers both question tools for one session.
4. An autopilot root task receives no question tool. Its system prompt tells it to
   continue with best judgment. A stale client call to the parent-question handler
   fails closed and creates no pending state.
5. Other task-mode tools remain governed by the existing mode and provider
   allowlists. “Minimum necessary” means that the session has no duplicate or
   unusable question capability; it does not remove tools required for task work,
   delegation, workflow control, or configured provider actions.
6. The capability is resolved when the session MCP server is built. If a task is
   reparented while its session is active, the next session start or resume rebuilds
   the capability. The backend never silently changes the active tool list.

## Consequences

- Autopilot agents cannot call the operator question tool by discovery or normal
  schema use.
- Root autopilot agents save the most context because they receive no question tool.
- One capability value prevents invalid states such as both tools being registered.
- A parent change may require a session restart or resume before the new question
  capability appears.
- Existing task and provider tools remain available, so this decision does not
  change task execution or provider automation semantics.

## Alternatives Considered

### Register both tools and rely on the system prompt

Rejected. The model still receives both schemas and can select the operator tool.

### Add a second `enableParentQuestion` boolean

Rejected. Two booleans allow an invalid state where both question tools are visible.

### Remove every task-mode MCP tool for autopilot

Rejected for this change. Autopilot agents still need task execution, delegation,
workflow, and provider tools. A complete per-task allowlist is a separate design.
