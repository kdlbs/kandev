---
status: current
system: tasks
created: 2026-10-02
updated: 2026-10-26
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
owners:
  - kandev
---

# Quick Chat opening composer system design

## Purpose and boundaries

The task system owns the transition from a setup draft to the first conversation
turn. UI owns the surrounding dialog and tab navigation. This design reuses
existing attachment, launch, and MCP boundaries. The startup acceptance amendment
reuses task external identities and session metadata. It needs no database migration.

Before this change, source inspection found two separate setup components and a
string-only `initialPrompt` handoff. Ordinary Quick Chat eagerly started a
session; configuration chat prepared one and used a separate prompt-bearing path
for passthrough profiles. The redesign covers both paths without duplicate
dispatch.

## Requirement mapping

| Requirement                       | Sections                                                                                                   |
| --------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| REQ-TASKS-QUICK-CHAT-COMPOSER-001 | Composer and draft ownership; Launch and payload delivery; Startup acceptance amendment; Recovery; Responsive composition; Compatibility |

## Composer and draft ownership

`quick-chat-modal.tsx` currently selects `QuickChatSetup` or `ConfigChatSetup`.
Use one opening composer for the modal's ordinary/configuration setup branches.
Keep the floating configuration panel as an existing caller; share launch payload
logic without changing that panel's composition.

Reuse `TaskFormInputs` attachment and prompt primitives from
`task-create-dialog-selectors.tsx`. Extract a small shared input boundary where
necessary; do not copy its upload pipeline or mount the complete task-create form.
Keep saved-prompt support through the existing input/dispatch contracts.
Do not add workflow, executor, task title, or external issue-import controls.

For voice, use `ChatInputPluginActions` with `surface: "quick-chat"`, null task
and session IDs before creation, and the existing composer capability. Its
`chat-input-actions` slot already supports a task-less composer. Do not expose
this input as task creation. Use synchronous draft refs so insert-and-submit
within one plugin callback submits the inserted text. Plugin availability owns
whether voice controls appear; no new host or plugin API is required.

Keep draft ownership above the mode-specific presentation and outside components
that unmount when another tab becomes active. Scope it by authenticated identity,
workspace, and setup identity. A mode switch must not close and recreate the
setup as `handleSetupKindChange` currently does. Preserve the editor instance or
restore its full text, selection, and attachment state without releasing uploads.

Draft fields are text/editor content, staged attachment descriptors, repository
rows, kind, explicit profile choice, and submission state. Default the profile
from the workspace's configuration default (then ordinary default) in config
mode, and ordinary default in chat mode. An explicit eligible selection wins.
Revalidate eligibility at submission. Do not substitute the first available agent.

Use the existing local draft text/content/attachment storage helpers for reload
recovery, behind a setup key scoped by identity and workspace. Store descriptors,
never File objects or bytes. Preserve per-tab in-memory state during navigation.
Closing a setup clears its storage and releases staged uploads best-effort.
Closing the modal follows the current setup-close behavior. Existing conversation
close and deletion semantics remain unchanged.

## Launch and payload delivery

Extend the frontend opening-message handoff to carry `ChatSubmitPayload`, including
attachment descriptors and a stable `clientMessageId`. Keep legacy string-only
`initialPrompt` callers compatible during migration. Update the UI slice types,
actions, reconciliation, `QuickChatSessionView`, `QuickChatContent`, and
`useQuickChatInitialPrompt` together. Clear an attempted automatic handoff before
waiting for acceptance, while retaining its recovery payload.

Structured chats use the existing HTTP creation endpoint without a prompt-bearing
launch. Register the returned session, attach the subscribed shell, and use the
existing message handler once its admission prerequisites are ready. This preserves
saved-prompt expansion, attachment claims, and observation of fast first turns.
Configuration chats retain their configuration endpoint and `config_mode` metadata.
Do not send the same payload through HTTP and the subscribed message handler.

Terminal-backed profiles have no structured composer after launch. Send their
opening payload through the backend launch path. Extend both quick-chat and
config-chat request types with optional `attachments` using `v1.MessageAttachment`.
The fields are additive; legacy empty-prompt API callers retain their behavior.
Reuse `validateAttachments`, attachment ownership/claim services, and
`LaunchSessionRequest.Attachments`. Reject attachments without a prompt in this
launch path instead of silently ignoring them.

The shipped ordinary path forwards Prompt and Attachments to one synchronous
`IntentStart`. The startup acceptance amendment replaces that ordinary path.
Configuration keeps `IntentPrepare` with `DeferredStart` and forwards the payload
once through `IntentStartCreated`. Validate and claim
uploads before agent dispatch, using the established task-create pattern. On a
preparation failure, use the existing lifecycle rollback. Retain canonical upload
references or restore staging ownership for a failed request; do not present a
ready descriptor whose file rollback deleted. Test that boundary explicitly.

A backend-accepted terminal launch keeps its task/session identity if subsequent
startup fails. Use its existing explicit start/retry path and retained payload;
do not create another chat or automatically replay on reconnect. Leave terminal
runtime policy and provider-native attachment limits to the established adapter.

## Startup acceptance amendment

This section is implemented. Its delivery record is the
[startup acceptance package](../../../plans/quick-chat-startup-acceptance/plan.md).
It satisfies `AC-TASKS-QUICK-CHAT-COMPOSER-001.13` through `.15` and preserves
the existing first-message, draft, and failure contracts.

### Allocation before startup

`TaskHandlers.httpStartQuickChat` shall separate session allocation from runtime
preparation. Reuse the shipped task-create pattern in `prepareStartAgentSession`
and `dispatchTaskSession`. Do not route ephemeral chats through the task form.

1. Authorize the workspace and resolve a keyed retry before new allocation.
2. Validate a new submission and create its ephemeral task through `Service.CreateTask`.
3. Allocate its session through `LaunchSession(IntentPrepare)`. Set
   `DeferredStart: true` and leave `LaunchWorkspace` false.
4. Persist required opening-payload ownership before accepting a request with an
   HTTP opening prompt. Use `models.NewInitialPromptSubmission`,
   `InitialPromptSubmission`, and the existing attachment claim services.
5. For a keyed create, call the shipped `Service.SettleExternalID` after required
   synchronous work. Only a newly created, successfully settled task can dispatch.
6. Return HTTP 200 with the existing task/session fields. Dispatch
   `IntentStartCreated` once, outside the request's cancellation lifetime.

A keyed POST retry that resolves an exact unsettled Quick Chat before calling
`CreateTask` holds the per-creation handler lock while it resumes missing
preparation and settlement. It reconstructs launch parameters from the retained
task, reuses its primary session when present, and dispatches only when that
session is still `CREATED`. If `CreateTask` itself returns `FoundUnsettled`
after an earlier lookup missed, return the incomplete descriptor without
preparing or dispatching; that outcome can still belong to a concurrent owner.
The status GET remains read-only. This endpoint-specific retry does not change
the generic external-ID create contract.

The response describes an accepted conversation, not a ready agent. It must be
available while repository preparation remains blocked. Keep HTTP 200 for legacy
callers and keep the global server write timeout unchanged.

Background work uses a bounded `context.WithoutCancel` child that retains the
authenticated principal. Use the configured `constants.AgentLaunchTimeout`,
which includes the preparation budget. Register the launch before releasing
request ownership. Follow the existing runtime shutdown and deletion guards.
Do not capture `gin.Context` in the background closure.

Browser structured chats omit the HTTP opening prompt and use
`NoInitialPrompt: true` for startup. That setting prevents task-description
fallback and an empty initial turn. The subscribed message path remains their
sole opening-message sender.
Terminal-backed profiles use one prompt-bearing `IntentStartCreated` with the
accepted payload. The browser must not also submit that payload through chat.
File-backed descriptors and text survive in the existing initial-submission
receipt. Legacy inline attachment bytes remain subject to its explicit
replay-blocked policy after an interrupted dispatch.

A legacy API caller can supply an HTTP prompt for a structured profile.
Preserve that contract through the same backend-owned receipt and prompt-bearing
start. Do not discard its prompt based on profile kind. Each request has one
opening-message owner, determined by whether it supplied an HTTP prompt.

Record asynchronous preparation and admission errors through the orchestrator's
typed failure transition. This includes errors before an execution exists.
Extend the prepared Quick Chat boundary in `quick_chat_launch_failure.go` where
necessary. Preserve task/session ownership, stale-attempt fences, sanitization,
and intentional capacity deferral. A handler log alone is not a failure outcome.
Retain an allocated conversation. Only a failure before session allocation can
use the existing safe task rollback.

A backend interruption between allocation and dispatch leaves a retained session
for explicit recovery. Opening or reconciling it must not manufacture a second
session or replay a terminal opening payload. This amendment adds no durable job
queue or autonomous restart worker.

### Stable creation identity

Add optional `creation_id` to the ordinary Quick Chat POST request. It is a
canonical UUID for one immutable submission, not an agent or message identifier.
The browser generates and saves it before the first network request.
Legacy callers can omit it and receive the same response fields.

For keyed requests, derive the task external identity on the server:

```text
quick-chat-create:v1:<sha256(authenticated-principal-id)>:<creation_id>
```

The existing `(workspace_id, external_id)` unique index supplies concurrency
protection. Reserve `quick-chat-create:` for this adapter. Extend
`refuseReservedExternalIDPrefix` to refuse public creation and release of that
namespace. The trusted adapter alone sets `AllowReservedExternalID` after
constructing the value. Do not accept a raw external identity or principal from
the browser. Verify that an existing holder has the expected ordinary-chat kind,
workspace, creator, and exact token before returning it. A conflicting legacy
holder is an explicit conflict, never an adoption candidate.

Identity retention follows the existing task lifetime. This package does not
retain server tombstones after physical task deletion. The browser retires the
submission token on explicit deletion and cancels queued creation retries.
It must not submit that token again. Existing local deletion tombstones still
refuse late response/event activation.

Use `Service.GetTaskByExternalID`, `CreateTaskResult.Outcome`, and
`Service.SettleExternalID`. These APIs ship today. Only the reserved Quick Chat
POST adapter may resume the exact unsettled Quick Chat it resolves before
`CreateTask`; generic external-ID callers and status GET remain read-only. The
aggregate creation coordinator in [Task creation protocol](task-creation-protocol.md)
is still a draft. This repair does not require or implement that separate migration.

Resolve a found identity before validating mutable creation fields or allocating
anything. A settled or archived result returns immediately. A same-token POST
that finds an unsettled Quick Chat may resume that task under the per-creation
handler lock. It never allocates another task or session. It dispatches only a
retained `CREATED` session after settlement. The `FoundUnsettled` result from
`CreateTask` remains read-only because it can still belong to another request.
Return the original task's persisted opening data even when the retry carries
changed creation fields. The browser resends only its captured immutable
snapshot. Status GET never repairs, prepares, settles, or dispatches.

Successful keyed responses add `creation_id` and `creation_complete: true`.
`creation_complete` means allocation and required acceptance work completed.
It does not mean startup completed. A `FoundUnsettled` create outcome returns
`creation_complete: false` and its task ID without post-create work. A keyed
POST retry that resolves the same Quick Chat before `CreateTask` may resume it
as defined above. Include a session ID only when the authoritative task/session
binding exists. Do not invent a session or choose the workspace's newest chat.
An archived or deleted result is never started again.

Add a read-only route:

```text
GET /api/v1/workspaces/:id/quick-chat/creations/:creation_id
```

Its authorization and identity derivation match POST. An authorized match returns
the same acceptance descriptor, with its persisted session state when available.
Absence or an inaccessible record returns 404. Invalid UUID syntax returns 400.
A lookup failure remains unknown. Lookup does not allocate, repair, start, or
send anything. A 404 permits an explicit same-token POST. It never proves that
an earlier request cannot still commit, so a new token is forbidden for that retry.

Return the exact token in authorized Quick Chat list and lifecycle projections
when present. Extend `QuickChatSession` additively. Older rows have no token.
They remain readable, but an uncertain request cannot adopt them by inference.
Token values belong in bounded diagnostics, never metric labels. Do not log the
opening text, attachment data, or authentication values.

### Browser reconciliation and payload ownership

Keep the pending submission above setup component lifetime. Scope it by auth
identity, workspace, setup generation, and creation ID. Persist its immutable
text, descriptor-only attachments, profile/repositories, stable client message ID,
and whether message admission was attempted. Store no file bytes or credentials.
Use the existing setup draft storage and opening-payload hooks.

HTTP acceptance, exact-token lookup, and exact-token lifecycle events merge one
task/session identity. Until `creation_complete` is true, keep the submission
pending and do not consume its opening payload. An event with a session ID alone
does not prove that a terminal payload was accepted.

The current request replaces its setup with the one accepted conversation.
Move the captured structured payload to its existing initial-prompt handoff.
Wait for normal subscription and startup admission, then submit it once.
Clear only the captured draft after ownership transfers. Preserve newer edits.
The accepted terminal receipt replaces browser delivery ownership.

An accepted startup owns its initial `CREATED` or `STARTING` session.
Suppress competing passive resumption and first-message launch while that startup
is pending. Use canonical session state and existing lifecycle arbitration.
Boot readiness permits normal message admission. Failure or an interrupted
startup exposes explicit recovery of the same session. Include a race between
passive status/resumption and the original background launch in regression tests.

For a network exception, proxy 502/503/504, malformed success body, or uncertain
server error, resolve the same creation ID before offering another create.
Show a localized creation-status message and a read-only **Check status** action.
An explicit retry keeps the same token. Do not label an unknown outcome as a
confirmed startup failure. The POST handler marks typed pre-allocation
validation and repository-reference rejections with
`error_code: quick_chat_creation_rejected`. The browser retires that receipt
and retains the editable setup only for this code. Other 4xx/5xx responses,
including an untyped 404, remain uncertain. A status lookup 404 never proves
that the original POST cannot still commit.

If lookup finds a task without a prepared session, retain the draft and show
incomplete creation. Never advertise a nonexistent conversation as restorable.
Explicit discard uses the existing authorized task-deletion confirmation for
that task ID. It does not release the external identity or silently create a
replacement. After confirmed deletion, retire the token and revalidate files
before a later new submission.

Reload can restore a draft and perform status reads. It cannot automatically
POST creation or replay an attempted opening message. Explicit recovery uses
the existing message/session admission path with the captured message identity.
Failures after acceptance stay in the retained conversation's recovery surface.

Request generations, auth/workspace changes, dismissal, and deletion tombstones
still control activation. A late result can reconcile its owned tab without
selecting it, reopening the modal, clearing another draft, or reviving deletion.
Include mixed healthy and pending/failed tabs in regression coverage.

### Desktop and phone status presentation

Reuse the existing dialog on desktop and full-height Quick Chat on phones.
Known identities show the existing session preparation rows and recovery card.
Unknown identities keep the submitted draft visible in setup with one status
region and **Check status**. An incomplete task additionally exposes the existing
confirmed deletion flow. Do not add an overlay for status reads.

The phone exemplar is the shipped Quick Chat surface in `quick-chat-modal.tsx`.
Keep its fixed navigation, one internal scroll body, dynamic height, and safe-area
handling. Status actions are visible buttons with at least 44px touch targets.
The primary phone action checks or resumes this same creation. Desktop controls
retain their compact sizing. New copy uses all seven locale catalogs.

### Regression evidence

A handler integration test holds preparation on a channel behind a real HTTP
server and reverse proxy. The accepted identity must arrive before that channel
is released, even with a short response write deadline. This reproduces the
original failure without a 36-second test delay.

Backend tests cover keyed concurrency, found-before-validation ordering,
settlement failure, actor/workspace/kind mismatches, lookup authorization,
pre-session rollback, asynchronous failure, deletion, and terminal receipts.
Hook tests cover WS-first/HTTP-first order, non-JSON 502 after allocation,
malformed 200, lookup errors, same-token retry, reload, message deduplication,
navigation, and newer-draft preservation.

Desktop and phone Playwright tests drop the response after the real request
allocates its session. They prove one conversation, preserved attachments, one
opening message after confirmed acceptance, and explicit recovery after reload.
Backend real-server evidence owns the response-deadline claim. Browser response
interception alone cannot prove that the backend returned before preparation.

## Recovery

Snapshot the draft at Send and lock duplicate submits synchronously. Keep an
inline creation error in setup. Once a session exists, move recovery ownership
there; do not offer another create operation for a failed message.

Persist both text and ready attachment descriptors before automatic structured
submission. Reuse `getChatDraftAttachments` and `setChatDraftAttachments` with the
existing descriptor conversion. Rejection, synchronous throw, and asynchronous
failure preserve the payload. Acceptance clears only the matching snapshot.
A newer manual draft must survive settlement of an older submission.

Do not retry automatically after an uncertain acknowledgement. Retain the stable
message identity for the existing admission path and expose explicit recovery.
Preserve request-generation guards for superseded starts, including workspace
changes and setup closure. Reconcile a late completed session into the workspace
tabs without activating or deleting it. A late response must not steal active
focus.

## Responsive composition

Desktop retains the resizable Quick Chat dialog and tab strip. Center a bounded
composer in the remaining slot. Repository chips and attachments share one
wrapping context row above the input. The configuration label uses that same row
in configuration mode. Center its contents vertically with balanced 8px padding
above and below; keep 12px horizontal padding. Phone chips wrap naturally within
the setup scroll body without a separate context scrollbar.
Add repository precedes Attach and opens a picker without allocating a blank row.
The configuration icon follows Attach, with explanatory hover/focus help and a
visible mode indicator when enabled. One agent/profile selector follows the composer.
The Send control lives inside the input action row. There is no setup footer.

Phone uses the existing full-height Quick Chat surface. Reuse the interaction
pattern of `task/mobile/mobile-picker-sheet.tsx`: visible labeled triggers and
inset bottom pickers with internal scrolling and focus return. Keep a single
setup scroll body below fixed navigation. Collapse decorative vertical space
when the keyboard reduces the viewport; scroll the focused editor/action into
view. Use dynamic viewport height and bottom safe-area padding. Do not center
against a fixed screen height. Repository selection uses an inset bottom picker; configuration help and its
switch use a bottom sheet. The agent selector occupies the row below the composer.

Use `useResponsiveBreakpoint`, shared UI tokens, 28px ordinary desktop controls,
and at least 44px phone/coarse-pointer targets. State survives breakpoint changes.
Test canonical phone width and narrow fine-pointer widths around 768px. Existing
transcript layout remains under the conversation viewport contract.

## Compatibility and security

| Path                                                     | Delivery                                                              | Evidence and fallback                                                                                                  |
| -------------------------------------------------------- | --------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Structured chat, including supported ACP/native adapters | Existing subscribed message handler, one full payload                 | Hook tests plus mock-agent E2E; block while admission is unavailable                                                   |
| Structured configuration chat                            | Configuration endpoint then subscribed handler                        | Config hook tests and E2E; retain configuration eligibility and tools                                                  |
| CLI passthrough chat/configuration                       | Backend prompt-bearing launch only                                    | Handler tests for prompt, attachments, deferred start, and one dispatch; preserve adapter rejection as a visible error |
| Installed voice plugin                                   | Existing quick-chat composer capability                               | Fixture insert/submit test; absent plugin exposes no fake microphone                                                   |
| Dynamic or unavailable profile                           | Existing selectable-profile policy and authoritative returned profile | Preserve request/response identity; unavailable selection blocks submission                                            |

The current modal allows configuration creation only when no configuration tab
exists in that workspace. Preserve this gate and Settings launch behavior.
Do not treat hiding the toggle as authorization. Existing backend workspace,
profile, attachment, and configuration permission checks remain authoritative.

No new MCP mode, security boundary, feature flag, or telemetry is needed. Use
existing request/session diagnostics without logging prompts or attachment data.

## Related contracts and decisions

- [Repository isolation ADR](../../../decisions/0038-quick-chat-repository-isolation.md).
- [Attachment ADR](../../../decisions/2026-08-04-file-backed-prompt-attachments.md).
- [Attachment design](prompt-attachments.md).
- [Saved prompt delivery](saved-prompt-delivery.md).
- [Session resumption](quick-chat-session-resumption.md).
- [Tab selection](../../ui/system-design/quick-chat-selection.md).

The repository-context requirement has been reconciled with the opening composer.
Its repository-isolation and recovery guarantees remain authoritative. The startup
amendment records its acceptance and retry decision in the
[creation acceptance ADR](../../../decisions/2026-10-26-quick-chat-creation-acceptance.md).

## Implementation plan

[Quick Chat opening composer](../../../plans/quick-chat-opening-composer/plan.md).


## Lossless opening draft restoration

Plain-text restoration must preserve every newline, including empty paragraphs.
`use-tiptap-editor.ts` must restore text with the existing structured
`textToEditorContent` conversion. An empty line is an empty paragraph, not a
paragraph containing a hard break. Keep stored rich-document restoration first.
Preserve literal text, code, mentions, and saved-prompt syntax during restoration.

Acceptance cleanup retains exact snapshot ownership. Do not collapse whitespace
or clear the editor unconditionally to compensate for conversion differences.
Clear matching text, rich content, and attachments together. Newer edits and
another session's draft must survive. Provider failure after message acceptance
does not restore that accepted message as unsent input.

This implements existing criteria `AC-TASKS-QUICK-CHAT-COMPOSER-001.9` and `.10`.
Delivery: [draft and rate retry package](../../../plans/quick-chat-draft-and-rate-retry/plan.md).

[Quick Chat startup acceptance](../../../plans/quick-chat-startup-acceptance/plan.md)
covers the implemented startup acceptance amendment.
