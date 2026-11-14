---
status: draft
system: agents
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
---

# Session recovery guidance

## Boundary and requirement mapping

Agents owns recovery eligibility and its active presentation.
This design extends [session recovery failures](session-recovery-failures.md) without adding another error-history owner.
Tasks owns the [Restart sequence](../../tasks/system-design/session-tab-restart.md).
Platform retains [delivery reconciliation](../../platform/system-design/session-recovery-convergence.md) and its ownership proofs.

| Criteria for requirement 006 | Design section |
| --- | --- |
| .2, .12, .18, .25 | Typed server guidance |
| .4, .5, .9, .16 | Request guidance lifetime |
| .6, .14, .15, .17 | Active recovery surface |

## Typed server guidance

`session.launch` and `session.recover` retain the existing `session_restore_required` conflict shape.
Its `recovery_action` is `continue_from_history`.
The bounded details include `reason`, `session_id`, and the positive current `generation`, when available.
Canonical block refusals also include optional `block_id` and `incarnation_id` fields.
These fields identify the incident when its projection arrives before the request failure.
Displaying this action grants no recovery authority.
The existing recovery endpoint repeats admission, ownership, archive, and continuation-checkpoint checks after explicit activation.

`resolveSessionRecoveryBlock` must preserve a typed classification after native proof fails.
Re-read the current block before describing that failure.
Only an unchanged, still-open block can supply its reason and generation.
A changed incident, failed read, or unknown classification must not produce guidance for the previous block.
Keep repository failures distinct from an unchanged block with incomplete evidence.
Existing concurrent authenticated resolution remains successful.

The response mapper retains its existing four continuation reasons.
It also recognizes the canonical `unknown_prompt_outcome` block classification for explicit history continuation.
This uses the existing continuation operation, not a new discard operation.
It never converts an unknown prompt outcome into successful completion or cancellation.
Provider prose and the text of a generic error cannot establish action eligibility.

An unchanged block that refuses native Resume can therefore return the existing typed hint instead of an unclassified internal error.
An unsupported refusal retains a safe explanation and existing task-menu actions.
Do not add automatic retries or advertise an operation from an arbitrary string match.

## Request guidance lifetime

`contextContinuationDetails` remains the normalization boundary for structured responses.
Validate the literal kind and action, optional nonempty identities, and optional positive safe-integer generation.
Reject a known mismatch with the captured request target or current block.
Older replies without optional fields remain usable only within their captured, still-current request.

Use one transient continuation-guidance entry per application-store task/session pair.
Follow the existing store-owner pattern in `lib/session/session-restart-state.ts`.
Implement the small guidance model in `lib/session/session-recovery-guidance.ts`.
It stores validated continuation details and their request identity, not another copy of error history or provider diagnostics.
Both ordinary recovery and Restart publish to this owner.
`useSessionRecoveryActions` and `SessionRecoveryProvider` consume it.
Replace the hook-local continuation-details authority rather than maintaining two independent authorities.

Bind each entry to the captured task/session and current recovery identity.
Reuse the existing recovery request key and operation fencing.
For delivery blocks, bind to block ID, incarnation, expected generation, and reason.
Use `deliveryRecoveryRequestIdentity` to fence the request, not to treat every presentation refresh as another incident.
When projection precedes the failure response, matching canonical identity can associate that response with the current incident.
It cannot override a later operation or a different block.
For native-state loss without a projected block, use the captured failure stamp and request generation.
Do not infer identity from equal error text.

An entry survives menu unmount, responsive remount, and another failed request for the same unresolved incident.
A plain continuation-bootstrap failure must not erase a still-current, validated hint.
A non-retryable guard temporarily blocks the action without transferring its authority to another session.
Matching successful recovery, a superseding incident, session deletion, or discarded application-store ownership invalidates the entry.
A newer presentation revision alone does not invalidate guidance for the same still-open incident.
A late response cannot clear or replace successor guidance.
Navigation never retargets a request or steals focus from a later user selection.

This state is transient.
After browser reload, canonical session blocks and failure records remain authoritative.
A new explicit Resume can retrieve the typed hint again.
Do not add local-storage persistence or treat retained browser guidance as server authorization.

## Active recovery surface

`SessionRecoveryCard` remains the active owner for a blocked mounted composer.
`SessionStoppedBanner` remains an adapter for its existing stopped and delivery states.
Historical transcript rows retain their details without competing controls.

The uncertain-delivery branch must include a validated history-continuation choice.
Use the same continuation-choice construction as the ordinary card and stopped banner.
Do not append unrelated workspace or fresh-start operations merely to reuse a helper.
When the current hint requests history continuation, show that action first.
Retain other eligible controls and their distinct effects.
Missing profiles, archive restrictions, relocation requirements, and non-retryable guards retain their precedence.

Show a localized disclosure before activation.
It explains that saved Kandev history starts a new native conversation and does not establish the interrupted prompt's outcome.
Keep the failure visible if continuation fails.
No render, reconnect, hint publication, or Restart failure may automatically submit `continue_from_history`.

Restart catches the structured launch failure and publishes its guidance for the captured target.
Its existing loading toast reports the failed request.
The toast is not the only place that retains the suggested action.
The selected target's existing recovery region shows the guidance after the menu closes.
`SessionRecoveryProvider` can derive its request-local model from matching guidance when durable failure projection is delayed.
The [Restart design](../../tasks/system-design/session-tab-restart.md) retains selection, draft, pending, and navigation ownership.

Desktop uses the existing wrapping action row.
Phone uses the inline composer recovery region with full-width stacked actions.
The nearest exemplars are `SessionRecoveryCard`, `RecoveryActions`, and the phone Sessions picker.
Restart remains reachable through the picker's visible row-actions button.
No new overlay or navigation destination is necessary.
Expanded recovery content keeps its existing bounded scroll owner and safe-area wrapper.
Details never acquire a nested scroller.
Phone and coarse-pointer targets measure at least 44 pixels, including narrow fine-pointer phone widths.
Ordinary desktop actions retain 28-pixel sizing.

## Compatibility and verification

No schema, journal operation, new recovery action, or runtime flag changes.
The response extends an existing reason mapping and preserves legacy optional fields.
All new copy uses locale keys in en, pt-pt, zh-cn, zh-hk, zh-tw, ja, ko, and pseudo.

Component tests cover ordinary and uncertain adapters with identical guidance.
Hook tests cover Restart handoff, unmount, failed continuation, and successor identities.
Handler tests cover the canonical block reasons and unsafe error shapes.
Desktop and phone E2E cover explicit action dispatch and layout.
Transport interception proves UI routing only.
Actual recovery requires the real backend scenario described in the [delivery package](../../../plans/blocked-session-recovery-guidance/plan.md).

The backend failures in [issue 5256](https://github.com/kdlbs/kandev/issues/5256) remain an independent dependency.
They include native-proof failures and continuation generation binding.
A visible button alone cannot establish that those sessions recover.
Do not close the combined recovery outcome until that dependency and the real-backend check pass.

Existing [active recovery ownership](../../../decisions/2026-09-20-active-session-recovery-owner.md) and
[honest recovery disposition](../../../decisions/2026-10-12-total-session-recovery.md) remain authoritative.
This correction needs no new architectural decision.
