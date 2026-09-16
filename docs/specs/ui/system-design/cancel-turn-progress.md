---
status: current
system: ui
requirements:
  - REQ-UI-CANCEL-TURN-PROGRESS-001
---

# Backend-owned Cancel-turn Progress System Design

## Purpose and boundaries

This design keeps the cancel control tied to the cancellation operation that the backend accepts,
instead of to the browser component that requested it. The UI system owns the shared control's
rendering and hydration behavior. The orchestrator and agent runtime retain ownership of execution
cancellation, session lifecycle, and prompt termination.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-CANCEL-TURN-PROGRESS-001` | [Runtime projection and client contract](#runtime-projection-and-client-contract), [Control flow](#control-flow), [Responsive behavior](#responsive-behavior), [Verification design](#verification-design) |

## Runtime projection and client contract

The orchestrator exposes a per-session `cancellation_pending` value and monotonically increasing
`cancellation_revision`. Task-session boot and REST payloads, subscription snapshots, and
`session.cancellation_changed` notifications carry both values. Clients merge a notification only
when its revision is current, so a delayed hydration response cannot replace a newer transition.

The shared chat cancel control combines this backend state with a short-lived local request flag.
The local flag supplies immediate feedback before acceptance; once the backend accepts, the runtime
projection is authoritative across remounts, route changes, and replacement pages. The projection
is not a durable session lifecycle state and is not persisted across backend restart.

## Control flow

1. A user activates the existing `agent.cancel` action for a running session.
2. The backend guards the per-session cancellation operation, publishes pending, and starts or joins
   lifecycle cancellation.
3. The current client and any later task-session hydration render the disabled loading control from
   the projection.
4. After lifecycle and session reconciliation settle, the backend publishes a false value with the
   next revision; the shared control returns to its ordinary state.

The mock agent recognizes `/e2e:cancel-hold` only in E2E mode. Its prompt waits at one predictable
cancellation boundary and produces no assistant output, allowing browser regressions to observe the
backend-owned pending operation without intercepting a browser request. Managed E2E startup carries
the profile-owned `PromptCancelJoinTimeout` in private `AgentctlStartupConfig`, through the process
adapter, to ACP. A zero value leaves ACP's normal three-second join bound in effect.

## Responsive behavior

Desktop task navigation remounts the same shared cancel control after returning to a task. On mobile,
the compact composer hydrates the same session projection after a full reload. Both surfaces preserve
their existing composition, controls, and touch behavior; no viewport-specific cancellation state or
new user-facing copy exists.

## Failure and recovery

If a cancel request is rejected before backend acceptance, the local request flag clears and no
pending projection is published. A missed live event is repaired by the next boot, REST, or
subscription snapshot. On success, reconciliation, rejection, or a bounded cancellation timeout,
the backend releases pending so a still-running session is retryable.

The E2E hold is scoped to test profiles. Production and non-E2E managed launch paths retain their
existing cancellation timing and ACP default join bound.

## Verification design

Mock-agent tests prove the hold does not emit assistant output. Configuration and process tests prove
the managed E2E timeout reaches ACP and that zero keeps the default. Desktop Chromium repeats prove
the disabled loading control remains visible through task navigation; mobile Chromium repeats prove
the same behavior through reload hydration. Both flows wait for the authoritative pending value and
eventual settlement rather than using a browser delay.

## Related decisions

- [ADR-2026-08-03: Keep Cancellation Progress Backend Owned](../../../decisions/2026-08-03-backend-owned-cancellation-progress.md)
