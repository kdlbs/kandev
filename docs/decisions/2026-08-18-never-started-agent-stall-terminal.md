# ADR-2026-08-18-never-started-agent-stall-terminal: Treat Never-Started Agent Stalls as Terminal

**Status:** accepted
**Date:** 2026-08-18
**Area:** backend, frontend, protocol

## Context

The five-minute stall watchdog normally reports advisory inactivity. A prompt
that has not produced any genuine agent event is different from a prompt that
started work and then became quiet. Without this distinction, a failed launch
can remain `RUNNING` and the user cannot recover it from the task view.

## Decision

Keep ordinary post-activity stalls advisory. When a current watchdog snapshot
shows no genuine turn event after the prompt was accepted, Kandev records the
existing launch-failure outcome and moves the session and task to `FAILED`.

The snapshot carries a prompt activity epoch. The orchestrator validates the
execution, prompt generation, and activity epoch before it applies the failure.
The terminal message uses settled error rendering and has no running-only cancel
action. Running notices keep their existing neutral cancel action.

## Consequences

Failed launches become visible and recoverable without waiting for a process
restart. A legitimate provider that emits no event before the threshold can be
classified as a failed launch, so the rule is limited to zero-event prompts and
does not change recovery for turns that already produced activity.

The lifecycle and orchestrator must keep the activity epoch in memory and must
reject stale watchdog events. Frontend rendering must not hide a terminal error
because it carries historical running metadata.

## Alternatives Considered

- **Keep every stall advisory.** Rejected because a never-started prompt can
  remain `RUNNING` forever with no user-visible terminal result.
- **Fail every quiet prompt.** Rejected because long-running tools can be quiet
  after they have already started.
- **Use only prompt generation.** Rejected because an agent event can arrive
  after the watchdog snapshot and before the bus consumer handles it.
