---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-EXPLICIT-STEERING-001
  - REQ-PLATFORM-EXPLICIT-STEERING-002
  - REQ-PLATFORM-EXPLICIT-STEERING-003
---

# Explicit Same-Turn Steering System Design

## Boundary and baseline

Platform owns foreground input delivery. Follow the
[explicit-intent decision](../../../decisions/2026-09-27-explicit-same-turn-steering.md).
This design adds an opt-in same-turn path alongside existing
[ACP/opportunistic steering](../requirements/mid-turn-steering.md), without
changing automatic sends, queued prompts, or the background-work admission tracker.
[Agent background work](../../agents/system-design/background-work.md) supplies
inspection surfaces, not a second authority for prompt admission.

PR #3916 baseline `b3ead0d0376317215ddcadc48b316b3743782744` contains the pinned
Codex 0.154.0 TurnSteerParams schema, but no native PromptSteer/SupportsSteering
implementation or typed turn/steer operation. Existing optional
`adapter.SteerablePrompter`, `SteerTask`, runtime/lifecycle steering forwarding,
and the composer's supports_steering boolean are reusable integration points,
not proof of native same-turn support.

| Requirement | Sections |
| --- | --- |
| REQ-PLATFORM-EXPLICIT-STEERING-001 | Capabilities and gates; Explicit admission; Codex mapping |
| REQ-PLATFORM-EXPLICIT-STEERING-002 | Targeting and receipts; Attribution and failure behavior |
| REQ-PLATFORM-EXPLICIT-STEERING-003 | Composer; Compatibility and verification |

## Capabilities and gates

Extend normalized agent/session capability projection additively with `steering`:
`mode` is `same_turn`, `provider_managed`, or `unsupported`; availability is
`available`, `pending`, or `unavailable` with a closed reason code; an available
same-turn projection includes an opaque `turn_ref`. This reference identifies
one root Kandev turn and its session incarnation, execution and prompt generation;
the provider thread/turn binding remains backend-owned. It is a correlation
precondition, not authorization. It must change when any ownership boundary
changes and cannot resolve to the latest turn by fallback.

The optional adapter capability reports semantics explicitly. Codex advertises
same_turn only after the pinned native implementation is connected and the
current ordinary turn can accept steering. Review/manual compaction and unknown
turn kinds default unavailable. ACP's current negotiated promptQueueing maps to
provider_managed, never same_turn. Providers without the extension are unsupported.
A future Claude-native adapter may advertise only semantics its tests establish.
Do not implement a provider-name whitelist or infer support from a background job.

Add `features.sameTurnSteering` / `KANDEV_FEATURES_SAME_TURN_STEERING`,
restart-required, default false in prod/dev/e2e. Use current runtimeflags registry
read/apply wiring, config tags, profile/embedded-profile contract and frontend
defaults. Native Codex requires this flag AND features.codexAppServer, but does
not require features.agentBackgroundWork. Existing features.claudeMidTurnSteering
and Claude background prompt handoff retain their meanings and defaults.

Backend gates cover capability serialization, explicit admission, receipt writes,
runtime dispatch and any direct agentctl entry. Disabled requests cannot reach
the provider or persist a pending send. Existing normalized messages, queueing,
ACP steering and cancellation continue. No new MCP or synthetic-wakeup entry.

For old clients preserve `supports_steering`'s legacy ACP contract: do not set it
true solely because native same_turn is available. New clients use `steering`
for explicit actions; missing metadata follows the existing boolean/queue path.
This prevents an older client from accidentally invoking new bypass semantics.
On reconnect, public availability stays false until live ownership is confirmed;
a stored capability is never proof that a root turn remains steerable.

## Explicit admission

Add an explicit delivery intent to the existing main-chat submission boundary:
`delivery_mode: active_turn`, `turn_ref`, and `client_delivery_id` for Send now.
Legacy omission remains automatic. Queue for later uses the existing queue API
and its client_queue_id receipt semantics; do not change that API or repurpose
its receipts. Existing message.add/ordinary prompt callers, comments, Office
routines and wakeups do not infer an explicit intent from capability alone.

Trace `use-message-handler.ts` / `sendMessageRequest`, the message submission
handler, orchestrator, executor, runtime/lifecycle and instance-scoped agentctl.
The active_turn branch runs before the generic RUNNING/queued-count decision,
but after task/session authorization and draft/input validation. Reuse the
existing session-admission critical section to reserve a single explicit-send
slot for the original root generation. A nonempty Kandev queue does not block
this deliberate mode or receive a new entry. Existing queued messages retain
their IDs, contents, positions, and future dispatch eligibility.

If another same-turn send is awaiting acknowledgement, return steer_busy with
no dispatch/queue write. Keep typing enabled. Release the slot on ack, definite
rejection, bounded timeout or disconnected/uncertain outcome; subsequent sends
may use a refreshed live target without waiting for root completion. An uncertain
send remains visible as such even when a later distinct send is allowed.

Do not implement this as an unconditional relaxation of SteerTask's queue-first
rules. Share validation/transport where their semantics match, while keeping
explicit intent distinguishable from legacy opportunistic steering. The native
path reuses the active dispatched prompt generation and its completion owner.
It does not transfer ACP prompt gates, cancel the predecessor, allocate a new
foreground turn, or await turn/completed before acknowledging accepted input.

Hold only bounded admission/ownership locks; no provider RPC or attachment I/O
under them. While an explicit send owns its dispatch reservation, queue drain
must respect that reservation, including a root completion racing the send.
Release/recheck drain on every terminal dispatch outcome. No stale reservation
can strand queued work. Automatic ACP steering and explicit mode cannot both
own the same session dispatch slot.

## Targeting and receipts

Resolve and authorize the owning task/session before receipt lookup or target
binding access. At admission and at agentctl dispatch compare incarnation,
execution/connection generation, root turn_ref and native expected turn. No
background-work ID or child-thread ID is accepted by this path. The final native
expectedTurnId precondition closes the completion-to-successor race after locks
are released. A stale/ineligible result never becomes a prompt to another turn.

Introduce task-owned `task_session_steering_receipts`, keyed by
(session_id, incarnation_id, client_delivery_id), with payload fingerprint,
immutable target binding, one user-message ID, and pending/accepted/rejected/
uncertain result. Validate a bounded client ID and input before reserving.
The fingerprint covers text, attachment/content references and target, not only
visible text. The atomic reservation and pending user-message write commit once
before provider dispatch; concurrent duplicate IDs return the existing receipt.
A changed payload conflicts. Use task repository conventions, idempotent SQLite
and PostgreSQL migrations, and session-deletion cascade. No historical backfill.

Keep receipt and pending transcript admission in one transaction. Unsupported or
unauthorized requests create neither. Definite provider rejection updates that
one message's delivery state rather than creating a delivered user turn. An
accepted response updates the same message. On reconnect/crash, pending receipts
become uncertain unless correlated provider evidence proves acceptance; never
resend because a response or message echo is missing. The same ID is a lookup,
not permission to dispatch again. Keep receipts for the retained session lifetime.

Return typed results through websocket and runtime transports:
accepted (target turn_ref), rejected (unsupported, stale_turn, turn_not_steerable,
steer_busy, incompatible_input, unavailable, forbidden), or uncertain.
`accepted` means provider RPC acceptance only. Use a 10s native call deadline
inside a 15s outer operation bound; cancellation or response-correlation cleanup
does not prove the remote action stopped. Ignore late responses for successor
ownership, but allow exact receipt correlation to resolve the original uncertain
message. No automatic queue fallback or provider retry after ambiguous delivery.

## Codex mapping

Add MethodTurnSteer and typed TurnSteerParams/Response to the shared pinned client.
Send `threadId`, exact `expectedTurnId`, and converted `input`. Check that the
response turnId matches the original target. Do not call turn/start, interrupt,
or turn/steer with the adapter's newest turn ID after an error.

The pinned schema also permits clientUserMessageId. Use the durable message ID
for echo correlation after proving its notification/history behavior with fixtures;
its presence does not imply upstream idempotent execution. Kandev's receipt owns
at-most-once dispatch attempts. A mismatched/malformed response after a possible
write is uncertain, not a safe-to-retry rejection.

Share existing native input conversion where supported and validate attachment
shapes before dispatch. Inputs that cannot be represented are rejected; never
drop an attachment. turn/steer cannot change model, cwd, sandboxPolicy or
outputSchema. Changed turn-level settings/plan-mode options are retained for a
future queued/new turn and make this draft's Send now unavailable when they would
otherwise be silently ignored. Distinguish current inherited settings from a
new override; ordinary composer defaults must not disable every steer.

Protocol errors proving no accepted turn (no active turn, expected-turn mismatch,
activeTurnNotSteerable) are typed rejection. Transport loss/timeouts after possible
write are uncertain. Test raw error envelopes from the pinned schema instead of
matching arbitrary English error strings. Maintain current root connection and
turn events. No new turn/started is expected for an accepted steer.

[Official protocol reference](https://learn.chatgpt.com/docs/app-server#steer-an-active-turn)
corroborates these semantics. Pinned schema and fixtures remain authoritative;
a live test must demonstrate two user inputs and one root turn completion, not
just an RPC acknowledgement or successful initialization.

## Attribution and failure behavior

Associate the pending/accepted user message with the existing root Kandev turn.
Native user-message item notifications/history reconcile against its durable
message correlation rather than append duplicate rows. Do not invent identity
by matching text: users may deliberately send the same text twice with distinct
IDs. Where an echo cannot be correlated, preserve explicit unavailable evidence
and reconcile using tested provider item IDs; never claim provider-side dedup.

Root assistant/tool streams, child bindings, usage observations, clarification
requests and the completion waiter retain their original generation. One
turn/completed settles the root exactly once even if acceptance arrives after
completion. Accepted steering adds no new usage event by itself; continue existing
provider usage normalization. Cancelling the root may settle the target while a
send is pending; classify that send from actual response/evidence, never steal
or synthesize another completion. Existing full-stop semantics remain unchanged.

Store/socket reconciliation uses session/incarnation and message-delivery revision
so a delayed pending/uncertain response cannot overwrite a newer accepted event.
Backend restart reconstructs receipt uncertainty, not a live dispatch reservation
or running generation. Unknown ownership disables Send now until reconciled.
Log/metric outcomes use closed mode/outcome labels without prompt text,
attachments, credentials, native IDs, or per-session metric labels.

## Composer

For available same_turn, make the main submit action explicitly Send now and
provide Queue for later in a delivery-mode menu. This remains true with queued
messages: supporting text says that Send now updates the current turn and leaves
the queued messages for later. The background-work pill remains small and
independent. The selected action is part of the submission payload, not inferred
server-side from queue count. No background-agent detail composer is added.

```text
UI-04: desktop, main conversation running
 ( 2 Background Jobs )
+----------------------------------------------------+
| Keep the public API unchanged...                   |
|                                   [Send now] [v]   |
+----------------------------------------------------+
 Queue: 2 messages for later
                         +--------------------------+
                         | Send now                 |
                         | Queue for later          |
                         +--------------------------+

UI-05: phone, same capability
 ( 2 Background Jobs )
+-----------------------------------+
| Keep the public API unchanged...  |
|                    [Send now] [v] |
+-----------------------------------+
 [2 queued for later]
 Tap v: inset delivery-choice drawer
+-----------------------------------+
| Send now                          |
| Add to the current turn           |
| Queue for later                   |
| Run after current work            |
+-----------------------------------+
```

Desktop uses a split-button/menu in existing chat toolbar anatomy; phone uses a
visible 44px delivery-choice trigger and inset Drawer with safe-area/keyboard
clearance. Reuse existing responsive menu/picker primitives. Switching choice
preserves draft/attachments. Selection is scoped to this composer session and
not persisted as a global provider preference. On turn change, invalidate the
old submission target; require a new deliberate action for an unsent/rejected
active-turn intent rather than silently delivering it as an ordinary prompt.
Normal idle composing continues to use ordinary Send; old pending/uncertain
messages retain their original state and are not automatically submitted.

Pending: show Sending to active turn, disable repeated Send now, retain editing
and allow deliberate Queue for later as a distinct action. Accepted: clear only
the submitted draft revision and show Sent to active turn, leaving any later
edits intact. Definite rejection: retain the draft and explain the reason.
Uncertain: show Delivery unknown with the submitted message identified; no
automatic resend button. A deliberate new send or queue action after uncertainty
warns that the previous message may already have arrived. Do not display raw
provider errors or claim the model acted on accepted input.

Use normalized capability mode for rendering: provider_managed retains its current
opportunistic copy/queue-first behavior; unsupported retains ordinary queueing.
Task chat and Quick Chat share intent/draft logic. Passthrough terminals keep
terminal semantics. Permission/clarification controls remain separate; ordinary
steering does not satisfy an outstanding question. Add all six language catalogs
and pseudo, using existing Traditional Chinese generation. Examples are structural,
not final untranslated copy.

## Compatibility and verification

The package adds two work orders under
[Agent Background Work](../../../plans/agent-background-work/plan.md): backend
same-turn contract/dispatch and shared composer delivery selection. Existing
background visibility/count changes remain forbidden from modifying admission;
only explicit active_turn intent invokes this new path. Existing ACP results
and prior plan evidence remain historical, not native compatibility proof.

Deterministic tests cover queue bypass without queue mutation, repeat sends after
ack, in-flight busy rejection, before-dispatch and after-write completion races,
old events after successor, cancellation, backend restart, duplicate delivery IDs,
provider echoes, once-only usage/completion, unsupported input/options, permission
ownership, feature-off/old-client fallback and remote routing. Use channel/barrier
coordination instead of sleeps. Include real SQL receipt/message round trips on
SQLite and PostgreSQL, plus desktop/phone E2E through a native fake server and
negative ACP fixtures. Optional live evidence must be labelled separately.
