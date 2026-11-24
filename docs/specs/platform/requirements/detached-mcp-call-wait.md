---
status: draft
system: platform
created: 2026-10-26
owners:
  - kandev
---

# Detached MCP Call Wait

## Overview

When the stream between the backend and an agentctl is down (VPN reset,
backend restart, SSH drop), the agent keeps running on its host. Every Kandev
MCP call it makes (step completion, plan read or write, user questions) then
fails after 5 seconds. A step completion sent during the outage is lost, and
the agent loops on sleep and retry while the task shows running.

This capability makes those calls wait for the stream instead. Nothing is
queued or replayed on the agent's behalf. The call is made once, after the
stream is back, by the agent's own request. A bounded offline budget stops an
agent that nobody can see or stop.

[Durable agent reattachment](../system-design/durable-agent-reattachment.md)
and [durable agent delivery](durable-agent-delivery.md) own detached output,
journal replay, and adoption. This document adds only the agent-to-Kandev
direction. It supersedes the call-waiting portion of the closed draft in
kdlbs/kandev#4255. Its Disconnected state, redial, and banner are not part of
this capability.

## Terminology

- **Instance:** one agent instance, meaning one agent process and its
  process manager, hosted by an agentctl process. One agentctl hosts many
  instances. Every attach, detach, clock, budget, and cancel in this document
  is per instance.
- **Detached:** the instance has no live backend stream.
- **Attached:** the instance has at least one live backend stream.
- **Writer-ready:** the instance's stream writer is serving Kandev calls for
  the current attachment. It is not ready during the journal replay that runs
  after an attach.
- **Stable attachment:** an attachment that has had at least one live stream
  for 30 consecutive seconds.
- **Kandev call:** a Kandev MCP tool call from the agent that needs the
  backend to answer.
- **Waiting call:** a Kandev call that agentctl has received but not yet
  bound to a backend stream.
- **Bound call:** a Kandev call that agentctl has bound to one backend stream
  and handed to that stream's writer. A bound call may or may not have been
  written.
- **Offline budget:** the longest time an instance may go without a stable
  attachment. An attachment shorter than 30 seconds does not interrupt it.
  When it passes, agentctl cancels the running turn.

## Requirements

### REQ-PLATFORM-DETACHED-MCP-WAIT-001: Kandev calls wait while detached

**Intent:** An outage must not turn into a failed call, a lost step
completion, or an agent retry loop.

**User story:** As a user, I want my agent's Kandev calls to wait for the
connection, so that a short network drop does not lose a step completion.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-MCP-WAIT-001.1:** While the instance is detached, or
  attached but not writer-ready, a Kandev call shall wait. It shall not
  return an error before the offline budget expires. The only other exits are
  the caller abandoning the call (001.4), a reset of the agent session (which ends the calls of that MCP session only and changes no attachment, budget, or decision state), and
  the client being closed; these return the errors they return today.
- **AC-PLATFORM-DETACHED-MCP-WAIT-001.2:** When a backend stream becomes
  writer-ready, each waiting call shall be bound to that stream, written to
  it, and answered by the backend's response to it. A call that waited is not
  subject to the 5-second send timeout of 001.5 until the stream is
  writer-ready.
- **AC-PLATFORM-DETACHED-MCP-WAIT-001.3:** A Kandev call shall take effect only
  when the backend processes it. agentctl shall not complete a step, answer a
  question, or change a plan on the backend's behalf, and shall not persist a
  waiting call. A waiting call does not survive an agentctl exit.
- **AC-PLATFORM-DETACHED-MCP-WAIT-001.4:** When the agent's MCP client abandons
  a call that is not yet bound, agentctl shall withdraw it. A withdrawn call
  shall never be bound or written. Withdrawal and binding are decided
  atomically: a call is written only if it was still pending when it was
  bound. A call abandoned after it was bound is not withdrawn: its caller returns its own cancellation error at once, agentctl removes the pending entry, and a later backend response or stream end for it is discarded under 003.5. It never fails with an unknown outcome, because no caller is left to read it.
- **AC-PLATFORM-DETACHED-MCP-WAIT-001.5:** While the instance is attached and
  writer-ready, a Kandev call that cannot be handed to the stream writer
  within 5 seconds of the attempt shall fail with the existing send-timeout
  error, as it does today. If the stream ends during those 5 seconds, the
  call returns to waiting under 001.1 instead of failing, and the 5 seconds
  restart at the next writer-ready.
- **AC-PLATFORM-DETACHED-MCP-WAIT-001.6:** A Kandev call that was waiting and is
  written to a stream shall receive no earlier answer than the backend's.
  Concurrent waiting calls from different tool invocations have no delivery
  order guarantee. Calls from one caller that waits for each answer keep the
  caller's order.
- **AC-PLATFORM-DETACHED-MCP-WAIT-001.7:** A best-effort notification that
  the MCP server sends on its own behalf, with no agent call waiting on it
  (the clarification-timeout notice), shall keep the 5-second send timeout
  and shall not wait.

### REQ-PLATFORM-DETACHED-MCP-WAIT-002: Progress notifications keep the client waiting

**Intent:** A waiting call must not be aborted by the agent's tool client
before the offline budget decides it.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-MCP-WAIT-002.1:** While any Kandev call is in flight,
  the MCP server shall send a progress notification for that call at intervals of 20 seconds or less, measured from the start of one notification to the start of the next, from receipt until its result is handed
  to the MCP transport. The notification carries the client's progress token,
  or a token the server synthesizes when the client sent none, which is unique among the calls in flight on that MCP server. Each call has
  exactly one notification stream, and its progress value increases with
  every notification.
- **AC-PLATFORM-DETACHED-MCP-WAIT-002.2:** The MCP server shall send no progress
  notification for a call once its result has been handed to the MCP
  transport. No send starts after the stop is requested. A failure to send a notification shall not fail the call. Each notification send is abandoned after 5 seconds, so a blocked send neither stretches the interval past 20 seconds nor delays the result by more than 5 seconds.
- **AC-PLATFORM-DETACHED-MCP-WAIT-002.3:** The notification text shall not
  state that the call is waiting for a person unless the call is
  `ask_user_question`.

### REQ-PLATFORM-DETACHED-MCP-WAIT-003: Outcome unknown is reported, not retried

**Intent:** A call that may have reached the backend must not be replayed
blindly or reported as a clean failure.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-MCP-WAIT-003.1:** When a stream ends after a Kandev
  call was bound to it and before its response arrived, the call shall fail
  with an error whose text states that the outcome is unknown and that the
  agent must check state before retrying.
- **AC-PLATFORM-DETACHED-MCP-WAIT-003.2:** agentctl shall not resend a call that
  failed with an unknown outcome.
- **AC-PLATFORM-DETACHED-MCP-WAIT-003.3:** A call that was never bound to a
  stream shall never fail with an unknown outcome. It keeps waiting. A call
  whose stream had already ended when it tried to bind is not bound; it
  returns to waiting. A call whose request cannot be encoded fails with an ordinary error, not an unknown outcome. The request is encoded before the call is bound, so an unencodable call is never bound and no stream end can change its error.
- **AC-PLATFORM-DETACHED-MCP-WAIT-003.4:** A call shall be bound to its stream
  before the write is attempted, and a call is bound only if its stream has
  not ended. A stream that ends during or immediately after the write shall
  still fail the call under 003.1, including a call that was bound but never
  written. No call shall remain pending without a stream or a waiting state.
- **AC-PLATFORM-DETACHED-MCP-WAIT-003.5:** A backend response that arrives for a
  call that already failed shall be discarded.
- **AC-PLATFORM-DETACHED-MCP-WAIT-003.6:** When a response and the end of its
  stream race, whichever the call's state lock admits first decides the
  call: an admitted response completes it with that response, and the later
  stream end changes nothing.

### REQ-PLATFORM-DETACHED-MCP-WAIT-004: Offline budget

**Intent:** An agent nobody can see or stop must not run without bound.

**User story:** As a user, I want an unreachable agent to pause after a known
period, so that a long outage cannot let it run away.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-MCP-WAIT-004.1:** The offline budget shall be 15
  minutes. An executor profile can set `offline_budget_minutes` to a base-10
  integer from 1 to 1440, surrounding spaces ignored. An absent or empty value
  means 15.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.2:** A profile save or launch with any
  other `offline_budget_minutes` value (a sign, fraction, unit, zero, or
  overflow) shall fail with a validation error that names the key and the
  accepted range. A launch shall not substitute the default.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.3:** For each instance, the budget clock
  shall start when the last backend stream ends. For an instance that has
  never had a stream, it starts when the instance is created. The deadline is
  the clock start plus the budget.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.4:** The clock shall restart, with a new
  start and a new deadline, only after a stable attachment. An attachment
  that ends before 30 seconds shall neither restart the clock nor move the
  deadline. When the deadline passes while the instance is attached but the
  attachment is not yet stable, the expiry decision shall be taken at the
  moment the instance next becomes detached, without a new full budget,
  unless the attachment becomes stable first, in which case no expiry
  occurs.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.5:** When the expiry decision is taken
  and a turn is running, agentctl shall cancel that turn through the same
  agent cancel as a user cancel, and shall record the decision before it
  fails any waiting call or cancels the turn. The cancel applies only if the
  turn that was running at the decision is still the running turn when the
  cancel is issued. A permission request pending in that turn shall be
  cancelled with it, neither approved nor denied. The turn's terminal event shall be recorded with stop reason `cancelled` and cause `offline_budget` even when the agent ends its turn on its own after the decision, unless the cancel failed under 004.11. A terminal event committed before the decision is unchanged. "Before" and "after" are decided by one ordering point: the step that stamps a terminal event and the decision both run under the instance's tracker mutex, so each terminal is wholly before or wholly after the decision. A running turn is a submission that the instance tracks as in flight from the moment its prompt is actually dispatched (not from admission, so a re-admitted settled submission is never in flight) until its dispatch returns, which requires durable delivery; an instance without a journal has no running turn, so 004.7 applies to it. Budget cancellation applies to a turn that ends with a completion terminal; a turn that ends with an error terminal keeps its error and gets no budget stamp. The stamp step reads the cancel's state at the moment it runs: the budget cause is stamped unless that state is already failed or skipped, so a cancel that is still outstanding, or acknowledged, stamps it, and a failure that arrives after the terminal was stamped changes nothing. A decision stays on record until the terminal of its turn is stamped, even after a stable attachment clears the exhaustion.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.6:** When the expiry decision is taken,
  every waiting call shall return an error stating that the offline budget
  was reached and that Kandev is unreachable. The error shall tell the agent
  to stop and end its turn without polling or sleeping. While the instance
  is detached after expiry, a Kandev call shall return the same error
  immediately. While it is attached and writer-ready, a Kandev call is written normally under 001, even before the next stable attachment. While it is attached but not yet writer-ready after expiry, a call waits under 001.1; it does not receive the budget error. A call that a reconnecting stream binds before the decision's failure pass reaches it is a bound call and is written; a call the failure pass reaches first receives the budget error. Exactly one of the two applies to each call.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.7:** When the budget expires with no
  running turn, agentctl shall cancel nothing and shall still apply 004.6.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.8:** After expiry the budget capability
  shall not stop the instance or agentctl and shall keep the journal. It
  shall start no turn on its own. It adds no exit path. The existing
  ownership and idle reapers keep their own periods unchanged, so the
  instance ends at whichever bound comes first.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.9:** A stream that attaches before the
  expiry decision is made shall prevent the cancel. A stream that attaches
  after the decision shall not undo it.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.10:** An instance created by a build of
  agentctl that has this capability, with no budget supplied, shall use 15
  minutes. An adopted instance keeps the budget it was created with. The budget field of the instance creation request is a JSON integer; absent or null means 15, and any other value outside 1 to 1440 (zero, negative, non-integer, or a string) makes agentctl reject the creation with a validation error naming the field and range, never substituting the default. A message client that has no attachment signal wired keeps today's 5-second send failure and never waits. An
  agentctl build without this capability has no offline budget and keeps the
  5-second failure of 001.5 for every call; the backend shall still launch
  instances on it.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.11:** When the agent cancel issued under 004.5 fails or is not acknowledged (the cancel returns an error, or returns nothing within 5 seconds), agentctl shall record a `budget_cancel_failed` event (a structured log entry with that reason and a counter increment, with no durable record), leave the turn running, and neither retry, kill the agent, nor retire the submission. A terminal stamped after that failure is not stamped with the budget cause: it settles and replays as an ordinary completion under 005.1. Calls keep failing under 004.6
  while the instance is detached.
- **AC-PLATFORM-DETACHED-MCP-WAIT-004.12:** When an instance is created with a
  budget at or above the control server's effective unowned period or the
  idle timeout, agentctl shall log a warning that names the budget and that
  period.

### REQ-PLATFORM-DETACHED-MCP-WAIT-005: Replay records what happened while detached

**Intent:** The user must be able to see, after reconnect, what the agent did
without Kandev.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-MCP-WAIT-005.1:** agentctl shall stamp a turn's
  terminal event, at the moment it commits it, with whether the instance was
  detached and the commit time. When a turn that was not cancelled by the
  budget ended while detached, the workflow actions on turn completion shall
  apply once after replay, as durable delivery defines, and the conversation
  shall record the commit time and that Kandev was unreachable.
- **AC-PLATFORM-DETACHED-MCP-WAIT-005.2:** When the offline budget cancelled a
  turn, the replayed completion shall settle as an ordinary cancel and shall
  trigger no automatic workflow advance, including by an older backend. The
  conversation shall record the time of the expiry decision and that the
  agent waits for the user's instruction. That turn gets this record only;
  it does not also get the 005.1 record.
- **AC-PLATFORM-DETACHED-MCP-WAIT-005.3:** Records and stamps apply to completion terminals committed to the durable journal; an error terminal gets the detached stamp fields but no record, and an instance without a journal stamps and records nothing. Each record shall appear once per turn terminal, however many times replay or projection runs for it or concurrently, and shall render on desktop and phone in all seven shipped locales. A terminal without the stamp, from an agentctl build without this capability, gets no record. A terminal stamped detached whose commit time is missing or unparseable still gets its record, dated at the projection time.

### REQ-PLATFORM-DETACHED-MCP-WAIT-006: Harness timeout and agent guidance

**Intent:** The harness must not abort a waiting call before the budget
decides it, and the agent must not invent a polling loop.

#### Acceptance criteria

- **AC-PLATFORM-DETACHED-MCP-WAIT-006.1:** For an agent that declares a
  tool-timeout environment key, launch and resume of a new instance shall set
  the managed default of that key, in milliseconds, to at least the resolved
  offline budget plus 10 minutes. An adopted instance's environment is not
  rewritten.
- **AC-PLATFORM-DETACHED-MCP-WAIT-006.2:** When a higher-precedence source sets
  that key to a positive integer number of milliseconds below the resolved
  budget plus 1 minute, launch shall fail with a typed error that names the
  key, its value, and the budget. When that source sets a value that is not a
  positive base-10 integer (empty, zero, signed, fractional, unit-suffixed,
  overflowing, or a secret reference), launch shall fail with a typed error
  that names the key and the budget and does not repeat the value. Launch
  shall not substitute the default in either case.
- **AC-PLATFORM-DETACHED-MCP-WAIT-006.3:** The Kandev system context given to
  the agent shall state that Kandev calls can wait through connection loss and
  shall not be retried in sleep or polling loops.

## Out of scope

- Redialing a lost link for SSH, remote Docker, and Sprites, the Disconnected
  session state, and its banner. A follow-up needs a transport re-establish
  contract per executor.
- Journaling agent MCP requests in agentctl and replaying them on reattach.
  Calls wait or fail; nothing is queued. Blind replay of step completion or a
  user question after the world has moved is unsafe.
- A request-id idempotency key on step completion. A narrower later
  refinement; a user question already has `clarificationRetryKey`.
- A tool-timeout floor for agents that declare no tool-timeout key (Codex
  among them). They receive the 002 notifications, which reset their timer
  only when the client supplied a progress token and honors it. Otherwise
  their own timeout applies and the call is withdrawn under 001.4. A
  measurement of whether Codex needs a floor is a follow-up.
- Detecting a half-open stream. A VPN black hole that leaves the TCP
  connection open keeps the instance attached, so calls behave as today
  (001.5) until the transport reports the stream ended.
- Changing the unowned or idle reaper periods. With default settings and
  durable agent survival on, the 10-minute unowned period ends the instance
  before the 15-minute budget; 004.12 logs that combination.
- Exactly-once execution of tools or MCP calls.
- Detached output, journal replay, and adoption, which durable delivery and
  reattachment own.
