# ADR-2026-11-01-claude-async-task-lifecycle-capability: Settle Claude background shells from the bridge's task lifecycle

**Status:** rejected
**Date:** 2026-11-01
**Area:** backend

## Context

A detached Claude shell has no terminal evidence on the current connection.
The launch tool completes when the command detaches, and `background_complete` names no tool.
Workload rows therefore stay `running` until process-end settlement, so the
background jobs count stays wrong for as long as the agent stays alive.

`claude-agent-acp` 0.81.2 publishes `async_task_state_update`, which carries the
state and `toolCallId`, but only to clients that advertise `asyncTasks` in the
AIR section of `clientCapabilities._meta`. The bridge also advertises that
capability to clients. The pinned `acp-go-sdk` fork has no such update variant,
and advertising the capability adds other notifications and a tool-call marker.

## Decision

Reject this adoption proposal for the captured bridge behavior. Kandev does not
advertise `asyncTasks` or consume its task-lifecycle frames. Process-end evidence
remains the settlement path while the proposal is rejected.

## Consequences

- `AC-AGENTS-BACKGROUND-WORK-010.2` remains undelivered for detached shells while
  their owning Claude agent remains alive.
- The recorded capability adds a task-notification follow-up turn and message.
  Adopting that behavior requires a product decision.
- The pinned SDK cannot represent `async_task_state_update`; an unknown frame
  loses the workload identifiers before adapter handling. Adoption also needs
  SDK support or a safe raw-frame path.

## Alternatives considered

- **Retire the oldest open row on `background_complete`.** The signal is
  unattributed and follows Monitor turns too, so it could end the wrong job.
- **Treat the status-less tool update as completion.** Its likely source is the
  PostToolUse hook, which reports the tool result and not the end of the job.
- **Process-end settlement only.** This is safe, but the count stays wrong while
  a long-lived agent runs. It remains the fallback.

## Outcome

Rejected for this proposal. The capture was recorded against bridge 0.81.2:
advertising `asyncTasks` makes the bridge send `async_task_spawned`,
`async_task_progress`, and terminal `async_task_state_update` frames carrying
`asyncTaskId` and the launch tool's `toolCallId`; without it, none are sent.
The capture also shows that the capability adds a task-notification follow-up
turn when the job completes, so adoption still needs a product decision on that
transcript/turn difference.

The capture shows both a protocol implementation blocker and a product decision
point. The pinned `acp-go-sdk` cannot represent `async_task_state_update`: an
unknown discriminator is decoded as an empty `user_message_chunk`, losing the
identifiers before the adapter can read them. The capture also adds a
task-notification follow-up turn. A future proposal may reopen after SDK support
or a safe raw-frame path is available and product approves that transcript/turn
behavior. Until then, process-end settlement stays authoritative (see
`REQ-AGENTS-BACKGROUND-WORK-010.1`), and `010.2` is not delivered.
