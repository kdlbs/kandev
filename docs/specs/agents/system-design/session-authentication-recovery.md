---
status: draft
system: agents
created: 2026-11-12
requirements:
  - REQ-AGENTS-SESSION-AUTHENTICATION-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
owners:
  - Kandev
---

# Session authentication recovery design

## Purpose and boundaries

Agents owns the interpretation of provider authentication evidence and its recovery presentation.
Tasks retains durable errors, exact-attempt admission, native identity, and workspace ownership.
Platform retains authenticated delivery and journal ownership.

The [session recovery design](session-recovery-failures.md) and
[startup explanation design](session-startup-failure-explanations.md) remain the shared recovery authorities.
This design adds authentication evidence to those paths without another recovery store or UI owner.
The [implementation package](../../../plans/cursor-startup-authentication-recovery/plan.md) records incident-specific evidence, work orders, and verification.

## Requirement mapping

| Requirement | Criteria | Design sections |
| --- | --- | --- |
| REQ-AGENTS-SESSION-AUTHENTICATION-001 | .1, .5 | Protocol evidence; lifecycle propagation; safety |
| REQ-AGENTS-SESSION-AUTHENTICATION-001 | .2, .3 | Recovery projection; durable reads; login guidance |
| REQ-AGENTS-SESSION-AUTHENTICATION-001 | .4, .6 | Retry and identity; desktop and phone |
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006 | .5, .8, .13, .17, .24-.26, .34 | Durable reads; shared card; safety |
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007 | .1, .2, .5 | Retry and identity; ordering |

## Protocol evidence

The ACP adapter receives a typed `*acp.RequestError` before the agentctl WebSocket response flattens it into text.
For supported ACP session operations, code `-32000` establishes authentication-required semantics.
The value is an ACP code, not an HTTP status or a generic JSON-RPC internal error.
Recognition uses `errors.As` at the actual operation boundary and retains nested Go wrappers.
It never searches arbitrary provider `Data`, MCP errors, or quoted diagnostic text for a matching code.

Add one shared session-failure projection in `internal/agentctl/types`.
Carry it in optional `ws.ErrorPayload.Details.session_failure` for `agent.session.new`, `load`, `resume`, and `reset`.
Its only supported cause in this change is `authentication_required`.
The payload contains that fixed code and an optional bounded snapshot of advertised authentication methods.
The enclosing response action and pending request retain operation identity.
Existing restore-required/blocked response codes and details retain precedence for their own conditions.
Authentication cannot become proof that native state is missing or authorize history continuation.

Use the adapter's cached initialize methods through a narrow optional accessor.
Do not require an already-created native session or change the base adapter interface for every transport.
An absent accessor or method list produces code-only evidence.
Keep the identity-free ACP `agent_capabilities` snapshot and connection-scoped `auth_required` event as supplementary signals.
Neither has native session identity, so agentctl forwards them as ephemeral startup control notifications without retaining them as agent output.
Lifecycle routes these snapshots before durable harness admission; a matching typed session-operation response remains the durable authentication authority.
If either event carries a session or durable delivery identity, it keeps the existing journal, ownership, and receipt checks.

ACP MCP attachment attempt metadata is also held until the session operation has an outcome.
A typed authentication rejection emits no attempt or configuration events because a failed first `session/new` has no committed native generation to own them.
On successful operations, the adapter emits the attempt preamble before its result evidence; other transports publish the same preamble at the API boundary.
This keeps MCP history ordered without bypassing durable generation, owner, cursor, or receipt validation.

The client validates this optional projection only for its matching session-operation response.
It constructs a typed error that preserves the existing diagnostic through `Unwrap` for backend consumers.
Unknown, malformed, oversized, or unsupported fields cannot create authentication evidence.
Older peers without the projection retain existing compatibility behavior; no parser of arbitrary nested error prose is added.

## Lifecycle propagation

`bootstrapFailureFor` consumes the typed operation evidence before the unknown fallback.
An authentication failure produces the existing bootstrap code `authentication_required` and fixed text `Agent authentication is required.`
`SafeAgentErrorCause` remains the durable cause boundary.
Context cancellation, terminal-session handling, timeout, supported selection evidence, and process-exit evidence retain their existing precedence.
Contradictory or stale evidence cannot replace those outcomes.

Carry the safe method snapshot with the failed operation through the runtime error to orchestration.
Keep methods outside `AgentErrorCause`; its existing cause count and evidence budget remain unchanged.
Both `handleAgentStartFailed` and executor `bootstrapFailureCause` consume the same safe bootstrap cause.
Preserve the initial `start` versus restoration `resume` operation.
Keep the outer bootstrap category `generic_launch_failure` where the task error contract requires it.
Use `routingerr.CodeAuthRequired` at routing boundaries rather than treating it as the bootstrap cause code.

## Recovery projection and durable reads

Use one authentication predicate over validated event evidence when constructing recovery metadata and actions.
The predicate accepts the normalized bootstrap cause or existing authoritative routing code.
It must not require authentication words to survive in `ErrorMessage`.
Keep existing text-only compatibility detection confined to legacy inputs without contradictory typed evidence.
An established auth decision in `handleAgentStartFailed` must reach recovery even without `ManagedRuntimeStartupError`.

Persist `last_agent_error.causes` with `authentication_required`, safe detail, phase, stamp, execution, and attempt identity.
Persist `is_auth_error: true` in the matching recovery transcript metadata.
Prefer the validated method snapshot from the failed response; use existing session capability lookup only for that same execution.
Missing methods never clear the authentication flag.
Project a provider name from the configured session/profile identity, never from the nested error message.

`persistLastAgentError`, `persistRecoveryStatusMessage`, boot/session reads, and task summary hydration retain the same cause.
Methods remain optional metadata, so no SQL migration is required.
After a repository reopen, reconstruction must use persisted safe evidence rather than a process-global error or capability cache.
Legacy unknown records are not rewritten or backfilled.

## Ordering and ownership

The request response belongs to its authenticated agentctl client and captured launch attempt.
It does not require native session creation to succeed.
Reuse cancellation, terminal-session, exact-execution, and stale-attempt guards before persistence and cleanup.
Do not bypass journal generation, owner, cursor, or active-submission checks to admit an early authentication event.

Tests cover event-before-response, response-before-event, no event, duplicate event, and event after terminal cleanup.
Each order must converge on one owned recovery outcome.
A successor attempt or another session remains unchanged by late evidence.
Retain the adapter's current empty-method event behavior; response classification does not depend on event emission.

## Login guidance, retry, and native identity

Reuse `SessionRecoveryCard`, `ActionMessageDetails`, `AuthMethodsPanel`, and `GenericAuthPanel`.
The active card needs a localized provider authentication summary before its technical disclosure.
Derive authentication from the normalized durable cause while matching transcript metadata is still loading or absent.
Pagination cannot remove the generic guidance or active retry controls.

A structured terminal method shows its bounded command as copyable text with execution-host instructions.
A command may launch only through a terminal validated for the same executor environment and live execution owner.
`HostShellDialog` starts on the Kandev backend host, so the authentication recovery surface does not use it to run an executor command.
Until an owned execution terminal is available, terminal methods remain copy-only.
A protocol-only method such as `cursor_login` is not a shell command.
Keep useful provider method identity, but show generic login guidance when the current execution cannot perform that method.
The existing authenticate endpoint may be used only with its live owned client and advertised method.
Do not expose a control that calls a released execution, concatenate method IDs into commands, or auto-authenticate during render or reload.
No new authentication continuation API is introduced by this correction.

The reported Cursor diagnostic requests CLI login and ACP authentication.
Guidance must retain that prerequisite without claiming that creating another conversation logs the user in.
Use disposable protocol fixtures to exercise the existing supported authentication call with `cursor_login` unchanged.
After the fixture's user-driven login state changes, an explicit retry must reach authoritative readiness before the card retires.
Native Cursor account repair remains a separate validation observation, not a claim inferred from a fixture.

Authentication changes base retry eligibility and labels inside the shared recovery-choice pipeline.
Keep its inspection-busy handling, read-only workspace restore, branch recovery, history continuation, and guard filtering.
Do not return a reduced authentication-only set before that shared choice handling.

Resolve Resume eligibility from validated evidence: the current session's non-empty `downstream_acp_session_id`, its `metadata.acp.session_id`, or an explicit positive resume-token projection.
These positive signals preserve Resume when matching transcript metadata is unavailable.
When a current-session snapshot is available, absence from both canonical native-identity fields confirms there is no native conversation.
When that snapshot is unavailable, an explicit negative resume-token projection confirms that Resume is unavailable.
If identity eligibility remains unknown, do not offer the identity-resetting restart by default; retain only explicitly supplied safe choices.
Positive identity evidence takes precedence over a conflicting negative transcript flag.
`buildRecoveryActions` uses authentication-aware restart copy for a failed creation with confirmed absence of native identity.
Supported restore failures retain their native identity and eligible Resume action.
Fresh start retains its existing identity-reset effect and confirmation policy.
All retries use the prepared workspace and existing admission paths.
Request acceptance, terminal completion, or a missing error row alone does not establish provider readiness.

An authentication-rejected first start can leave the task session in `WAITING_FOR_INPUT` while the bootstrap error remains active.
That state is eligible for initial-submission replay only when the durable cause is typed `authentication_required` for `start` and the session has no native conversation identity.
Generic waiting sessions and sessions with provider identity retain their existing replay eligibility rules.

## Desktop and phone

The closest shipped exemplar is the inline `SessionRecoveryCard` with `RecoveryActions`.
Desktop keeps a wrapping row of eligible controls; phone keeps full-width stacked controls in the Chat composer region.
Authentication guidance stays in that card, before technical details, without a new overlay or route.
The card retains its bounded `50dvh` scroll owner, safe-area wrapper, and separate transcript scroll region.
Use shared control sizing: 28 pixels for ordinary desktop controls and at least 44 pixels for phone/coarse-pointer controls.
Existing compact authentication controls need scoped sizing correction when reused here.

The localized summary identifies Cursor only when the configured identity confirms Cursor.
It states that authentication is required, without guessing whether a token is missing or expired.
Update English, Portuguese, Simplified Chinese, Traditional Chinese, Japanese, Korean, and pseudo catalogs for new copy.
Generate Traditional Chinese variants with the repository tooling.
Do not translate protocol identifiers or copy raw provider errors into labels.

## Safety and observability

Keep at most eight authentication methods and a combined 4096-byte projected method budget.
Validate IDs and structured fields, sanitize before bounds, and omit unsafe fields or methods.
IDs have a 128-byte bound; names and command fields have a 256-byte bound; descriptions have a 512-byte bound.
Terminal arguments share the total budget and existing terminal command trust rules.
Unsafe terminal metadata must fall back to generic guidance, without partial secret-bearing commands.
No automatic command execution is added.

The durable cause remains fixed host-authored text.
Preserve current detail sanitization and display/copy equivalence.
New structured logs can carry the bounded cause code and existing correlation fields, never method payloads or credentials.
No new metrics, flags, database tables, generic error registry, or raw provider payload fields are needed.

Existing decisions remain authoritative:

- [Provider classification before policy](../../../decisions/2026-08-17-provider-error-classes-and-policies.md)
- [Active recovery ownership](../../../decisions/2026-09-20-active-session-recovery-owner.md)

The existing boundaries and this scoped design preserve the rationale; no separate architectural decision is required.
