# ADR-2026-11-01-claude-async-task-lifecycle-capability: Settle Claude background shells from the bridge's task lifecycle

**Status:** proposed (blocked)
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

Advertise `asyncTasks` only when the connected bridge advertises it, and consume
only terminal state for a workload already recognized by `toolCallId`. Ignore
stop controls, spawn cards, and progress.

Adoption depends on a recorded capture proving that the capability leaves
transcript content, turn completion, and prompt admission unchanged. If the
capture fails, do not advertise it. Rows then settle from process-end evidence only.

## Consequences

- Detached shells settle with their reported outcome while the agent is alive.
- Kandev depends on an AIR-namespaced extension that the bridge can change.
  Negotiation keeps an older or changed bridge on process-end settlement.

## Alternatives considered

- **Retire the oldest open row on `background_complete`.** The signal is
  unattributed and follows Monitor turns too, so it could end the wrong job.
- **Treat the status-less tool update as completion.** Its likely source is the
  PostToolUse hook, which reports the tool result and not the end of the job.
- **Process-end settlement only.** This is safe, but the count stays wrong while
  a long-lived agent runs. It remains the fallback.

## Outcome

Proposed and blocked. The capture was recorded against bridge 0.81.2:
advertising `asyncTasks` makes the bridge send `async_task_spawned`,
`async_task_progress`, and terminal `async_task_state_update` frames carrying
`asyncTaskId` and the launch tool's `toolCallId`; without it, none are sent.
The capture also shows that the capability adds a task-notification follow-up
turn when the job completes, so adoption still needs a product decision on that
transcript/turn difference.

The remaining blocker is the pinned `acp-go-sdk`, which cannot represent
`async_task_state_update`: an unknown discriminator is decoded as an empty
`user_message_chunk`, losing the identifiers before the adapter can read them.
Adoption resumes when the SDK exposes the variant (or the adapter gains a safe
raw-frame path). Until then, process-end settlement stays authoritative (see
`REQ-AGENTS-BACKGROUND-WORK-010.1`), and `010.2` is not yet delivered.
