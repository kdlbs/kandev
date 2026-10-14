---
status: current
system: tasks
created: 2026-10-02
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
owners:
  - kandev
---

# Quick Chat opening composer system design

## Purpose and boundaries

The task system owns the transition from a setup draft to the first conversation
turn. UI owns the surrounding dialog and tab navigation. This design reuses
existing attachment, launch, and MCP boundaries. It needs no database migration.

Before this change, source inspection found two separate setup components and a
string-only `initialPrompt` handoff. Ordinary Quick Chat eagerly started a
session; configuration chat prepared one and used a separate prompt-bearing path
for passthrough profiles. The redesign covers both paths without duplicate
dispatch.

## Requirement mapping

| Requirement                       | Sections                                                                                                   |
| --------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| REQ-TASKS-QUICK-CHAT-COMPOSER-001 | Composer and draft ownership; Launch and payload delivery; Recovery; Responsive composition; Compatibility |

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

For an ordinary prompt-bearing launch, forward Prompt and Attachments to the
single `IntentStart`. For configuration, keep `IntentPrepare` with `DeferredStart`
and forward the payload once through `IntentStartCreated`. Validate and claim
uploads before agent dispatch, using the established task-create pattern. On a
preparation failure, use the existing lifecycle rollback. Retain canonical upload
references or restore staging ownership for a failed request; do not present a
ready descriptor whose file rollback deleted. Test that boundary explicitly.

A backend-accepted terminal launch keeps its task/session identity if subsequent
startup fails. Use its existing explicit start/retry path and retained payload;
do not create another chat or automatically replay on reconnect. Leave terminal
runtime policy and provider-native attachment limits to the established adapter.

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
Its repository-isolation and recovery guarantees remain authoritative. No new ADR
was needed because this design reuses those established boundaries.

## Implementation plan

[Quick Chat opening composer](../../../plans/quick-chat-opening-composer/plan.md).
