# ADR-2026-09-07-separate-runtime-permissions-and-advisory-actions: Keep live advisory actions separate from runtime permissions

**Status:** accepted
**Date:** 2026-09-07
**Area:** backend, protocol

## Context

Office runtime capabilities are enforced permissions for the runtime HTTP
action surface. They are persisted on a run and copied into the agent JWT.

Workflow-step decision recording is different. It is a task-bound Office CLI
command backed by the runtime HTTP surface, and the handler performs live
workflow-seat authorization when the agent calls it. A run-launch seat lookup
can therefore select prompt and skill guidance, but it cannot grant or revoke
the decision action.

Putting the decision action in `runtime.Capabilities` makes `Allows` and JWT
data look like authorization even though the runtime handler must resolve the
current reviewer or approver seat on every call.

## Decision

Keep `runtime.Capabilities` limited to static permissions enforced directly by
the runtime HTTP handlers. Store the seat-derived decision affordance in
`RunContext.AvailableActions`.

The scheduler may merge `AvailableActions` with runtime capability keys when it
renders the prompt and may use it to inject a narrow Kandev skill. It must not
serialize those actions into runtime JWT capability claims. The task, session,
and agent come only from the signed run context; the runtime handler resolves
the current workflow step and participant seat live before it delegates to
`DashboardService.RecordAgentDecision`.

The Office MCP registry does not expose `record_step_decision_kandev`. Decision
recording uses `$KANDEV_CLI kandev task decision`, which calls the narrow Office
runtime endpoint. The CLI and endpoint accept only `decision` and `reason`; they
do not accept task, step, role, participant, session, or agent identifiers.

If the seat lookup fails, context construction fails and the scheduler uses its
existing retry path. A missing resolver, missing step, or absent seat produces
no advertised action.

## Consequences

Prompts and injected skills can advertise the decision command without changing
the meaning of the runtime permission model. Run input snapshots retain the
launch-time advisory state for inspection, while JWT claims remain limited to
static enforced permissions.

Seat changes after launch can make the prompt stale. This is safe because the
decision handler resolves the current step and participant seat again. Moving
the action out of MCP also removes its tool schema from every Office turn.

Transient seat lookup failures delay a run through the normal retry policy
instead of launching a prompt that requires an unavailable action.

## Alternatives Considered

- Keep `record_step_decision_kandev` in the Office MCP registry: rejected
  because every Office turn pays for a decision schema even when the caller
  does not occupy a decision seat, while the established Office CLI already
  provides a task-bound authenticated transport.
- Keep `record_step_decision` in `Capabilities`: rejected because a false flag
  would not represent the live participant slate and a true flag would not
  guarantee current authority.
- Enforce the CLI action only with the runtime JWT capability: rejected because
  it would duplicate workflow-seat authorization and turn a launch-time
  snapshot into an authority source.
- Continue on lookup errors and omit the action: rejected because review and
  approval prompts still require the decision call, which would create a
  contradictory launch state.
