# ADR-2026-09-07-separate-runtime-permissions-and-advisory-actions: Keep MCP availability separate from runtime permissions

**Status:** accepted  
**Date:** 2026-09-07  
**Area:** backend, protocol

## Context

Office runtime capabilities are enforced permissions for the runtime HTTP
action surface. They are persisted on a run and copied into the agent JWT.

The `record_step_decision_kandev` MCP tool is different. It is always exposed
to the bound session, and the handler performs live workflow-seat
authorization when the agent calls it. A run-launch seat lookup can therefore
describe prompt availability, but it cannot grant or revoke the MCP action.

Putting the MCP action in `runtime.Capabilities` makes `Allows` and JWT data
look like authorization even though the MCP handler does not enforce them.

## Decision

Keep `runtime.Capabilities` limited to permissions enforced by the runtime HTTP
handler. Store seat-derived MCP affordances in `RunContext.AvailableActions`.

The scheduler may merge `AvailableActions` with runtime capability keys when it
renders the prompt. It must not serialize those actions into runtime JWT
capability claims. The MCP handler remains the live authorization boundary.

If the seat lookup fails, context construction fails and the scheduler uses its
existing retry path. A missing resolver, missing step, or absent seat produces
no advertised action.

## Consequences

Prompts can advertise the decision tool without changing the meaning of the
runtime permission model. Run input snapshots retain the launch-time advisory
state for inspection, while JWT claims remain limited to enforced permissions.

Seat changes after launch can make the prompt stale. This is safe because the
decision handler resolves the current step and participant seat again.

Transient seat lookup failures delay a run through the normal retry policy
instead of launching a prompt that requires an unavailable action.

## Alternatives Considered

- Keep `record_step_decision` in `Capabilities`: rejected because a false flag
  does not block the MCP call and a true flag does not guarantee live authority.
- Enforce the MCP action with the runtime JWT capability: rejected because it
  would duplicate workflow-seat authorization and couple the generic runtime
  permission model to one tool transport.
- Continue on lookup errors and omit the action: rejected because review and
  approval prompts still require the decision call, which would create a
  contradictory launch state.
