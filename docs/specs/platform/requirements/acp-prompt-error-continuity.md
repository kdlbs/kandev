---
status: active
system: platform
created: 2026-10-29
updated: 2026-11-05
owners:
  - Kandev
---

# ACP prompt error continuity requirements

## Overview

A failed ACP prompt can leave its runtime ready for another message.
Platform owns shared runtime continuity, error attention, and acknowledgement across task surfaces.
Tasks retains ownership of message admission and durable conversation records.

Requirements 004 and 005 moved from [transient turn runtime continuity](transient-turn-runtime-continuity.md) without changing their IDs.
The November 5 amendment adds criteria 005.9 through 005.12.
Its [delivery package](../../../plans/turn-error-continue-capacity/plan.md) records the implementation and validation results.

## Requirements

### REQ-PLATFORM-TURN-CONTINUITY-004: Continue chat after ACP prompt errors

**Intent:** A user can send a follow-up after a failed prompt without restarting a usable ACP session.

#### Acceptance criteria

- **AC-PLATFORM-TURN-CONTINUITY-004.1:** When an initialized, usable ACP conversation returns a correlated prompt error response, Kandev shall preserve that runtime.
  This applies across ACP providers, including providers without error-specific support.
  Error wording, semantic category, and retry classification shall not determine runtime lifetime.
- **AC-PLATFORM-TURN-CONTINUITY-004.2:** Generic internal, storage, authentication, quota, model-selection, and unknown prompt errors shall qualify under the same runtime conditions.
  Missing diagnostic text shall use a safe error explanation.
  Error data shall not grant runtime authority or expose sensitive information.
- **AC-PLATFORM-TURN-CONTINUITY-004.3:** After the failed turn settles, Chat shall show one durable error entry and an enabled composer.
  The next user prompt shall use the same execution, process, connection, and native conversation.
  Repeated prompt failures shall preserve this behavior while the runtime remains usable.
- **AC-PLATFORM-TURN-CONTINUITY-004.4:** Runtime preservation shall require proof that the current prompt received its error response and that its ordered updates finished processing.
  Local send failures, malformed replies, context expiry, and missing proof shall not establish readiness.
  A process exit or connection loss shall override earlier runtime evidence.
- **AC-PLATFORM-TURN-CONTINUITY-004.5:** Cancellation, failed initialization, startup or restore failure, explicit lifecycle actions, and ambiguous prompt settlement shall retain their existing handling.
  A late or duplicate failure shall not stop or settle a successor turn.
  Failure persistence shall finish before another foreground prompt enters the runtime.
- **AC-PLATFORM-TURN-CONTINUITY-004.6:** Runtime preservation shall not authorize automatic retry, continuation, fallback, or model changes.
  Existing recovery classification, effect fences, provider support, timing, and budgets shall remain authoritative.
  Completed or uncertain tools shall not prevent later manual input solely because automatic recovery is unavailable.
- **AC-PLATFORM-TURN-CONTINUITY-004.7:** Desktop, phone, reload, pagination, and additional viewers shall preserve the error explanation and usable Chat.
  Retained errors shall not require Resume or Start fresh.
  New errors shall remain distinct from earlier failed turns and genuine runtime recovery.
- **AC-PLATFORM-TURN-CONTINUITY-004.8:** The failed turn shall remain unsuccessful and shall not advance a success-driven workflow.
  Existing queue and background-work admission rules shall govern later input.
  Office, dynamic routing, automation, utility, passthrough, and non-ACP transport policies remain outside this extension.

- **AC-PLATFORM-TURN-CONTINUITY-004.9:** An error-terminated turn shall not emit a successful turn-finished notification or an implicit needs-answer notification.
  Existing message unread accounting remains independent of task-error acknowledgement.
  This change adds no failure-push notification type.

The October 29 [prompt-error package](../../../plans/acp-prompt-error-continuity/plan.md)
extends criteria 001.1, 001.8, and 002.2 through requirement 004.
Implementation status belongs to that package. Completed earlier packages retain their recorded results.

### REQ-PLATFORM-TURN-CONTINUITY-005: Task attention after failed turns

**Intent:** An idle task with a failed agent turn stays visible until the user acknowledges its error.

#### Acceptance criteria

- **AC-PLATFORM-TURN-CONTINUITY-005.1:** An unresolved retained-turn error shall show the task error icon in the sidebar and corresponding task navigation.
  The task can remain idle with a usable composer.
  A failed turn from any eligible session can raise this task-level attention signal, including while a sibling session runs.
- **AC-PLATFORM-TURN-CONTINUITY-005.2:** The active turn-error explanation shall provide an explicit Close or Dismiss action.
  The action shall clear that observed task attention without deleting the error history or stopping ACP.
  Historical errors shall remain readable after dismissal.
- **AC-PLATFORM-TURN-CONTINUITY-005.3:** A newly accepted manual prompt shall acknowledge only its client-observed task error from the same session.
  When supplied, the request shall carry that client-observed error stamp; a prompt without an observation remains valid and leaves attention unchanged.
  Missing or mismatched evidence shall leave attention unchanged.
  Accepted queued input can acknowledge that stamp. A later queue drain cannot acknowledge an error created after acceptance.
  Drafts, rejected or duplicate sends, queue edits, and automatic or agent-origin prompts shall not acknowledge errors.
- **AC-PLATFORM-TURN-CONTINUITY-005.4:** A normal task with turn-error attention shall expose Dismiss error in its task actions.
  Desktop shall support the task context menu.
  Phone shall provide a visible, touch-accessible task-menu entry point without requiring right-click, hover, or long press.
- **AC-PLATFORM-TURN-CONTINUITY-005.5:** Focusing, opening, reading, or switching to a task shall not dismiss its turn error.
  Unrelated agent output shall not dismiss it.
  Attention and acknowledgements shall survive reload and synchronize across viewers through authoritative task state.
- **AC-PLATFORM-TURN-CONTINUITY-005.6:** A dismissal shall acknowledge only the observed error identity.
  A newer failure, including one from a sibling session, shall remain visible or raise attention again.
  Sending in another session shall not acknowledge that sibling error. Task-level Dismiss can acknowledge its explicitly observed stamp.
  The latest task-level error represents earlier failed turns for attention purposes. Their history remains separate.
- **AC-PLATFORM-TURN-CONTINUITY-005.7:** Turn-error attention shall remain separate from runtime recovery and task activity.
  Acknowledgement shall not clear an independent runtime error, change workflow state, mark work successful, or suppress pending input requests.
  Ordinary error text and recoverable tool failures shall not independently create a terminal turn error.
- **AC-PLATFORM-TURN-CONTINUITY-005.8:** A failed acknowledgement shall retain the visible attention and provide actionable feedback.
  It shall not repeat an already accepted prompt.
  Desktop and phone shall provide the same acknowledgement outcomes with localized controls.

- **AC-PLATFORM-TURN-CONTINUITY-005.9:** When the current retained-turn error permits ordinary manual input, Chat shall offer **Dismiss and continue** beside **Dismiss turn error**.
  The action shall target that error's session, including task-backed Quick Chat.
  Historical errors and errors from another session shall not expose this action.
- **AC-PLATFORM-TURN-CONTINUITY-005.10:** One activation shall submit the exact user message `continue` through normal manual prompt admission.
  Acceptance shall acknowledge only the displayed same-session error stamp under criterion 005.3.
  The user message and error history shall remain visible after reload and in another viewer.
  The action shall preserve the composer draft, attachments, current configuration, and conversation.
- **AC-PLATFORM-TURN-CONTINUITY-005.11:** Repeated activation, another viewer, or a lost response for the same error shall not submit another logical continuation.
  Rejected input shall retain attention and provide localized feedback.
  An acknowledgement failure after acceptance shall retain attention without repeating the accepted prompt.
  A newer error shall retain its own attention.
- **AC-PLATFORM-TURN-CONTINUITY-005.12:** The action shall remain unavailable while automatic recovery or another foreground turn owns the session.
  Existing permission, cancellation, lifecycle, access, and prompt-admission barriers shall remain authoritative.
  Desktop and phone shall expose the same outcome without another confirmation step.
  Phone controls shall stack within Chat and provide targets of at least 44px.
  Busy status shall be accessible, and all new copy shall be localized.

The sidebar amendment extends the October 29 package through requirement 005.
It changes feedback and acknowledgement, without making a retained turn error a runtime failure.
Task-backed Quick Chat receives runtime continuity, durable attention, inline Dismiss, and same-session prompt acknowledgement.
It does not gain a normal sidebar row or task menu. Sidebar/menu criteria apply where those normal task surfaces exist.

## Compatibility and exclusions

The [runtime continuity contract](transient-turn-runtime-continuity.md) owns shared retention and automatic capacity recovery.
This manual action uses existing message admission and acknowledgement.
It adds no automatic retry authority, model switching, runtime restoration, or success-driven workflow transition.
Normal task-menu dismissal remains available independently of inline continuation.

## System design and delivery

- [System design](../system-design/acp-prompt-error-continuity.md)
- [Original delivery package](../../../plans/acp-prompt-error-continuity/plan.md)
- [Dismiss and continue amendment](../../../plans/turn-error-continue-capacity/plan.md)
