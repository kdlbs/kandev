---
status: current
system: tasks
requirements:
  - REQ-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001
---

# Peer Message Dispatch System Design

## Purpose and mapping

The task system owns peer-message admission and dispatch. This design implements
[AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1](../requirements/parent-child-message-interrupt.md)
through the routing, readiness, and failure boundaries below. It supplements the
migrated requirement's technical detail without changing its authorization rules.

## Routing and authorization

`internal/mcp/handlers.handleMessageTask` validates the sender, resolves the
recipient session, expands prompt references, and retains sender metadata.
Explicit session selection and fallback session selection remain pinned.

`dispatchTaskMessage` routes busy sessions to durable queue admission. Idle
sessions use existing turn-start, prompt, and resume operations. An explicit
interrupt remains restricted to the target task's direct parent and uses
`QueueAndInterruptForPeerMessage`; ordinary queue admission must never invoke it.

## Initial launch ownership

An accepted initial launch can still have a `CREATED` session during workspace
preparation. That row does not authorize a peer message to start another agent.
An execution identifier alone also does not distinguish workspace preparation
from an admitted agent start.

Serialize the peer-message start-or-queue decision with the existing
orchestrator session lifecycle admission. Re-read the selected session inside
that boundary. If an initial launch owns admission, retain the peer message in
the existing queue. Preserve the original brief, turn, profile, and recipient.
Do not run `prepareSessionForTaskMessage` or its rollback against that launch.

Cover admission before workspace preparation and the asynchronous bootstrap
interval after `LaunchPreparedSession` returns. Reuse current launch ownership
and runtime startup evidence. Do not use the passthrough-only initial-prompt
marker as ordinary ACP ownership. If a narrow internal admission token is
necessary, bind it to the session incarnation and retire it on every outcome.
It must not become a second scheduler or a persisted session state.

Queue insertion and lifecycle notifications must not run under a lock that
their synchronous callbacks acquire. Return a typed busy/start-owned outcome
from the guarded decision, then use existing identity-aware queue admission.
Revalidate incarnation at insertion. Keep the post-admission readiness check.
While the original prompt owns the turn, boot readiness cannot dispatch the
follow-up. The normal turn boundary makes it eligible under Auto-run policy.

At the executor boundary, `startAgentOnExistingWorkspaceWithRequest` must
recheck agent startup/running evidence before changing description, turn binding,
configuration, or session state. An active agent returns a non-destructive
busy outcome. A prepared workspace without an agent remains startable.
Treat a lost start claim as contention, not a provider startup failure.
Do not classify errors by matching their message text.

Failure and rollback writes must prove ownership at the mutation boundary.
Reuse the existing execution, startup-attempt, session-incarnation, and conditional
write mechanisms. An execution ID alone is insufficient when the same workspace
hosts multiple startup attempts. Only the failed attempt can change its error,
turn, task state, or runtime. Missing ownership fails closed for cleanup.
Never restore an old `CREATED` snapshot over a launch that advanced independently.
Keep genuine owned failures and explicit cancellation behavior intact.

This correction applies the existing lifecycle ownership and queue contracts.
It adds no public parameter, schema, timer, or parent-side retry requirement.

## Readiness after queue insertion

The session snapshot used for routing can become stale before queue insertion.
Every successful ordinary peer-message queue admission requests an automatic
readiness check with the exact admitted `QueueSessionIdentity`.

Expose the orchestrator's existing `CheckQueueAdmissionReadiness` operation on
the MCP `SessionLauncher` collaborator. Make this capability required so that
production wiring cannot silently omit it. Invoke it after queue admission
releases its locks. Queue status publication remains a separate projection;
publishing `MessageQueueStatusChanged` does not request dispatch.

The orchestrator owns all eligibility decisions. Reuse its identity-aware,
task-admission-aware drain and atomic automatic reservation. Retain Auto-run,
clarification, WIP admission, cancellation, steering, active-dispatch, and
session-incarnation guards. Never substitute the manual `DrainQueuedMessage`
operation, which enables Auto-run.

If insertion wins first, the later readiness event sees the accepted work.
If readiness wins first, the admission check sees the now-promptable session.
Concurrent checks serialize through existing reservation and dispatch guards.
The check attempts the FIFO head, which can precede the newly admitted message.
Automatic merging retains its existing behavior and surviving entry identity.

## Failure, persistence, and observability

Queue admission success is independent of a subsequent deferred dispatch.
Retain the existing `queued` response for this admission path; it does not
promise provider acceptance. A skipped readiness check or dispatch failure must
not convert committed admission into rejection. Existing dispatch restoration
and queue status events remain responsible for pending work and its projection.

Use the existing checker and dispatch lifecycle. Do not wait for agent turn
completion or add a scheduler, timer, detached worker, or new persistence layer.
The checker must preserve the accepted identity even if a session is replaced.
Existing structured dispatch logs must not include prompt content.

This design does not guarantee a response after transport loss, make unidentified
MCP submissions idempotent, or repair historical stranded rows at startup.
Those are separate contracts. No schema, public tool parameter, or UI change is
required.

## Dependencies and verification

- [Server-owned Auto-run](../../../decisions/2026-08-16-server-owned-queue-auto-run.md)
  defines automatic reservation policy, including guarded deferral.
- [Prompt generation ownership](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)
  defines turn-event ownership and per-session serialization.
- [Resume prompt queue](resume-prompt-queue.md) describes the existing browser
  producer's admission/readiness pattern. MCP reuses its orchestrator operation.
- [Queue automation controls](../../ui/requirements/message-queue-automation-controls.md)
  retains the independent Auto-run contract; no presentation change is proposed.

Deterministic tests cover both readiness/insertion orders, one accepted provider
prompt under concurrent triggers, paused queues, stale identities, and blocked
admission. A handler-to-orchestrator test must prove delivery, not just a smaller
queue count or a recorded method call.

## Implementation plans

- [MCP queued-message wakeup](../../../plans/mcp-queued-message-wakeup/plan.md)
- [Peer messages during initial launch](../../../plans/peer-message-initial-launch/plan.md)
