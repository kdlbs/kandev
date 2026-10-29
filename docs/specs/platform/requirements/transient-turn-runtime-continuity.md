---
status: active
system: platform
created: 2026-10-03
updated: 2026-10-28
owners:
  - Kandev
---

# Transient turn runtime continuity requirements

## Overview

A temporary provider error can end one turn while its ACP process remains usable.
Keep that process available so the user can continue without restarting the agent.
Platform owns this contract because runtime lifetime and shared recovery admission cross provider and task boundaries.

This capability extends [provider error recovery](provider-error-recovery.md).
It separates runtime preservation from permission to repeat work.
Automatic continuation retains the separate [interruption contract](provider-interruption-continuation.md).
Requirement `003` adds capacity continuation on the same usable runtime.
It permits completed tool effects under the confirmed policy described below.

## Terminology

- **Turn failure:** The provider ends the current prompt without completing the requested work.
- **Usable runtime:** The initialized ACP session retains a live process, an open connection, and a settled foreground prompt.
- **Transient provider error:** A current, high-confidence temporary capacity, overload, rate-limit, availability, or provider-stream error.
- **Runtime failure:** The ACP process exits, its connection closes, or its foreground prompt cannot settle under existing cancellation policy.

## Requirements

### REQ-PLATFORM-TURN-CONTINUITY-001: Preserve usable ACP runtimes

**Intent:** A temporary provider failure must not force the user to restart an otherwise usable agent.

#### Acceptance criteria

- **AC-PLATFORM-TURN-CONTINUITY-001.1:** When a supported transient error ends a prompt on a usable runtime, Kandev shall fail the turn and preserve the runtime.
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
- **AC-PLATFORM-TURN-CONTINUITY-001.8:** Initial support shall apply to concrete-profile interactive task sessions using tested ACP error shapes.
  Missing evidence, unsupported shapes, dynamic routing, Office, utility calls, passthrough, and automation-owned executions shall retain their existing policies.

### REQ-PLATFORM-TURN-CONTINUITY-002: Explain failed turns without blocking usable chat

**Intent:** The user needs the provider error and a working composer, not startup recovery controls for a running process.

#### Acceptance criteria

- **AC-PLATFORM-TURN-CONTINUITY-002.1:** When automatic recovery is unavailable or finishes unsuccessfully on a usable runtime, Chat shall show one durable error entry for the failed turn.
  The composer shall remain usable.
  The error shall not require Resume, Start fresh, or workspace restoration.
- **AC-PLATFORM-TURN-CONTINUITY-002.2:** The error shall identify the actual provider condition using sanitized information.
  A capacity error shall not become a startup failure in active or historical presentation.
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

## Compatibility and exclusions

This changes the lifetime of supported interactive ACP runtimes after transient errors.
It does not make interrupted work successful or permit replay after writes.
It does not change models automatically or add a separate scheduler.
The extended timing applies only to eligible capacity failures on a retained usable runtime; other transient recovery retains its existing limits.
Transport-loss continuation availability is owned by the
[interruption continuation contract](provider-interruption-continuation.md).
Capacity continuation does not guarantee exactly-once agent actions or preservation of an interrupted model invocation.
Completed effects remain in history, and the agent must decide the next unfinished action from that history.
Older remote components without the new evidence retain conservative existing recovery.

## System design and delivery

- [System design](../system-design/transient-turn-runtime-continuity.md)
- [Implementation plan](../../../plans/transient-turn-runtime-continuity/plan.md)
- [Capacity continuation amendment](../../../plans/model-capacity-continuation/plan.md)
- [Delayed capacity retries](../../../plans/delayed-capacity-retries/plan.md) (October 21 timing amendment)
- [OpenCode capacity retry](../../../plans/opencode-capacity-retry/plan.md) (October 28 amendment)
