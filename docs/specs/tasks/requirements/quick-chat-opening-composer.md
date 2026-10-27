---
status: active
system: tasks
created: 2026-10-02
updated: 2026-10-26
owners:
  - kandev
---

# Quick Chat opening composer requirements

## Overview

Users start a Quick Chat by composing its first message. The same surface supports
ordinary chats and configuration sessions. The task system owns this capability
because it controls session creation, opening-message delivery, and recovery.

The user accepted the desktop and phone ASCII proposal on 2026-10-02.
This document defines the Quick Chat opening-composer behavior. Existing
repository, attachment, and session contracts remain dependencies.

## Requirements

### REQ-TASKS-QUICK-CHAT-COMPOSER-001: Compose and start a conversation

**Intent:** Let a user write the first prompt before creating a session.

#### Acceptance criteria

- **AC-TASKS-QUICK-CHAT-COMPOSER-001.1:** A new chat shall immediately show an editable multiline prompt, an agent selector, attachment controls, and Send. Desktop shall focus the prompt. Opening the setup alone shall not create a server session or launch an agent.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.2:** Desktop shall center the composer beneath a short heading. One selector displaying the agent and profile label shall follow it. An icon-only repository action shall precede Attach in the composer toolbar. Send shall share the toolbar controls' vertical center. The separate Cancel/Start chat footer shall be absent.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.3:** The selector shall use the eligible workspace default for the selected chat kind. An explicit eligible selection shall survive a mode change. Missing, disabled, or unavailable profiles shall block Send without blocking prompt edits. No arbitrary profile shall silently replace the user's selection.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.4:** An ordinary chat shall accept zero or multiple repository/branch chips through Add repository. The action shall open a picker directly without creating an empty chip. Selected repositories shall appear as removable chips inside the composer. Duplicate repositories and incomplete selections shall block invalid submission. Repository isolation and branch defaults shall follow the existing repository-context contract.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.5:** When configuration chat is available, the setup shall expose an off-by-default Configuration session toggle. Its icon shall follow Attach. Hover and keyboard focus shall explain its settings-management capability; phones shall expose that description and a switch in a settings sheet. Enabling it shall show a mode indicator, hide repository chips, and disable Add repository. Toggling shall preserve text, attachments, and explicit agent choice. Returning to ordinary chat shall restore repository selections. A configuration launch shall exclude these repositories and use the existing configuration permissions and tools.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.6:** Attachment upload, retry, removal, limits, and delivery choices shall match task creation. Send shall remain disabled while an upload is incomplete or failed. Failed upload recovery shall retain the text and other attachments.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.7:** Installed voice controls shall insert text into the current draft through the existing composer integration. A plugin submit action shall use the same validation and submission path as Send. Without a compatible installed plugin, the composer shall remain usable without a nonfunctional microphone control.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.8:** Send shall require non-whitespace text, an eligible agent, ready attachments, and valid applicable repository selections. Attachments do not allow an empty or whitespace-only prompt. One activation shall create the conversation and submit its first message, including attachments. Repeated activation during submission shall not create a second conversation or send the message twice. The shared composer keyboard behavior shall apply, including multiline input and composition-event protection.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.9:** A failure before session allocation shall show an inline error and retain the setup draft for explicit retry. A launch failure after allocation shall open the retained session and preserve the payload for explicit recovery. A first-message delivery failure after creation shall retain the full payload in that session for explicit retry. Remount, reconnect, or profile hydration shall not automatically replay an attempted message or overwrite a newer draft.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.10:** Switching tabs or viewport size shall preserve each open setup draft. Explicitly closing its setup tab shall discard that draft and release unsubmitted attachments. A successful send shall clear only the accepted payload. Ready attachment descriptors shall retain the same-browser reload behavior of the attachment contract.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.11:** Phone users shall complete the same flow in a full-height surface. Agent and repository choices shall use touch-accessible pickers. The primary action shall remain reachable with a reduced viewport and bottom safe area. Controls shall have at least 44px touch targets, and the document shall have no horizontal overflow.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.12:** Labels, validation, and errors shall be localized. Controls shall have accessible names, keyboard navigation, and focus return from pickers. An active configuration conversation shall retain a visible mode indicator. Existing chats, configuration entry points, and terminal-backed profiles shall continue to work.

#### Startup acceptance amendment (2026-10-26)

The following criteria are active. Their implementation is recorded in the
[startup acceptance package](../../../plans/quick-chat-startup-acceptance/plan.md).
The existing criteria remain active.

- **AC-TASKS-QUICK-CHAT-COMPOSER-001.13:** An ordinary Quick Chat shall return its allocated conversation identity before repository preparation, setup scripts, or agent startup finishes. Slow startup shall show progress in that conversation. A later startup failure shall retain the conversation and its opening payload for explicit recovery.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.14:** An ordinary Quick Chat submission shall keep one creation identity through a lost response, explicit retry, reconnect, and same-browser reload. A same-token POST retry may resume missing preparation and settlement for the exact retained Quick Chat task under per-creation serialization. It shall reuse an existing session and dispatch its deferred start only when that session remains `CREATED`; it shall never allocate another task or session or replay an opening message. A concurrent `FoundUnsettled` create outcome shall remain an incomplete descriptor without post-create work. An incomplete creation shall remain observable without an automatic replacement. Explicit deletion shall retire the browser's submission identity.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.15:** An uncertain creation result shall preserve the submitted text, ready attachment descriptors, and creation identity. Recovery shall select only the matching authorized conversation. It shall not select an unrelated chat by title, profile, repository, or recency. A late result shall preserve newer drafts, selection, explicit deletion, and dismissal. Reload or status lookup shall not replay a message whose delivery was attempted.
- **AC-TASKS-QUICK-CHAT-COMPOSER-001.16:** A typed response that proves validation or a referenced repository failed before allocation shall retire the rejected submission identity and keep its editable draft. Untyped HTTP errors, transport failures, and a status lookup miss shall remain uncertain and shall not authorize a new identity.

## Related contracts

- [Repository context](quick-chat-repository-context.md): branches, isolation, and rollback.
- [Prompt attachments](prompt-attachments.md): upload ownership, limits, and draft descriptors.
- [Saved prompt delivery](saved-prompt-delivery.md): expansion for supported structured providers.
- [Quick Chat expiration](quick-chat-expiration.md): conversation lifetime and cleanup.
- [Conversation viewport](../../ui/requirements/quick-chat-viewport-layout.md): transcript layout after creation.

## Out of scope

New voice services, live voice conversations, remote repository entry, executor
selection, workflow controls, configuration-session repository support, new MCP
modes, and cross-device draft synchronization are excluded. This change does not
alter existing session selection, tab ordering, or repository isolation.

## Implementation plan

[Quick Chat opening composer](../../../plans/quick-chat-opening-composer/plan.md).


Follow-up implementation: [draft and rate retry package](../../../plans/quick-chat-draft-and-rate-retry/plan.md).
Criteria .9 and .10 already cover lossless accepted-payload cleanup; no new
requirement is introduced for this regression.

[Quick Chat startup acceptance](../../../plans/quick-chat-startup-acceptance/plan.md)
records the implemented startup and uncertain-response repair.
