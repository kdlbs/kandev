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

## Readiness after admission

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
