# ADR-2026-10-03-transient-turn-runtime-lifetime: Separate transient turn failure from ACP runtime lifetime

**Status:** accepted
**Date:** 2026-10-03
**Area:** backend, frontend, protocol

## Context

Codex reported a model-capacity error through an initialized ACP conversation.
Its prompt RPC settled while the process remained connected.
Kandev converted the error into terminal execution failure and stopped the process.
The user then needed another agent launch to continue.

The user requires a transient error to preserve a usable ACP session.
The [provider recovery decision](2026-08-08-provider-neutral-agent-error-recovery.md) already limits replay through current-invocation and effect evidence.
The [continuation decision](2026-10-02-safe-interrupted-conversation-continuation.md) adds safe native continuation under a separate experimental policy.
Neither replay refusal nor continuation refusal establishes that ACP itself failed.

## Decision

Treat transient turn outcome and runtime usability as separate facts.
A settled transient provider failure does not stop an initialized, connected ACP runtime solely because the turn failed.
Typed adapter evidence and current lifecycle ownership establish usability.
Error classification alone does not establish it.

Preserve the execution, process, connection, native conversation, and settings for later human input.
Fail and explain the interrupted turn.
Keep automatic replay and continuation under their existing safety and ownership contracts.
When an eligible automatic operation uses a preserved runtime, it does not restart that runtime first.

Actual process exit, broken ACP transport, failed initialization, explicit lifecycle actions, and bounded cancellation escalation retain existing teardown authority.
Unsupported or missing evidence retains conservative existing behavior.
Initial delivery applies to concrete-profile interactive ACP sessions.
Office, dynamic routing, passthrough, utility, and automation policy remain outside this amendment.

## Consequences

Capacity errors can leave a usable composer and a truthful failed-turn entry.
Existing replay fences still prevent repeated writes and uncertain tool work.
Runtime slots and resources remain occupied while their preserved process remains alive.
Lifecycle and orchestration need a distinct failed-turn event and exactly one settlement owner.
Remote version skew must preserve conservative behavior when evidence is absent.
Tests must assert real runtime identity, not only the absence of duplicate startup messages.

## Alternatives considered

- Stop and restore every transient error: loses a usable connection and makes temporary capacity failures require runtime recovery.
- Treat the error as successful completion: can advance workflows and report incomplete work as successful.
- Remove replay safety checks: can repeat writes and uncertain operations.
- Preserve every error based on message text: can admit work on a dead process or uninitialized session.
- Add a provider-specific orchestration branch: duplicates protocol interpretation and recovery policy across consumers.

## Related contract

- [Requirements](../specs/platform/requirements/transient-turn-runtime-continuity.md)
- [System design](../specs/platform/system-design/transient-turn-runtime-continuity.md)
- [Fix package](../plans/transient-turn-runtime-continuity/plan.md)
