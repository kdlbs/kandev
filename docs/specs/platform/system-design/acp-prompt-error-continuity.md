---
status: current
system: platform
created: 2026-10-29
updated: 2026-10-29
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-004
  - REQ-PLATFORM-TURN-CONTINUITY-005
owners:
  - Kandev
---

# ACP prompt error continuity

## Purpose and ownership

Platform owns the shared lifetime boundary between a failed prompt and a usable ACP runtime.
This extends the [existing design](transient-turn-runtime-continuity.md) across ACP providers and prompt error classes.
The [decision](../../../decisions/2026-10-29-acp-prompt-error-runtime-lifetime.md) records why protocol origin replaces an error allowlist.

| Requirement | Design boundary |
| --- | --- |
| REQ-PLATFORM-TURN-CONTINUITY-001 | Runtime proof and lifecycle settlement |
| REQ-PLATFORM-TURN-CONTINUITY-002 | Failed-turn history and desktop/phone Chat |
| REQ-PLATFORM-TURN-CONTINUITY-004 | Response provenance, provider-neutral retention, ownership, compatibility, and verification |
| REQ-PLATFORM-TURN-CONTINUITY-005 | Durable task attention, acknowledgement, and sidebar/Chat/task-menu presentation |

Scope covers concrete-profile interactive task sessions on ACP, including task-backed Quick Chat.
Quick Chat receives the same runtime retention and durable task attention.
It exposes inline Dismiss and observed same-session prompt acknowledgement, without new normal-sidebar rows or task menus.
Office, dynamic routing, automation, utility calls, passthrough, and native Codex app-server retain their current ownership policies.
Startup, restore, configuration-operation, and explicit stop errors keep their existing owners.

## Evidence and defect

OpenCode 2.2.0 logged a lock timeout at 2026-10-29T20:32:55.769Z.
Its cause chain ended in `SQLiteError: database is locked` while the session runner published an event.
At 20:32:58 UTC, Kandev marked execution `d3410a71-406f-417a-8755-71ec95a3ba4e` failed.
The session later restored at 21:18:43 UTC with the same saved native identity.
Restoration proves saved conversation continuity. It does not prove that the original process accepted another prompt.

`Adapter.retainableACPApplicationError` currently requires selected provider identities and high-confidence transient classification.
OpenCode additionally requires `RuleOpenCodeCapacityAdmission`.
Lifecycle applies another provider-name allowlist in `isRetainableACPAgent`.
A generic error therefore becomes terminal even when the transport can remain usable.

The permanent regression first returns a generic prompt error through the real SDK wire path.
It then submits a distinct follow-up on the same adapter and verifies process, connection, execution, and native session identity.
No live user database or process participates in reproduction.

## SDK response provenance

`apps/backend/go.mod` pins the maintained `github.com/kdlbs/acp-go-sdk` fork through a replace directive.
The baseline revision is `48a57e4292e8`.
Its `RequestError` also represents local send failures, result decoding failures, expired contexts, and peer disconnects.
The fork already provides `ErrPeerDisconnected`, but that sentinel does not identify every local failure.

Add `(*RequestError).IsRemoteResponse() bool` to that fork.
Use an unexported, nonserialized field. Its zero value is false.
Set the field only when `Connection.handleResponse` accepts an actual error envelope for a pending request.
A valid matching request identifier and an error-only response envelope establish provenance.
Invalid identifiers, unmatched replies, notifications, and contradictory error/result envelopes never establish it.
`SendRequest` still waits for its ordered notification watermark before it returns that error.

Local constructors and conversion helpers retain false provenance.
Marshal failures, write failures, result decode failures, context expiry, and disconnect errors remain distinguishable from remote replies.
`errors.As` can discover the marked error through diagnostic wrappers.
Neither raw `Data` nor a JSON field can set the private marker.
No new JSON-RPC field, raw frame recorder, or second line parser is necessary.

Publish a reviewed immutable fork revision and pin it before the host consumes the new method.
Do not modify the Go module cache or commit a local filesystem replace directive.
Keep the existing wire representation, `errors.Is(ErrPeerDisconnected)` behavior, and notification barriers intact.

## Shared ACP adapter

Retain the existing `retain_runtime` disposition and turn-failure event pipeline.
Replace the provider/classification gate for ordinary `session/prompt` errors with response-origin evidence.
All providers using this transport share that rule, including a previously unknown ACP identity.
Remove `acpDialect.retainedApplicationErr`, `mockRetainedApplicationError`, and their mock dialect wiring.
Remove the lifecycle name allowlist, including its mock-agent exception.
Mock errors establish origin only by passing through a real SDK response, never through Data or a dialect shortcut.
Keep separate mock continuation/capacity capabilities where automatic recovery still requires them.

The generic path requires every condition:

1. The prompt RPC returned an error with positive remote-response provenance.
2. The prompt context and adapter lifetime remain active.
3. The caller still owns the same nonzero prompt generation and initialized native session.
4. The SDK and adapter notification barriers completed.
5. The current SDK connection remains open and matches the adapter connection.
6. No cancellation, handoff supersession, or authoritative runtime failure owns settlement.

Run existing cancellation and transport-loss handling before generic retention.
An error callback for a superseded prompt cannot acquire current-turn authority.
Do not probe with another prompt, initialize another session, or infer health from diagnostic wording.

Reuse `ProviderErrorFromError` for sanitized diagnostics, including its safe fallback for empty messages.
Preserve the richer correlated Codex quota diagnostic instead of returning before the common retention path.
Raw `RequestError.Error()` and arbitrary `Data` must not enter history, generic logs, or UI.
Recognized provider metadata and remediation guidance retain their existing allowlists.

Provider-specific terminal diagnostics delivered before a successful prompt response keep their current evidence gates.
Ordinary assistant text, tool failures, and advisory retry notifications do not become terminal turn errors.
New provider stream formats require separate evidence before they affect settlement.

Capacity classification still controls `CapacityContinuationSnapshot` and automatic recovery support.
Removing the OpenCode retention gate does not make broad capacity prose or an unknown provider eligible for automatic continuation.
Keep the native retry owner exclusive until the prompt actually settles.

## Runtime and turn settlement

Lifecycle trusts the host-generated disposition with its existing execution, stream, profile, generation, and task-owner checks.
Remove the redundant provider-name allowlist for this typed ACP attestation.
Only the ACP adapter emits this generic evidence. Browser input and provider `_meta` cannot create it.
Other transports do not inherit support from a matching agent name.

Reuse `retainPromptFailureRuntime`, `AgentTurnFailed`, and the orchestrator's `handleAgentTurnFailed` owner.
Keep the execution ready and tracked without synthetic exit codes or process cleanup.
Persist one sanitized failed-turn status entry with `failure_scope=turn`, `runtime_retained=true`, and `recovery_actions=false`.
After durable settlement, expose `WAITING_FOR_INPUT` without a blocking session error.

The existing prompt settlement fence prevents successor admission before persistence finishes.
Keep failed or uncertain managed-input outcomes, delivery identity, and success-workflow suppression.
Runtime preservation does not report success, consume queued work as successful completion, or authorize original-prompt replay.

A real process exit, stream close, failed settlement publication, shutdown, reset, archive, deletion, or explicit stop retains its existing authority.
A late prompt reply cannot revive a failed or replaced execution.
A failed durable message write preserves the existing closed admission fence until an authorized owner settles the boundary.

## Automatic recovery remains independent

Reuse existing classification and recovery admission after retention.
Unknown, authentication, hard quota, invalid-model, and storage errors gain no automatic action from this amendment.
Existing supported transient replay and capacity continuation keep their provider support, safety checks, timing, and budgets.
Pending tools, permissions, and background work retain their existing input fences.
Manual continuation never promises exactly-once provider behavior.

## Failed-turn history and desktop/phone Chat

Reuse the existing inline status entry and normal composer.
Relevant surfaces include `messages/action-message.tsx`, its recovery-history helpers, task Chat, preview Chat, and Quick Chat.
Add a localized dismissal control for the current turn-error alert and a task-menu dismissal action.
Use existing Chat and menu primitives. Preserve the existing navigation and drawer composition.

The nearest shipped phone exemplar is `mobile/session-mobile-layout.tsx` with the existing retained-capacity error row.
Phone users open Chat through the task's existing conversation navigation.
The hierarchy remains transcript, inline error/details with Dismiss, then the composer and Send action.
The transcript retains one scroll owner. The composer retains its draft, safe-area, and keyboard behavior.
Details wrap inside the transcript. Existing phone actions retain their touch targets.
Desktop keeps the same inline explanation and compact controls.

Authentication and quota explanations remain actionable through existing settings or account guidance when available.
They do not become startup recovery solely because the turn failed.
Reload, pagination, and another viewer preserve each historical failure without granting runtime or retry authority.
Later genuine runtime loss can expose normal recovery without rewriting earlier turn errors.
The [plan](../../../plans/acp-prompt-error-continuity/plan.md) contains the desktop and phone previews.
Quick Chat uses its current inline surface and Dismiss only; ordinary sidebar/menu geometry requirements do not create new Quick Chat surfaces.

## Durable task attention

A usable conversation and task attention are independent facts.
Retained-turn settlement must preserve a visible error signal without creating blocking recovery metadata.

Persist one task-owned `last_turn_error` record in existing task metadata.
Include its unique stamp, source session, execution, turn, message ID, occurred time, sanitized message, and optional acknowledgement time/reason.
Use the persisted error message creation time for ordering. Replaying an existing message does not refresh that time.
Sanitize first, then truncate the stored message to 4096 UTF-8 bytes at a rune boundary.
The transcript remains the full sanitized history.
Use the idempotent failed-turn message ID as the stamp. Identical error text in another turn receives a different identity.
A later failed turn replaces the current attention pointer, while all failed-turn transcript entries remain durable.
The latest pointer represents earlier turn failures for task attention. No per-session acknowledgement inventory or browser history scan is necessary.

`last_turn_error` and `last_launch_error` are task metadata. `last_agent_error` belongs to session metadata in `task_sessions.metadata`.
Session `error_message` is a separate column. Keep these owners and recovery meanings distinct.
It cannot authorize recovery or affect runtime, workflow, foreground, pending-action, or queued-input state.
Reserve the task metadata key for the trusted failure owner.
Use the existing task metadata write admission and conditional metadata primitives, preserving unrelated task fields and title ownership.
Serialize setting and acknowledgement with task-scoped mutation ownership.
Duplicate delivery must not re-arm a dismissed stamp. A late older failure must not replace a newer accepted failure.

The manual retained-error settlement owner persists the message and attention pointer before it acknowledges foreground settlement.
Existing automatic recovery keeps its retry notice while it owns the failure.
When it falls back to a manual failed-turn entry, that entry raises task attention.
No new automatic retry policy or per-retry attention loop is added.

Extend `TaskStatusSummary` with optional `turn_error`.
Carry only the source `session_id`, `turn_id`, `message_id`, `stamp`, `occurred_at`, and a sanitized `preview`.
Sanitize first, then truncate the preview to 512 UTF-8 bytes at a rune boundary.
Bound the stamp to the existing 64-byte error-stamp limit.
Use the existing identifier and timestamp bounds for other fields. Omit acknowledged attention.
Do not add transcript payloads, arbitrary provider Data, recovery actions, or full error details to compact summaries.

The task-owned record lets the existing task-status projector derive attention without subscribing sidebar rows to session transcripts.
Extend projector input, semantic equality, persistence, startup hydration, explicit invalidation, and revision guards together.
Publish ordinary task mutation and summary events through their existing owners.
Live updates and boot/task-list snapshots must agree, including after backend restart.
Older clients ignore the optional field and retain their existing recovery behavior.
Historical transcript rows do not recreate current attention when a marker is absent.

Task rows choose existing blocking `active_error` first, then `turn_error` for their error decoration.
When both exist, the row preview describes the runtime/recovery error. Chat retains separate runtime-recovery and turn-error controls.
The new Dismiss turn error action targets only `last_turn_error`; it must not imply that a visible runtime-error badge will disappear.
Keep the existing session recovery/dismissal affordance separate. Acknowledge neither record through the other endpoint.
Activity remains independent: a task can show a running sibling and a turn-error badge.
The new field does not pass through `selectActiveSessionRecovery` or bootstrap/composer-blocking selectors.
Existing session-based local acknowledgement maps must not hide a new task-owned turn stamp.
The accepted bounded task-status contract still owns transport limits and freshness.

## Explicit acknowledgement and accepted prompts

Add an authorized, task-scoped `POST /api/v1/tasks/:id/turn-error/dismiss` action with required nonempty `stamp`.
Use the same operation for the inline Dismiss control and task context/overflow menu.
It acknowledges the currently observed task attention, without touching transcript content or independent recovery errors.
An older transcript entry cannot dismiss a later task error.
Keep the operation idempotent for an already acknowledged stamp.
A mismatched newer stamp returns a conflict with no mutation. A foreign or unavailable task follows existing access-hiding behavior.

Reuse the conditional task metadata pattern in `metadata_launch_error_cas.go` and task service authorization/event publication.
Do not perform a full task snapshot update to dismiss an error.
Set acknowledgement metadata instead of deleting the history pointer.
A client failure leaves the indicator visible and permits a later dismissal attempt.
Use existing localized error feedback. Do not hide the badge permanently through optimistic local state.

The client attaches optional `observed_turn_error_stamp` when it renders the current error for the target session before submission.
Task and target-session IDs remain explicit. Only a newly accepted manual prompt with that stamp can acknowledge its same-session error.
The backend validates the stamp and requires the attention record's source session to match the prompt target.
Queued input uses the existing admission metadata and client queue identity in `messagequeue/admission_metadata.go` and `types.go`.
Do not capture or substitute the backend's current stamp when the client omits it.
A send in session A cannot acknowledge an unseen error from session B, even when that error is task-wide.
Explicit task-level Dismiss can acknowledge the observed sibling error without sending a prompt.

Persist this acknowledgement intent with the new logical submission's identity before returning acceptance.
After durable dispatch or queue acceptance, conditionally acknowledge only its supplied stamp.
Do not wait for assistant output. A stale or missing stamp leaves attention unchanged without rejecting otherwise valid input.
Include the field in submission identity/replay validation so a retry cannot attach a newly observed stamp to an earlier accepted input.
Do not let callers overwrite trusted manual origin or change task/session ownership through this optional field.

A rejected send, draft change, queue content edit, or duplicate submission creates no new acknowledgement authority.
Input queued before a failure has no observation of that later error.
Automatic drain, retry, or redispatch retains the original acknowledgement intent and never samples the current task stamp.
New queued input accepted after the client observes an error can acknowledge only that observed same-session stamp.
A new steering prompt through `message.add` can carry fresh observation when the backend accepts that new submission.
Send now on an existing queue entry retains its original intent. Queue maintenance and drain do not count as new submissions.
Automatic recovery, workflow, agent-origin, and internal prompts do not acknowledge attention through this rule.

If another failure replaces the captured stamp, the conditional acknowledgement must preserve the newer attention.
If acknowledgement persistence fails after input acceptance, keep the attention and report the acknowledgement failure.
Do not resend the accepted prompt to repair that metadata failure.
The next distinct failure re-arms the task badge even when its text matches a dismissed error.
An accepted manual prompt acknowledges only client-observed attention from its own source session.
Independent sibling runtime failures remain visible through existing recovery projection.

Opening Chat, focusing the task, marking messages read, changing tabs, loading history, and unrelated agent output do not acknowledge this record.
This task-level acknowledgement is shared across authorized viewers, matching existing durable error dismissal.

## Sidebar, Chat, and task actions

Show the normal task error icon for current turn attention, with a sanitized preview and an accessible failure description.
Reuse `task-session-sidebar-item.ts` and shared task-row state rather than mounting transcript subscriptions.
Retain the task activity icon, workflow state, and other attention badges.

Only the current failed-turn alert owns its Dismiss control.
After acknowledgement, retire the active alert treatment and keep a readable historical error row.
A later error receives a fresh active alert and badge.
All controls use the current task ID and observed stamp. A stale menu cannot acknowledge a new failure silently.

Desktop task right-click exposes Dismiss error when only turn attention exists.
When a runtime error also exists, label the separate action Dismiss turn error to identify its narrower effect.
Use the same action in the visible task overflow menu and the shared task menu builder.
The current phone task-switcher sheet has no per-row More options control.
Task 04 adds a visible per-row trigger in `mobile/session-task-switcher-sheet.tsx` for the shared dismissal action.
Reuse the shipped touch menu primitives, not a claimed existing trigger in that sheet.
Long press can remain an optional shortcut but cannot be the only phone entry point.

Nearest precedents are `task-switcher-context-menu.tsx`, `TaskActionsMenuTrigger`, and `mobile/session-task-switcher-sheet.tsx`.
Phone navigation retains one menu scroller, safe-area clearance, focus return, and at least 44px action targets.
Desktop keeps its existing compact control sizes. Dismissing an error does not navigate or discard a draft.
New labels, accessible names, and failure feedback use all required locale catalogs and the Traditional Chinese/pseudo generators.
Plan views UI-01 and UI-03 show the alert, task badge, and menu action on desktop and phone.

## Compatibility and failure matrix

| Input | Lifetime result | Automatic action |
| --- | --- | --- |
| Proven remote prompt error on any usable ACP provider | Fail turn and retain runtime | Existing policy only |
| Generic internal, storage, auth, quota, or invalid-model error reply | Same retained-turn path | None unless independently eligible under existing policy |
| Known terminal stream diagnostic with settled RPC | Existing typed provider path | Existing provider policy |
| Local SDK send/decode error or expired context | Existing uncertain/terminal handling | No new authority |
| Actual peer disconnect or process exit | Runtime recovery | Existing restoration policy |
| Missing origin, stale generation, closed stream, or failed initialization | No generic retention attestation | Existing conservative policy |
| Office, dynamic, automation, utility, passthrough, non-ACP | Existing owner | Existing policy |
| Older remote agentctl without generic attestation | Existing conservative behavior | Existing policy |
| Newer agentctl with older backend | Older lifecycle provider allowlist can reject unlisted providers; existing terminal/recovery path applies | No new host recovery authority |

The mock `/transport-lost` command currently returns a remote error containing a disconnect-shaped Data value while leaving ACP open.
That command cannot prove actual transport death under this contract.
Keep application-error and actual pipe/process-loss fixtures distinct, and migrate tests that intend real transport loss to a causal close or crash.
Do not preserve the old expectation by treating a diagnostic string as process authority.

## Notifications and unread state

`internal/backendapp/gateway.go` subscribes to `TurnCompleted` and invokes `HandleTaskTurnFinished`.
The current guard handles abandoned turns but does not explicitly reject error-terminated turns.
Carry and inspect authoritative `TurnMetaKeyErrorTerminated` evidence before sending successful turn-finished notifications.
Do not derive that decision from `WAITING_FOR_INPUT`, empty session error text, or ordinary assistant prose.
Apply the guard to both retained and genuine runtime-failure turns that complete with this metadata.

`AgentTurnFailed` and its idle settlement do not synthesize clarification or needs-answer notifications.
A real existing clarification request retains its own semantics and pending indicator.
The notification service's supported events contain no dedicated agent-failure push type.
Keep that behavior: this package adds no desktop/system/push failure subscription or fabricated completion notification.
Inline history and durable task attention provide failure feedback.

Preserve existing message-based unread accounting for the persisted error row through the normal message path.
Unread/read cursors and task focus remain independent of explicit error acknowledgement.
Do not synthesize a success occurrence or duplicate unread update when attention changes.
Add `TestErrorTerminatedTurnDoesNotNotify` at the gateway notification wiring and a retained-turn integration case.
Verify genuine successful turns and explicit clarification notifications still dispatch under existing subscriptions.
Browser tests observe unread/read behavior independently from the task error badge.

## Verification and operations

SDK tests prove positive response origin and negative local-error provenance with real pipes and request correlation.
Adapter tests cover generic internal, storage, auth, quota, model, and unknown errors across known and previously unknown providers.
They include empty and sanitized messages, wrapped errors, malformed success results, real pipe failure, expiry, and cancellation races.
Every positive case sends another distinct prompt on the same connection.

Lifecycle and orchestrator tests prove current ownership, durable error uniqueness, successor fencing, runtime-loss precedence, and unchanged autonomous policies.
Existing capacity and provider-recovery behavior remains in the targeted selection.
Retarget legacy rejection tests to the new boundary instead of preserving their provider/error restrictions.
`TestRetainableACPApplicationErrorRequiresClassifiedProviderCapacity` gains remote-error positives and local/no-origin negatives.
`TestTransientTurnFailureRequiresCurrentEligibleRuntime` replaces its unproven-provider negative with missing attestation or actual transport loss.
Update `TestRetainableACPAgentCoversOpenCode` to typed-attestation coverage when removing the obsolete name predicate.
Origin-positive generic/auth/quota errors and previously unlisted ACP providers must be retained.
A native OpenCode test uses isolated HOME/XDG directories and a local controlled provider.
It produces a real temporary SQLite writer lock, releases it, and proves follow-up acceptance on the same process and native session.
If that provider exits or closes ACP, record the actual loss and verify recovery handling instead of claiming native continuity.
Do not weaken the generic protocol contract or conceal a native compatibility failure.

Desktop and phone browser tests use real error envelopes through the mock ACP subprocess under production provider identities.
They prove one inline error, an enabled composer, a new follow-up response, no automatic replay, and stable runtime trace identity.
Include reload, an additional viewer, repeated failures, and a later actual crash.
No schema migration, production setting, feature flag, retry loop, or metrics family is added.
Persisted metadata and the additive compact summary field carry the new attention state.
Existing bounded lifetime diagnostics distinguish retained-turn settlement from actual process exit.

Acknowledgement tests cover client-observed same-session stamps, focus, explicit dismissal, accepted input, rejected/duplicate input, and stale-stamp races.
Include queued-before-failure automatic drain, queue edits, fresh Send now/steer actions, Quick Chat, and phone composer input.
Include two sessions: a late acknowledgement of one error must preserve a newer sibling failure.
An active sibling and genuine recovery error must remain independent of turn attention.
Phone proof uses the visible task menu and inline Dismiss with touch input and measured targets.
