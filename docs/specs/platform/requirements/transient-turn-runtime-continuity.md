---
status: active
system: platform
created: 2026-10-03
updated: 2026-10-29
owners:
  - Kandev
---

# Transient turn runtime continuity requirements

## Overview

A prompt error can end one turn while its ACP process remains usable.
Keep that process available so the user can continue without restarting the agent.
Platform owns this contract because runtime lifetime and shared recovery admission cross provider and task boundaries.

This capability extends [provider error recovery](provider-error-recovery.md).
It separates runtime preservation from permission to repeat work.
Automatic continuation retains the separate [interruption contract](provider-interruption-continuation.md).
Requirement `004` extends manual continuity to settled prompt errors across ACP providers.
Requirement `003` adds capacity continuation on the same usable runtime.
It permits completed tool effects under the confirmed policy described below.

## Terminology

- **Turn failure:** The provider ends the current prompt without completing the requested work.
- **Usable runtime:** The initialized ACP session retains a live process, an open connection, and a settled foreground prompt.
- **Transient provider error:** A current, high-confidence temporary capacity, overload, rate-limit, availability, or provider-stream error.
- **Runtime failure:** The ACP process exits, its connection closes, or its foreground prompt cannot settle under existing cancellation policy.

## Requirements

### REQ-PLATFORM-TURN-CONTINUITY-001: Preserve usable ACP runtimes

**Intent:** A failed prompt must preserve an otherwise usable agent for later user input.

#### Acceptance criteria

- **AC-PLATFORM-TURN-CONTINUITY-001.1:** When an eligible prompt error ends a turn on a usable runtime, Kandev shall fail the turn and preserve the runtime.
  Requirement 004 defines eligibility across ACP providers.
  The error alone shall not close ACP, stop the process, or remove its execution binding.
- **AC-PLATFORM-TURN-CONTINUITY-001.2:** Prior assistant output or tool activity shall not prevent runtime preservation.
  Completed writes and uncertain tool results shall remain visible and shall continue to prohibit unsafe automatic replay.
- **AC-PLATFORM-TURN-CONTINUITY-001.3:** After settlement, a new user prompt or supported model change shall use the same execution, process, connection, and native conversation.
  Kandev shall not initialize, load, resume, or create another ACP session for that operation.
- **AC-PLATFORM-TURN-CONTINUITY-001.4:** An eligible automatic replay shall retain the existing budget, timing, evidence fence, and backend owner.
  It shall use the preserved runtime while that runtime remains usable.
  Cancelling a waiting retry shall cancel automatic work without stopping the idle runtime.
- **AC-PLATFORM-TURN-CONTINUITY-001.5:** Enabled automatic continuation shall retain its existing provider support, tool-safety checks, native identity, and cancellation contract.
  A preserved usable runtime shall receive the continuation without being restarted.
  An unusable runtime shall retain the existing restoration policy.
- **AC-PLATFORM-TURN-CONTINUITY-001.6:** A runtime failure, failed initialization, explicit stop, archive, deletion, reset, or authorized workflow teardown shall retain its existing recovery policy.
  A later runtime failure shall override earlier evidence of runtime usability.
- **AC-PLATFORM-TURN-CONTINUITY-001.7:** Duplicate or stale failure callbacks shall not settle another turn, stop a successor, or schedule duplicate work.
  Failure settlement shall precede admission of a successor prompt.
  The failed turn shall not trigger ordinary successful-turn workflow processing.
- **AC-PLATFORM-TURN-CONTINUITY-001.8:** Support shall apply to concrete-profile interactive task sessions using ACP, including task-backed Quick Chat.
  Requirement 004 removes provider and transient-classification restrictions for proven prompt error responses.
  Missing runtime evidence, dynamic routing, Office, utility calls, passthrough, and automation-owned executions shall retain their existing policies.

### REQ-PLATFORM-TURN-CONTINUITY-002: Explain failed turns without blocking usable chat

**Intent:** The user needs the provider error and a working composer, not startup recovery controls for a running process.

#### Acceptance criteria

- **AC-PLATFORM-TURN-CONTINUITY-002.1:** When automatic recovery is unavailable or finishes unsuccessfully on a usable runtime, Chat shall show one durable error entry for the failed turn.
  The composer shall remain usable.
  Requirement 005 defines separate task attention and explicit acknowledgement.
  The error shall not require Resume, Start fresh, or workspace restoration.
- **AC-PLATFORM-TURN-CONTINUITY-002.2:** The error shall identify the actual provider condition using sanitized information.
  A retained prompt error shall not become a startup failure in active or historical presentation.
  An execution identifier alone shall not establish a startup failure.
- **AC-PLATFORM-TURN-CONTINUITY-002.3:** Retry feedback shall report actual attempts and their current owner.
  A refused replay shall not claim retry exhaustion.
  Exhaustion or cancellation shall leave the composer available when the preserved runtime remains usable.
- **AC-PLATFORM-TURN-CONTINUITY-002.4:** Reload, pagination, and another viewer shall preserve the error explanation without launching recovery or creating duplicate errors.
  A successful later turn shall not erase the historical failure.
- **AC-PLATFORM-TURN-CONTINUITY-002.5:** Desktop and phone shall provide the same error explanation and continuation capability.
  Phone content shall wrap within Chat, retain its existing scroll owner, and expose required controls through touch interaction.
  New product copy shall be localized.

### REQ-PLATFORM-TURN-CONTINUITY-003: Continue after model capacity errors

**Intent:** A temporary capacity error after completed work shall use automatic recovery without repeating the original request.

#### Acceptance criteria

- **AC-PLATFORM-TURN-CONTINUITY-003.1:** A supported capacity failure on a settled, usable runtime shall permit continuation after assistant output or completed tools.
  Completed shell, write, and MCP actions shall not alone prevent this continuation.
  Pending permissions, pending tools, failed tools, unknown outcomes, conflicting evidence, and unaccounted background work shall prevent automatic continuation.
  Missing or stale evidence shall never authorize recovery.
- **AC-PLATFORM-TURN-CONTINUITY-003.2:** Recovery shall send a new instruction in the same provider conversation to continue unfinished work.
  The instruction shall prohibit repetition of completed actions and require user input for uncertain outcomes.
  Recovery shall preserve the transcript, completed tool results, execution, process, connection, native conversation, workspace, selected model, and permissions.
  It shall not resend the original prompt or its attachments.
  Loss of runtime continuity shall stop this recovery instead of creating or restoring a conversation automatically.
- **AC-PLATFORM-TURN-CONTINUITY-003.3:** Eligible model-capacity recovery on a retained usable runtime shall share one episode with at most eleven additional automatic attempts and a thirty-minute admission window beginning at the first capacity failure.
  Nominal delays shall be 5, 10, 20, 40, 60, 120, 240, 300, 300, 300, and 300 seconds, measured from each preceding failure settlement.
  Valid short provider reset hints shall apply to the first five attempts only.
  A capacity reset hint shall not shorten the nominal delay.
  No retry shall start at or after the window deadline, including when a delayed callback fires late.
  The deadline shall not interrupt an already dispatched turn.
  Output or completed tools shall not replenish the budget.
  Success shall end the episode. A later independent capacity failure shall receive a new budget.
- **AC-PLATFORM-TURN-CONTINUITY-003.4:** Cancel, stop, archive, deletion, reset, a new user prompt, queued user work, and configuration changes shall supersede automatic recovery.
  Duplicate callbacks, reload, and additional viewers shall not create another dispatch.
  An ambiguous dispatch outcome shall require manual continuation.
  Failure settlement shall not report success or advance the workflow.
- **AC-PLATFORM-TURN-CONTINUITY-003.5:** Desktop and phone Chat shall reuse one current retry notice with its reason, attempt, countdown, and Cancel action.
  The notice shall show the recovery-window deadline during capacity recovery and the correct maximum attempt count.
  Success, refusal, cancellation, and exhaustion shall retire the actionable notice.
  Refusal shall report zero started attempts when no dispatch occurred.
  Exhaustion shall report actual started attempts.
  Feedback shall distinguish an expired recovery window, an exhausted attempt budget, a safety refusal, and cancellation.
  A retained usable runtime shall leave the normal composer available.
- **AC-PLATFORM-TURN-CONTINUITY-003.6:** Support shall cover tested Codex and OpenCode ACP capacity evidence in concrete-profile interactive task sessions.
  The capacity path shall operate independently of transport-loss continuation's
  native restoration capability and admission evidence.
  Eligible pre-result replay for the same capacity condition on a retained usable runtime shall use the same extended timing and episode budget, retaining its existing no-output/no-effects admission fence.
  Other error classes, unsupported providers, older components, Office, dynamic routing, automation, and passthrough shall retain their existing policies.
- **AC-PLATFORM-TURN-CONTINUITY-003.7:** Backend shutdown shall cancel waiting capacity recovery. Backend restart shall retire its actionable notice and require manual continuation, even when the runtime survives independently. Browser reload and additional viewers shall retain the latest notice without starting another retry.
- **AC-PLATFORM-TURN-CONTINUITY-003.8:** Every capacity retry shall revalidate current recovery eligibility. A different error class, ambiguous dispatch, uncertain work, or lost runtime continuity shall end the extended episode without granting another capacity budget. Output, completed tools, and changes between eligible replay and continuation shall not reset the original deadline or attempt count.
- **AC-PLATFORM-TURN-CONTINUITY-003.9:** When OpenCode ends its native retries with a supported capacity refusal, Kandev shall admit eligible automatic recovery.
  Recovery shall use the same usable conversation.
  While OpenCode still owns an active prompt, Kandev shall not dispatch another prompt or start a competing retry timer.
  Missing runtime evidence shall retain manual recovery.
  Pending, failed, or uncertain tools shall prevent continuation even when another tool completed.
  Desktop and phone shall use the existing capacity notice, Cancel action, and usable composer.

The October 28 [OpenCode package](../../../plans/opencode-capacity-retry/plan.md)
extended criterion 003.6, added criterion 003.9, and made the short reset hint a
minimum delay in criterion 003.3. Other accepted criteria retain their current
lifecycle.

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
  The request shall carry that error stamp. Missing or mismatched evidence shall leave attention unchanged.
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

The sidebar amendment extends the October 29 package through requirement 005.
It changes feedback and acknowledgement, without making a retained turn error a runtime failure.
Task-backed Quick Chat receives runtime continuity, durable attention, inline Dismiss, and same-session prompt acknowledgement.
It does not gain a normal sidebar row or task menu. Sidebar/menu criteria apply where those normal task surfaces exist.

## Compatibility and exclusions

This changes interactive ACP runtime lifetime after proven prompt error responses.
The protocol response and current runtime state establish manual continuity.
Provider-specific terminal diagnostics still require their existing tested evidence.
It does not make interrupted work successful or permit replay after writes.
It does not change models automatically or add a separate scheduler.
The extended timing applies only to eligible capacity failures on a retained usable runtime; other transient recovery retains its existing limits.
Transport-loss continuation availability is owned by the
[interruption continuation contract](provider-interruption-continuation.md).
Capacity continuation does not guarantee exactly-once agent actions or preservation of an interrupted model invocation.
Completed effects remain in history, and the agent must decide the next unfinished action from that history.
Older remote components without the required evidence retain conservative existing recovery.

## System design and delivery

- [System design](../system-design/transient-turn-runtime-continuity.md)
- [Implementation plan](../../../plans/transient-turn-runtime-continuity/plan.md)
- [Capacity continuation amendment](../../../plans/model-capacity-continuation/plan.md)
- [Delayed capacity retries](../../../plans/delayed-capacity-retries/plan.md) (October 21 timing amendment)
- [OpenCode capacity retry](../../../plans/opencode-capacity-retry/plan.md) (October 28 amendment)

- [ACP prompt error extension](../../../plans/acp-prompt-error-continuity/plan.md) (October 29 amendment)
