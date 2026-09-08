---
status: active
system: platform
created: 2026-08-04
owners:
  - kandev
---
# Mid-Turn Steering Requirements

## Overview

While a Claude session is genuinely generating, an operator who spots a wrong turn has only two bad options: queue a message that lands after the agent has already made the decision the message would have prevented, or cancel and lose the turn's work. Claude Code's own terminal and desktop surfaces accept typed input during generation and act on it at the next tool boundary, so operators arriving from those surfaces experience Kandev as strictly worse at the moment correction matters most.

## Requirements

### REQ-PLATFORM-MID-TURN-STEERING-001: Mid-Turn Steering

**Intent:** While a Claude session is genuinely generating, an operator who spots a wrong turn has only two bad options: queue a message that lands after the agent has already made the decision the message would have prevented, or cancel and lose the turn's work. Claude Code's own terminal and desktop surfaces accept typed input during generation and act on it at the next tool boundary, so operators arriving from those surfaces experience Kandev as strictly worse at the moment correction matters most.

#### Acceptance criteria

- **AC-PLATFORM-MID-TURN-STEERING-001.1:** While a steer-capable session is generating, operator input SHALL reach the running turn rather than waiting for the turn to end.
- **AC-PLATFORM-MID-TURN-STEERING-001.2:** Steering is additive, not destructive: it MUST NOT cancel the running turn, discard completed tool work, or drop the turn's accumulated context.
- **AC-PLATFORM-MID-TURN-STEERING-001.3:** Steer eligibility is **negotiated from the connected agent**, never inferred from an agent name or a normalized message shape. An agent that does not advertise the capability keeps exactly today's queue behavior.
- **AC-PLATFORM-MID-TURN-STEERING-001.4:** Delivery is **opportunistic**. Kandev delivers the steer to an agent that advertises it can accept a prompt while one is in flight; whether the agent folds that prompt into the live turn or runs it as the next turn is the agent's decision and is not advertised over the protocol. Both outcomes MUST produce correct transcripts, correct attribution, and no operator-visible error.
- **AC-PLATFORM-MID-TURN-STEERING-001.5:** A steered turn's completion SHALL be attributed to the work that actually produced it. When an agent settles the predecessor's prompt early to hand the turn to the steer, that early settlement MUST NOT be presented to the operator as the predecessor's answer, and MUST NOT trigger predecessor-owned cleanup that would sweep live tool work, consume the successor's usage, or clear the successor's trace.
- **AC-PLATFORM-MID-TURN-STEERING-001.6:** Background work that was live when the turn was handed off SHALL stay live and continue to report, including work whose completion arrives with no prompt in flight.
- **AC-PLATFORM-MID-TURN-STEERING-001.7:** Message order is preserved. While a session still has queued messages, new input continues to queue; steering applies only when nothing is already queued ahead of it.
- **AC-PLATFORM-MID-TURN-STEERING-001.8:** At most one steer is in flight per session. A second steer attempt while one is unacknowledged queues instead.

## Migrated source detail

## Why

While a Claude session is genuinely generating, an operator who spots a wrong
turn has only two bad options: queue a message that lands after the agent has
already made the decision the message would have prevented, or cancel and lose
the turn's work. Claude Code's own terminal and desktop surfaces accept typed
input during generation and act on it at the next tool boundary, so operators
arriving from those surfaces experience Kandev as strictly worse at the moment
correction matters most.

## What

- While a steer-capable session is generating, operator input SHALL reach the
  running turn rather than waiting for the turn to end.
- Steering is additive, not destructive: it MUST NOT cancel the running turn,
  discard completed tool work, or drop the turn's accumulated context.
- Steer eligibility is **negotiated from the connected agent**, never inferred
  from an agent name or a normalized message shape. An agent that does not
  advertise the capability keeps exactly today's queue behavior.
- Delivery is **opportunistic**. Kandev delivers the steer to an agent that
  advertises it can accept a prompt while one is in flight; whether the agent
  folds that prompt into the live turn or runs it as the next turn is the
  agent's decision and is not advertised over the protocol. Both outcomes MUST
  produce correct transcripts, correct attribution, and no operator-visible
  error.
- A steered turn's completion SHALL be attributed to the work that actually
  produced it. When an agent settles the predecessor's prompt early to hand the
  turn to the steer, that early settlement MUST NOT be presented to the
  operator as the predecessor's answer, and MUST NOT trigger predecessor-owned
  cleanup that would sweep live tool work, consume the successor's usage, or
  clear the successor's trace.
- Background work that was live when the turn was handed off SHALL stay live and
  continue to report, including work whose completion arrives with no prompt
  in flight.
- Message order is preserved. While a session still has queued messages, new
  input continues to queue; steering applies only when nothing is already
  queued ahead of it.
- At most one steer is in flight per session. A second steer attempt while one
  is unacknowledged queues instead.
- Synthetic prompts (`ScheduleWakeup`) are never steers and can neither consume
  nor race a steer.
- The operator can tell that input will be **delivered now rather than held**,
  which is the only promise Kandev can honestly make: the capability is
  advertised by the bridge, while whether the message folds into the live turn is
  decided by the separately-versioned agent CLI underneath it. The composer
  affordance therefore commits to delivery, never to folding, and the deferred
  outcome MUST be indistinguishable from an ordinary queued message arriving.
- The behavior ships behind a default-off runtime toggle and remains a
  kill-switch after rollout.

## API surface

Reuses the existing session contracts in
[background-work-liveness](background-work-liveness.md); this feature adds:

- **Agent capability (inbound, ACP `initialize`)** —
  `agentCapabilities._meta.claudeCode.promptQueueing: true` is the negotiated
  precondition that the agent accepts `session/prompt` while another is in
  flight. Kandev already retains `agentCapabilities` including its `_meta` map.
  No new protocol method is introduced and no new ACP RPC is required.
- **Session record / boot payload / `session.state_changed` /
  `session.activity_changed`** — a boolean `supports_steering`. True only when
  the session's connected agent advertised the capability, the runtime toggle is
  enabled, and the session is promptable-or-generating. Absent or false means
  the composer keeps queue behavior.
- **Prompt admission** — the existing prompt path accepts a steer intent. A
  steer is admitted while the durable session state is `RUNNING`; every other
  coarse state follows the existing admission table.
- **Runtime toggle** — `features.claudeMidTurnSteering`, environment variable
  `KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING`, experimental, high risk,
  restart-required, `"false"` in every embedded profile.

## State machine

Per-session steer lifecycle, additive to the coarse session lifecycle:

| State | Meaning | Transition trigger |
|---|---|---|
| `none` | No steer outstanding | initial, or terminal steer settled |
| `offered` | Session is generating and steer-capable | capability + toggle + generating |
| `sent` | Steer delivered to the agent, unacknowledged | operator submits while `offered` |
| `folded` | Agent took the steer into the running turn | predecessor settles without having produced its own answer |
| `deferred` | Agent ran it as its own turn instead | predecessor settles after producing its own answer |
| `none` | | successor turn completes, or session cancelled |

`folded` and `deferred` are both success. They differ only in how completion is
attributed, and the operator sees a correct transcript either way.

The coarse admission table from
[background-work-liveness](background-work-liveness.md) is otherwise unchanged:
a steer is the single new reason a `RUNNING` session accepts direct input.

## Failure modes

- The agent does not advertise the capability: input queues exactly as today. No
  steer affordance is shown.
- The toggle is off: input queues exactly as today, for every agent.
- The agent advertises the capability but rejects the concurrent prompt: the
  steer is reported as queued, the operator sees no error, and the message runs
  as the next turn.
- The agent accepts the steer but never folds it: `deferred`. Correct
  transcript, no error.
- **The advertised capability and the actual behavior come from independently
  versioned components.** The bridge advertises `promptQueueing`; the agent CLI
  beneath it decides whether to fold, and its version is not observable over the
  protocol (the ACP `agentInfo.version` is the bridge's). An installation whose
  bridge is current but whose CLI is too old to fold therefore reports itself
  steer-capable and produces `deferred` on every steer. This is a supported
  configuration, not an error: it degrades to exactly the queue behavior that
  installation has today. Kandev MUST NOT gate on, infer, or warn about a CLI
  version it cannot see.
- The predecessor's early settlement arrives while the successor is producing
  output: the predecessor's settlement is stale by generation and MUST be
  suppressed before it can emit completion, sweep tool ownership, consume usage,
  or clear the successor's trace.
- Usage arrives attributed entirely to one of the two turns (observed: the
  predecessor's early settlement reports zeroed usage and the real usage lands
  on the successor). Usage MUST be recorded once, against whichever turn the
  agent reports it on, and never double-counted across the boundary.
- The operator cancels during a steer: both the predecessor and the steer are
  cancelled, the prompt gate is released, and the session does not remain
  falsely busy.
- The agent connection dies mid-steer: the steer fails like any in-flight
  prompt, and the session returns to a promptable coarse state.
- Background work whose completion produces a model cycle with no prompt in
  flight MUST still be surfaced and MUST still settle the session's activity;
  the steer boundary does not orphan it.
- A steer whose session was replaced (new/loaded session) while it waited is
  dropped rather than delivered to the new session.
- **The steer must attach to a live, dispatched foreground turn, never a
  half-started or finished one.** A steer reuses the predecessor turn's identity
  so the folded completion is attributed to the predecessor's still-open waiter.
  Therefore a steer that arrives after the foreground turn was admitted but
  before its prompt actually reached the agent MUST NOT overtake it (its stream
  buffers are still being reset); and a steer that arrives after the foreground
  turn has already completed MUST NOT attach to that finished turn. Both cases
  degrade to an ordinary prompt delivered in submission order — never a steer
  bound to a non-live turn, which would strand the operator's message on a
  completion that never arrives.
- **A steer dispatch error is never silently re-sent.** The agentctl request can
  be written to the agent and then have its acknowledgement fail (a stream
  disconnect or context cancellation after the write), so a steer that returns an
  error may already be in flight. The handler therefore surfaces the error rather
  than re-running the message as an ordinary prompt, which could deliver it twice.
  Only a steer that provably never reached the agent — the session became
  ineligible before dispatch, or there was no live turn to fold into — degrades to
  an ordinary prompt.

### Outcome taxonomy

Four distinct outcomes are specified above, and only the last one is an error.
Collapsing them is the mistake this table exists to prevent — in particular,
"the agent refused the concurrent prompt" is a **success**, not a failure.

| Outcome | When | Operator sees |
|---|---|---|
| **Steered** | The agent accepted the concurrent prompt and folded it into the running turn. | The message acts on the running turn. |
| **Deferred** | The agent accepted the concurrent prompt but ran it as the next turn — including an agent that refuses concurrency outright, and an installation whose CLI is too old to fold. | Reported as queued. Correct transcript, **no error**. |
| **Degraded to an ordinary prompt** | The steer provably never reached the agent: the session became ineligible before dispatch, or there was no live dispatched turn to fold into (admitted-but-not-dispatched, or already completed). | Ordinary queue behavior, in submission order. **No error.** |
| **Errored** | The write or its acknowledgement failed *ambiguously* — delivery to the agent cannot be ruled out. | The error surfaces. The message is **never** re-sent, because it may already be in flight. |

## Known residuals

These are narrow, accepted windows inherent to opportunistic steering over an
asynchronous agent boundary. All are gated behind the default-off toggle.

- **Acknowledgement is not delivery.** agentctl acknowledges a steer, then runs
  the concurrent `session/prompt` on a goroutine. Once the request has been
  accepted no client-side fallback can fire, so if the concurrent prompt then
  fails on an agent that advertised prompt queueing yet rejects it (abnormal),
  the message can be stranded with success already reported.
- **The fold is decided agent-side, and the predecessor may settle first.** The
  lifecycle lock closes the backend-side selection/dispatch race, but the
  acknowledgement proves acceptance, not a fold: the predecessor turn is
  suppressed only once the successor actually acquires (transfers) the prompt
  gate, which happens when the async steer runs — not when it is accepted. If the
  predecessor settles agent-side before that transfer (it naturally ends, or the
  agent runs the steer as a fresh turn instead of folding), the steer's turn is
  tagged with the reused generation and its completion bookkeeping
  (on_turn_complete, usage) is discarded as a duplicate. The operator's message
  is still delivered; only that turn's second completion is lost. This window is
  inherent to opportunistic delivery over an async boundary and is not closable
  from the Kandev side.
- **Steer history ordering.** A dispatched steer's user message is appended to
  the agent's resume history after the lifecycle lock is released (so a slow
  history store never extends the lock). A completion can settle in that narrow
  window, so on resume the steer message may order after the turn it folded into.
  The durable DB transcript is unaffected; this is a resume-fidelity residual.
- **Session replacement during a fall-through wait.** A steer that finds no
  handoff-eligible turn (e.g. a synthetic wakeup holds the gate) falls through to
  ordinary gate acquisition. If the ACP session is replaced (`session/new` /
  `session/load`) while it waits there, it is delivered to the replacement rather
  than dropped. The drop guarantee above holds for the wakeup-pinned path; this
  fall-through path is the residual.

## Persistence guarantees

Steer state is in-memory and authoritative only while the owning agent
execution is connected. A restart does not reconstruct an in-flight steer, a
`sent` steer's acknowledgement, or the fold-vs-defer outcome. Durable coarse
session state remains the source of truth for prompt admission after restart.
`supports_steering` is derived at serialization time from the live connection
and is never persisted.

## Scenarios

- **GIVEN** the toggle is off, **WHEN** the operator submits input to a
  generating session, **THEN** Kandev queues it and shows no steer affordance.
- **GIVEN** a steer-capable generating session with an empty queue, **WHEN** the
  operator submits input, **THEN** Kandev delivers it to the running turn
  without cancelling that turn.
- **GIVEN** an agent that does not advertise `promptQueueing`, **WHEN** the same
  input is submitted to a generating session, **THEN** Kandev queues it and the
  session's `supports_steering` is false.
- **GIVEN** a delivered steer that the agent folded, **WHEN** the agent settles
  the predecessor's prompt before that prompt produced any answer, **THEN**
  Kandev does not present that settlement as the predecessor's answer and does
  not emit a completion for it.
- **GIVEN** a folded steer whose predecessor had live background work at the
  boundary, **WHEN** the predecessor's prompt settles, **THEN** that background
  work stays live and its later completion is still reported.
- **GIVEN** a folded steer, **WHEN** both turns have settled, **THEN** the
  session's recorded usage equals what the agent reported and no token or cost
  is counted twice.
- **GIVEN** a delivered steer that the agent ran as its own turn instead,
  **WHEN** both turns settle, **THEN** the transcript shows both answers in
  submission order and no error is surfaced.
- **GIVEN** a steer-capable session whose underlying agent CLI is too old to
  fold, **WHEN** the operator steers a generating session, **THEN** the message
  runs as the next turn, the transcript is correct, and the operator sees no
  error and no version warning.
- **GIVEN** a session with one already-queued message, **WHEN** the operator
  submits again while generating, **THEN** the new message queues behind the
  first rather than steering.
- **GIVEN** a steer that has been sent and not acknowledged, **WHEN** the
  operator submits again, **THEN** the second message queues.
- **GIVEN** a scheduled wakeup due while a steer is in flight, **WHEN** the
  wakeup fires, **THEN** it waits for the owning prompt rather than consuming
  the steer's handoff.
- **GIVEN** an in-flight steer, **WHEN** the operator cancels the session,
  **THEN** both the predecessor turn and the steer end and the session becomes
  promptable.
- **GIVEN** an in-flight steer, **WHEN** the active session is replaced by a new
  or loaded session, **THEN** the steer is dropped rather than delivered to the
  replacement.
- **GIVEN** a steer-capable session that is idle rather than generating,
  **WHEN** the operator submits input, **THEN** it dispatches as an ordinary
  prompt.

## Out of scope

- Adding a steer RPC to ACP, or adopting an agent-specific steer primitive such
  as Codex `turn/steer`. This feature uses only the existing prompt method.
- Making the fold itself detectable in advance. The agent does not advertise it
  and Kandev must not infer it from a version string.
- A minimum agent-CLI version floor, a version warning, or any version-derived
  gate. The CLI version is not observable over the protocol, and gating on the
  bridge version would gate on the wrong component.
- Backward-compatibility branches for older agents. There is nothing to branch
  on: an agent that cannot fold produces `deferred`, which is a specified
  success path, so old and new installations run the same code.
- Editing, reordering, or withdrawing an already-queued message.
- Steering a subagent's turn rather than the session's foreground turn.
- Changing the coarse busy signal or the activity tiers owned by
  [background-work-liveness](background-work-liveness.md).
- Reconstructing steer state across a restart.

## Decision record

Supersedes the mid-turn-steering non-goal in
[background-work-liveness](background-work-liveness.md) and the out-of-scope
note in
[ADR 0049 — Fine-grained foreground-idle busy signal](../../../decisions/0049-fine-grained-foreground-idle-busy-signal.md),
which recorded that mid-turn steering "needs ACP concurrent-prompt support and
per-agent capability gating". The concurrent-prompt precondition is now
observed to be satisfied by the shipped Claude bridge; the capability gating
requirement stands and is specified above.

## Implementation plan

[Mid-turn steering plan](../../../plans/mid-turn-steering/plan.md)
