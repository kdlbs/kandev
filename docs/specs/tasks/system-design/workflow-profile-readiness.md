---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
---

# Workflow profile readiness

## Purpose and boundaries

The task system owns workflow session reuse and readiness after a completed turn.
This design extends [session lifecycle](workflow-profile-session-lifecycle.md).
The agent conductor retains provider selection, candidate validation, and concrete model enforcement.
The [agent requirements](../../agents/requirements/dynamic-agent-routing.md) own that contract.
This design preserves the existing Dynamic routing decision. It adds no routing authority.

## Requirement mapping

| Criteria                                            | Design section      |
| --------------------------------------------------- | ------------------- |
| AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1, 001.5     | Profile validation  |
| AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11, 001.14   | Completion recovery |
| AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1, 001.3, 001.7 | Profile validation  |

## Components and responsibilities

`event_handlers_workflow.go` owns profile reuse checks and transition preflight.
`handleAgentReady` owns the completed-turn boundary under existing session guards.
`setSessionWaitingForInput` owns state persistence and publication.
The concrete lifecycle resolver rejects virtual profiles through `ErrVirtualProfile`, re-exported by the `agent/runtime` facade for higher-level callers.
The Dynamic conductor owns concrete execution identities and launch policy.

## Data and contracts

Logical profile identity remains on the task session. Concrete execution identity remains on the existing route and runtime records.
A Dynamic logical profile has no concrete model for comparison with a provider runtime snapshot.
A wrapped `ErrVirtualProfile` distinguishes that case from missing profiles and repository errors.
No new API, event, database field, or flag is required.

## Profile validation

In `sessionHasUnauthorizedExactModelDrift`, handle the typed virtual-profile result as no logical exact-model drift.
Use `errors.Is` rather than text matching. Keep every other resolver error fatal to the transition.
Keep the concrete resolver rejection intact. Do not make virtual profiles launchable.

The shared drift helper serves preflight, current-session reuse, and parked-session selection.
All these paths must use the same distinction. Concrete profiles retain existing exact-model checks, including unknown persisted models.
The change must not select a candidate, reset a route, or compare unrelated provider models.
Missing and disabled candidates retain their existing conductor validation and recovery behavior.

## Completion recovery

A failed destination preflight must leave a completed source conversation promptable.
For `on_turn_complete`, settle the still-owned completed session through the existing waiting-state helper before returning the rejection.
Use the current lifecycle guard and execution/turn ownership checks. Preserve the original rejection for diagnostics and caller error handling.

The shared transition helper also handles manual moves, turn start, and guarded decisions.
Do not settle these callers merely because their preflight fails. In particular, a manual move can fail during a live turn.
A newer turn, replaced execution, or terminal session must retain its state.
Do not infer cancellation from guard ownership.

The transition must not commit, retire the source, create a destination prompt, or consume a successful-transition marker after failed preflight.
The already-completed turn remains complete. Duplicate READY or complete events must not repeat entry actions or disturb a later turn.
Complete-stream handling must not depend on a second READY event that will never arrive.

## Failure and recovery

The existing error channel reports the validation failure. The source remains primary on its current workflow step.
The existing session update makes the composer ready on desktop and phone. Reload reads the same persisted state.
A later message starts a normal turn. No cancellation, runtime restart, or synthetic completion is required.
Do not add a timer or weaken admission checks for genuinely active turns.

## Persistence and security

Use existing conditional state writes and transition serialization. No migration or archive rewrite is required.
Keep managed Git credential checks and profile lookup failures intact.
Diagnostics contain identifiers and sanitized errors, never prompts or credentials.

## Verification design

Use a real settings store and lifecycle resolver in at least one regression to prove the virtual-profile result.
Cover default and explicit reuse, current and parked Dynamic sessions, and concrete exact-model controls.
At READY, cover successful Dynamic transition and rejected preflight with persisted source recovery and an accepted follow-up prompt.
Cover duplicate completion and a newer turn, plus manual and turn-start rejection during active work.
A browser scenario completes a Dynamic-profile turn, enters the next step, and sends another message without cancellation.
Backend failure-path tests assert the existing state event and persisted readiness.

## Observability

Retain the preflight rejection log and session state event. No new metric is required.
Evidence must distinguish a completed turn from an active execution before reporting a stalled session.

## Related decisions

- [Dynamic profile routing](../../../decisions/2026-08-13-dynamic-agent-profile-routing.md)

## Implementation plans

- [Dynamic workflow completion](../../../plans/dynamic-workflow-completion/plan.md)
