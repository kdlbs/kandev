---
status: draft
system: agents
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
created: 2026-09-28
updated: 2026-09-28
owners:
  - cfl
---
# Repeated Unclassified Fallback Design

## Purpose and boundaries

The agent system owns candidate selection and durable route state.
This design extends [dynamic routing](dynamic-agent-routing-01.md), with a narrow exception to the shared provider policy.
The [decision](../../../decisions/2026-09-28-repeated-unclassified-fallback.md) records the accepted boundary.
The [implementation package](../../../plans/dynamic-unclassified-fallback/plan.md) and its three work orders are complete.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002` | Configuration, Evidence admission, Streak lifecycle, Integration, Compatibility and presentation |

## Configuration

Extend each candidate's `routingpolicy.Document` and `dto.DynamicAgentPolicyDTO` with optional `unclassified`:

```json
{"enabled": true, "consecutive_failure_threshold": 3}
```

Retain document version 1 for this additive field. Old readers fail closed by ignoring the extension.
Absent or null means disabled. Canonical disabled form uses `enabled: false` and threshold zero.
Enabled thresholds are integers from 2 through 10. Reject disabled nonzero thresholds and malformed sections.
The sample threshold is illustrative, not an enabled default.
Backend DTO normalization and `routingpolicy.ValidateDocument` must use matching validation rules.
Legacy rule conversion never enables this extension.

Add `disable_unclassified_fallback` to workflow step models, requests, export definitions, and event payloads.
Use a non-null database boolean with false default, through the repository's existing dialect-compatible migration mechanism.
Update requests use presence semantics: omitted preserves, false clears, true sets, and explicit null is invalid.
Old imports omit the field and receive false. Export/import and repository synchronization retain true and false.
Step read failures block admission. A task with authoritatively no step has no step veto.

This is saved candidate policy, not a new runtime feature flag. Existing dynamic-routing availability gates remain authoritative.

## Evidence admission

Do not set `routingerr.Error.FallbackAllowed` globally for unknown errors.
`routingpolicy.Evaluate` currently gates both semantic eligibility and safety through that flag.
Add a separate typed failure context for the exception, with a zero value that refuses admission.
The conductor and orchestrator supply the context from their trusted launch or terminal-event boundary.
Client-submitted events and diagnostic prose cannot supply it.

The context carries execution scope, session, step, route generation, concrete profile, attempt identity, phase, and effect evidence.
Prompt attempts use concrete execution ID plus nonzero prompt generation.
Pre-execution startup attempts receive a unique attempt ID before launch and retain it through all error wrappers.
No attempt ID can be synthesized at error-consumption time.

Eligible origins are:

- `unknown_provider_error` from an authoritative terminal provider result, before any output or effects.
- `agent_runtime_error` from a proven agent process-start or session-initialization failure, before prompt dispatch.

A generic task launch error, a missing phase, or an arbitrary wrapped `errors.New` is not proven agent startup failure.
The startup boundary must distinguish agent process/session initialization from executor, repository, and task preparation.
All other unclassified codes remain excluded in this version.
User denial, cancellation, managed-runtime policy errors, and resume corruption remain vetoes even during startup.
Known transient and hard errors continue through the existing evaluator without altered flags or classification.
Low semantic confidence alone is not proof of unsafe execution, but missing or conflicting provenance always refuses admission.

Task ownership must positively exclude Office runs and utility invocations.
Do not infer task scope only from a session ID or the absence of an Office field in a partial DTO.
Revalidate the current step, route generation, and concrete candidate at the durable transition boundary.
Use the current cancellation/settlement owner so a step change or stop cannot race a successor launch.

## Matching identity

Count identical diagnostics, not merely identical classes or candidate IDs.
A versioned fingerprint includes code, trusted origin, phase, provider identity, and the complete diagnostic text after whitespace normalization.
Retain the case and punctuation. Do not strip timestamps, paths, IDs, or variable suffixes to manufacture equality.

Before hashing, apply the existing diagnostic sanitizer and bounds.
Only a nonempty diagnostic of at most 1024 UTF-8 bytes that requires no redaction or truncation can qualify.
The producing boundary must attest completeness before a lossy projection.
An already sanitized projection without this attestation cannot prove identity and remains manual.
Persist only the SHA-256 fingerprint and bounded structured identity fields, never raw diagnostic text.
This deliberately sacrifices coverage when diagnostics contain sensitive or unstable identifiers.

## Streak lifecycle

Extend `dynamic.PolicyState` in `RouteState.PolicyStateJSON` with a versioned streak record:

- concrete candidate and logical profile IDs, profile version, and step ID,
- fingerprint and count, bounded by the configured threshold,
- last accepted attempt identity and its route generation.

Missing old JSON means no streak. Malformed streak data fails closed.
The record belongs to one task session, not to shared health or candidate configuration.
A failure can extend the record only after all currentness and safety gates pass.
A different eligible fingerprint starts a new count of one.
A duplicate attempt or stale generation changes nothing.
A current ineligible failure clears the count and enters manual recovery.

Below threshold, persist the count with `action_required`. Do not create a timer.
An explicit retry of the same candidate preserves the count across the route generation change.
A successful process launch alone does not clear a streak for prompt failures.
Successful turn completion, output, or tool activity clears it through the same current-attempt fencing.
For a startup streak, successful initialization clears it before prompt dispatch.
Explicit stop, candidate/profile/step changes, profile-version changes, policy disabling, or step veto clear it.

At threshold, claim the next candidate once, in configured order, using the existing route transaction.
Do not wrap, open a shared circuit, or transfer the count to the successor.
Persist continuation and attribution before downstream launch, as current routing requires.
If selection, persistence, or continuation fails, expose manual recovery without launching an unrecorded successor.

Restart restores committed counts but never reconstructs missing in-flight safety evidence.
Only a new attempt can extend the streak after restart.
Manual recovery must preserve the record in both selection and action-required repair paths.
Generation-and-status compare-and-swap protects concurrent handling of the same failure.
Repeated delivery while already `action_required` must not increment again.

## Integration

`dynamic.Engine.preparePolicyFailure` owns streak evaluation and policy snapshots.
`applyPolicyFailure` owns fenced persistence and successor selection.
Extend `Conductor.nextAfterLaunchFailure` so eligible unknown failures reach evaluation before the current `ActionFor`/`FallbackAllowed` shortcuts reject them.
Extend `RouteAfterFailure` and `ProfileExecutionResolver` to carry typed context, without permissive defaults for existing callers.

`orchestrator/dynamic_evidence.go` remains the authority for prompt output/effect evidence.
The backend route-action handler preserves a same-candidate streak through manual retries.
Workflow step gates must reach both synchronous startup failures and asynchronous terminal failures.
Existing callers without an eligible context retain their current behavior.

## Compatibility and presentation

Workflow storage, task-side step projections, import/export, copy/sync paths, HTTP/WS handlers, and `stepevents.payload` retain the optional veto.
Use the existing profile-session-policy fields as a propagation reference, including frontend step event types and state merging.
Candidate policy API serialization, browser normalization, editor drafts, and save requests preserve the new section.
A browser edit of unrelated settings must not disable an API-configured policy.

No rendered controls or layout changes are planned. Desktop and phone use existing manual recovery and route-change presentation.
Use the existing `policy_skip` path and structured unknown error code. Do not manufacture a classified provider reason.
Existing chat state, composer contents, and historical provider attribution remain intact.
Public documentation must explain API configuration, threshold counting, the workflow veto, and the conservative eligibility limits when implementation ships.

## Verification boundaries

Pure policy tests prove defaults, bounds, matching, and threshold decisions.
Engine/repository tests prove restart, resets, deduplication, and generation/status races.
Conductor/orchestrator tests prove actual evidence admission, not caller-supplied true booleans alone.
API and workflow tests prove round trips and veto propagation.
Desktop and mobile E2E seed the policy through the API and exercise existing recovery controls.
