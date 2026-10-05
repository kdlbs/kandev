---
status: active
system: platform
created: 2026-10-02
updated: 2026-10-05
owners:
  - Kandev
---

# Provider interruption continuation requirements

## Overview

Recover an interrupted interactive conversation without resending instructions
that have already produced work. Platform owns shared recovery admission,
scheduling, cancellation, and feedback. Agents owns provider capability
translation; Tasks owns conversation records, prompt admission, and workflow
transitions.

This extends [Provider Error Recovery](provider-error-recovery.md) with a
separate continuation contract. Its original-prompt replay fence remains in
criteria `.8` and `.15`; eligible post-output recovery uses the experimental,
default-off continuation path described here.

Continuation permits output and successfully completed foreground tools,
including shell, write, and MCP tools. Pending or uncertain tool outcomes require
manual recovery. The same Cursor conversation receives the internal prompt
`continue`; that prompt does not appear in Kandev chat history. Continuation does
not authorize original-prompt replay or promise exactly-once agent actions.

## Terminology

- **Replay:** Resend the original user prompt following a failed attempt.
- **Continuation:** Restore the same provider conversation and send a new
  instruction to continue its unfinished work using existing history.
- **Recovery episode:** The original interruption and its automatic recovery
  attempts, ending on successful turn completion, cancellation, supersession,
  exhaustion, or a refusal to proceed.
- **Completed foreground work:** Every observed foreground tool has an
  unambiguous provider-reported successful completion. Completion says that
  the tool finished; it does not say that the action is safe to repeat.

## Requirements

### REQ-PLATFORM-INTERRUPTION-CONTINUATION-001: Safe conversation continuation

**Intent:** Temporary provider failures should not require intervention
when conversation restoration and the interrupted work are unambiguous.

#### Acceptance criteria

- **AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1:** On an enabled installation,
  a current, high-confidence short-retryable provider failure in a supported
  concrete-profile task session shall permit continuation after assistant or
  thought output when the provider conversation is restorable and all foreground
  tool outcomes are known and successful. Unsupported providers, passthrough,
  dynamic routing, Office, and utility invocations shall retain their existing
  policies.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2:** A turn containing only
  successfully completed foreground tools shall be eligible regardless of
  tool category, including writes, shell commands, and MCP invocations.
  Pending, failed, cancelled, unknown, malformed, or conflicting outcomes,
  active background work, subagents, and unresolved permissions shall require
  manual recovery. A turn with both a completed tool and an unresolved tool
  shall be refused. Missing or stale evidence shall never authorize continuation.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3:** Automatic continuation shall
  preserve the task session, workspace, provider conversation, selected
  execution profile, model, mode, and permission settings. It shall retain
  the transcript and tool results already persisted by Kandev. The
  [runtime continuity contract](transient-turn-runtime-continuity.md) preserves
  a proven usable runtime; otherwise, restore the provider's saved history
  under the same native session ID. It shall send
  the exact context-independent prompt `continue` instead
  of the original prompt or its attachments, and never silently create a fresh
  conversation or switch providers. Failed restoration shall preserve the
  saved conversation identity.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4:** Existing original-prompt
  replay shall remain restricted to known attempts with no output or tool
  activity. Continuation eligibility shall not make such attempts replay-safe
  or change dynamic fallback eligibility.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5:** Recovery shall begin only
  after the interrupted provider prompt has settled. Provider-owned recovery
  that resumes output before settlement shall not start a second recovery
  loop. Ambiguous continuation-prompt acceptance shall require manual recovery
  rather than resending the continuation.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-001.6:** Automatic `continue`
  shall be an internal dispatch, absent from user-message rows, live chat,
  paginated history, reload, other viewers, and desktop or phone prompt history.
  An explicit user-authored message containing `continue` shall remain visible.
  Recovery status, accepted-turn identity, attempts, and agent output shall
  remain observable.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-001.7:** Supported short-retryable
  resource exhaustion and provider availability failures shall use the same
  continuation admission and episode owner as transport interruptions. Their
  presentation shall retain the actual condition and shall not claim that a
  connection dropped without transport evidence. Hard quota/authentication
  errors and unknown diagnostics shall not enter this path.

### REQ-PLATFORM-INTERRUPTION-CONTINUATION-002: Bounded recovery ownership

**Intent:** Recovery should survive a brief connectivity gap while remaining
bounded and subordinate to user actions.

#### Acceptance criteria

- **AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1:** One recovery episode shall
  own at most five additional automatic attempts, shared by conversation
  restoration and continuation. Its nominal delays shall be 5, 10, 20, 40,
  and 60 seconds. A confirmed transient restoration failure before prompt
  dispatch shall consume an attempt and use the next delay; hard,
  unclassified, and ambiguous failures shall stop automatically. Model output
  alone shall not replenish the episode budget.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2:** Cancel, stop, archive,
  delete, start fresh, a new user prompt, and model or profile changes shall
  supersede pending automatic work. Late callbacks shall not resume the old
  episode or affect its successor. Cancelling waiting recovery shall expose
  the normal composer when runtime continuity is proven, or manual recovery
  actions otherwise; cancelling dispatched continuation shall stop that turn
  through the ordinary cancellation path.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3:** Browser reload or multiple
  viewers shall not dispatch additional attempts. Backend shutdown shall
  cancel recovery. After backend restart, an abandoned process-local recovery
  episode shall expose manual recovery and no live countdown, without
  automatically repeating an uncertain dispatch.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-002.4:** An interrupted turn shall
  remain identifiable as interrupted. Starting recovery shall not report
  successful task completion, advance a workflow, drain queued work ahead of
  recovery, or complete a CI-fix outcome. Normal successful continuation shall
  return to existing completion processing exactly once.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5:** The release toggle shall be
  off in all shipped profiles initially. Disabled installations shall retain
  existing replay and manual-recovery behavior without collecting new
  continuation evidence, dispatching continuation, or advertising it as an
  available automatic operation. Operators shall enable it on a selected
  installation with the existing runtime-toggle precedence and restart rules.

### REQ-PLATFORM-INTERRUPTION-CONTINUATION-003: Accurate recovery feedback

**Intent:** Users should know whether recovery is waiting, reconnecting,
continuing, exhausted, or deliberately unavailable.

#### Acceptance criteria

- **AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1:** Desktop and phone task chat
  shall show at most one current recovery notice containing the selected
  provider, mode of recovery, scheduled attempt, maximum attempts, countdown
  when waiting, and visible Cancel action. Reconnecting and continuing shall
  have distinct labels; a countdown shall not imply browser-owned retry.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2:** A failure for which no
  automatic attempt was dispatched shall not say retries were exhausted.
  Manual recovery shall distinguish unsafe or unknown work, unsupported
  restoration, missing evidence, cancellation, and exhaustion. Only a genuinely
  exhausted episode shall claim exhaustion; its displayed count shall reflect
  attempts actually started. This correction shall also apply with the toggle
  disabled.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3:** Success, cancellation,
  exhaustion, supersession, or shutdown shall retire actionable retry notices.
  Reload and another viewer shall not resurrect a resolved countdown after
  successful cleanup. A failed cleanup shall retain diagnostics and remain
  eligible for later cleanup without creating another actionable notice.
- **AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4:** Phone recovery shall remain
  inline in Chat, use one vertical scroll owner, expose controls of at least
  44px, wrap technical details, and avoid horizontal document overflow. Status
  changes shall be announced without stealing focus or announcing every
  countdown tick. All product copy shall be localized.

## Out of scope

- Original-prompt replay after completed tools, and automatic continuation
  after uncertain tool work.
- Exactly-once execution guarantees for arbitrary agent actions.
- Detecting Wi-Fi changes, altering host networking, HTTP compatibility modes,
  provider purchase/authentication, or interpreting inactivity as a failure.
- New provider switching, dynamic candidate policies, persistent retry jobs,
  response-attempt transcript retraction, or automatic flag promotion.

## System design and delivery

- [System design](../system-design/provider-interruption-continuation.md)
- [Hidden continuation and truthful Cursor errors plan](../../../plans/cursor-hidden-continuation/plan.md)

- [Original implementation package](../../../plans/provider-interruption-continuation/plan.md)
