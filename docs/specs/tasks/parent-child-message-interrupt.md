---
status: shipped
created: 2026-07-13
owner: cfl
---

# Parent-Child Message Interrupt

## Why

A parent task's `message_task_kandev` call to a busy child (session `RUNNING`/`STARTING`)
is queued and delivered at the child's next turn boundary, like any other peer message.
For a long-running child, that boundary can be minutes or hours away — so a parent's
steer/stop message (`"stop and re-check X"`, `"the requirements changed"`) sits behind
the child's current turn the whole time, with several concurrent children compounding the
delay.

An earlier round of this feature made interruption automatic: any parent-to-child message
cancelled the child's in-flight turn and delivered immediately, purely because the sender
happened to be the target's parent. Review pushback (PR #1653) rejected inferring interrupt
intent from the relationship alone: not every parent message is a stop/steer command — a
parent might send an FYI note that should wait its turn like anything else — so cancelling
a turn must be a deliberate, explicit choice made per call, not a side effect of who is
calling.

## What

### `delivery_mode` request parameter

`message_task_kandev` takes an optional `delivery_mode` string: `"queued"` (default) or
`"interrupt"`.

- **`"queued"` (or omitted):** the message is appended to the target session's FIFO queue
  and delivered when the target's current turn ends — the same behavior every other peer
  message gets, regardless of whether the sender is the target's parent. This is a real
  behavior change from the automatic-interrupt round: a parent messaging a busy child
  without opting in no longer interrupts.
- **`"interrupt"`:** the message is queued *and* the target's current turn is cancelled so
  the message is delivered immediately instead of waiting for the turn to end naturally.

`delivery_mode` only affects dispatch when the target session is `RUNNING` or `STARTING`.
For every other session state (`WAITING_FOR_INPUT`, `COMPLETED`, `CREATED`) the message is
delivered through its normal path (prompt-with-resume or session start) regardless of
`delivery_mode` — there is no in-flight turn to interrupt.

### Authorization: parent-only, hard-rejected otherwise

`delivery_mode="interrupt"` is honored only when the sender is the target task's **direct**
parent (`targetTask.ParentID == senderTask.ID`). This is the same relationship check the
automatic-interrupt round used — it now gates an explicit request instead of driving an
implicit one.

A non-parent sender that explicitly requests `"interrupt"` gets a **hard rejection**, not a
silent downgrade to `"queued"`:

- Response: WS error, code `FORBIDDEN`, message containing `"direct parent"`.
- The rejection has **no side effect** — nothing is queued, dispatched, or interrupted. The
  authorization check runs before the queue insert, so a rejected request never reaches the
  message queue.

This is a deliberate design choice: silently reinterpreting a rejected `"interrupt"` request
as `"queued"` would misreport what happened and hide caller misuse from the agent that made
the request. The caller needs to know its request was rejected outright, not quietly
reinterpreted — if it still wants the message delivered, it can resend with
`delivery_mode="queued"` (or omit the field).

Any `delivery_mode` value other than `"queued"`/`"interrupt"`/empty is rejected up front —
code `VALIDATION_ERROR` — before any task or session lookup runs.

### Status reporting

The response `status` field reflects what actually happened, not just what was requested:

- `"queued"`: the message is sitting in the FIFO queue, waiting for a turn boundary. This is
  the outcome for every `delivery_mode="queued"` call, and also the outcome for an
  `delivery_mode="interrupt"` call whose cancel-and-take step ran but did not end up
  dispatching anything immediately (queued successfully, interrupt not needed or its
  delivery attempt didn't land — see "Composition" below).
- `"sent"`: `delivery_mode="interrupt"` actually cancelled the target's turn and dispatched
  the message right away.
- `"started"`: the target session was `CREATED` and got launched with this message as its
  first prompt (`delivery_mode` has no effect on this path).

A failure in the interrupt's cancel step, once the message is already safely queued, is
never surfaced as an error to the caller: the message will still be delivered by the
target's normal turn-completion drain, so the interrupt is purely a latency optimization on
top of that always-safe default. The call reports `"queued"` in that case (an accurate
description of the actual outcome) instead of an error the caller has no useful recovery
action for — retrying would enqueue a duplicate, since queuing is not idempotent. The
failure is still logged server-side.

### Composition with the FIFO queue and turn-safety guarantees

Decision: [ADR-0035](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)

`delivery_mode="interrupt"` queues the message and cancels-and-takes it in one atomic
orchestrator call, not two separate steps: queuing followed by a separate interrupt call
would leave a window where the target's turn could complete naturally, the ordinary FIFO
drain could pick up the just-queued entry and start dispatching it as a normal turn, and the
interrupt's later cancel could then land on and kill that very turn — orphaning the parent's
message mid-delivery. Atomicity here means the queue insert and the cancel-and-take decision
happen while holding the same per-session serialization point described next.

Every path that can cancel a session's active turn or take-and-dispatch its next queued
message — the parent interrupt, a manual or workflow-triggered queue drain, CI-automation
drains, and clarification-timeout recovery — serializes through one per-session guard. Two
such operations targeting the same session never interleave: whichever acquires the guard
first completes its cancel/take-and-dispatch through prompt acceptance before the other
proceeds. The guard is released immediately after acceptance and is never held while waiting
for the dispatched agent turn to complete. This guarantees at
most one dispatch per queued entry (no double dispatch) and no entry is ever silently
dropped — but it does *not* guarantee that one of the two racing callers itself performs an
immediate delivery. The loser backs off once it observes the winner already changed the
session/turn state; depending on what the winner was, backing off means either "my message
was already delivered by the other side" (the winner was a drain/recovery that delivered
the very entry the loser was also trying to take) or "my message stays safely queued for a
later natural drain" (the winner started a genuinely different, unrelated turn — e.g.
clarification-timeout recovery — that the loser correctly does not cancel into; see
`QueueAndInterruptForPeerMessage`'s "unrelated successor" handling). Either way, the entry
is accounted for exactly once: dispatched, or left queued for its own future turn boundary.

The guard is acquired *before* any turn-completion bookkeeping runs (not just around the
final queue-drain decision). Lifecycle assigns each accepted prompt an immutable generation,
passes it through agentctl, and requires the terminal event to echo it before lifecycle may
flush or complete that prompt. Each resulting turn-ending `agent.ready` signal carries the
immutable `(agent_execution_id, prompt_generation)` captured from that accepted prompt.
Once the guard is held, bookkeeping verifies that identity still owns the session and also
re-validates the active orchestrator turn when turn tracking is wired. A ready event that
raced a parent interrupt and lost cannot complete (or transition the workflow for) a turn the
interrupt already cancelled and replaced, even if delivery did not begin until after the
replacement turn started. It backs off instead, leaving that turn's own eventual completion
to whatever superseded it.
Conversely, if the interrupt's own cancel genuinely fails while a ready event was blocked
behind the same guard, the message is not stranded: the still-pending ready event finds the
turn exactly as it left it (not stale) and delivers the message itself through the ordinary
FIFO drain once it is unblocked.

## API

Request (`message_task_kandev` tool call → `message_task` WS action):

```json
{
  "task_id": "<target task id>",
  "prompt": "<message text>",
  "sender_task_id": "<calling agent's task id>",
  "sender_session_id": "<calling agent's session id>",
  "delivery_mode": "queued"
}
```

- `task_id`, `prompt`, `sender_task_id` are required. `sender_session_id` is optional
  (used for message attribution). `delivery_mode` is optional; omitted is equivalent to
  `"queued"`.

Response:

```json
{
  "task_id": "<target task id>",
  "session_id": "<target session id>",
  "status": "queued"
}
```

`status` is one of `"queued"`, `"sent"`, `"started"` — see "Status reporting" above.

### Errors

| Condition | Code | Notes |
|---|---|---|
| `delivery_mode` not `"queued"`/`"interrupt"`/empty | `VALIDATION_ERROR` | Checked before any task/session lookup. |
| `delivery_mode="interrupt"` from a non-parent sender | `FORBIDDEN` | Message contains `"direct parent"`. No side effect — nothing queued or dispatched. |
| Target session `FAILED`/`CANCELLED` | `INTERNAL_ERROR` | Unrelated to `delivery_mode`; same for every peer message. |
| Target queue full | `CONFLICT` (queue-full error) | Unrelated to `delivery_mode`. |

## Scenarios

**GIVEN** a child task's session is `RUNNING`, **WHEN** its parent calls `message_task_kandev`
without `delivery_mode` (or with `delivery_mode="queued"`), **THEN** the message is appended
to the FIFO queue, the child's current turn is left running untouched, and the response
reports `status="queued"`.

**GIVEN** a child task's session is `RUNNING`, **WHEN** its parent calls `message_task_kandev`
with `delivery_mode="interrupt"`, **THEN** the message is queued and the child's current
turn is cancelled in the same atomic call, the message is dispatched immediately, and the
response reports `status="sent"`.

**GIVEN** a task's session is `RUNNING`, **WHEN** a non-parent (sibling, unrelated, or the
task's own child) sender calls `message_task_kandev` with `delivery_mode="interrupt"`,
**THEN** the call is rejected with `FORBIDDEN` and no message is queued, dispatched, or
otherwise delivered — the sender must resend without `delivery_mode="interrupt"` to get the
message delivered.

**GIVEN** any sender, **WHEN** `message_task_kandev` is called with an unrecognized
`delivery_mode` value (e.g. `"immediately"`), **THEN** the call is rejected with
`VALIDATION_ERROR` before any task or session lookup runs.

**GIVEN** a parent's `delivery_mode="interrupt"` call is queuing and taking its own targeted
entry while a concurrent manual/workflow-triggered drain is already in flight for the same
session (about to take a *different*, already-queued sibling entry), **WHEN** both
operations resolve, **THEN** at most one dispatch happens for each entry — the interrupt
delivers its own entry, and the drain, having lost the race, backs off without
double-dispatching or dropping the sibling entry (which stays queued for a later drain).

**GIVEN** a parent's `delivery_mode="interrupt"` call is racing clarification-timeout
recovery for the same session, **WHEN** the recovery wins the guard and starts a fresh,
unrelated successor turn before the interrupt acquires it, **THEN** the interrupt does not
cancel that successor turn — it defers instead, and the parent's message stays safely
queued for the recovered turn's own eventual natural drain rather than being dropped or
causing a second, competing dispatch.

**GIVEN** a parent's `delivery_mode="interrupt"` call cancels and redispatches a session
through a brand new turn, **WHEN** a `agent.ready` event for the *original* (now-cancelled)
turn is delayed until after the replacement prompt has claimed `RUNNING`, **THEN** that ready
event retains the original prompt generation and backs off without completing the replacement
turn, consuming its pending move, or evaluating its `on_turn_complete` transition.

**GIVEN** clarification recovery has cancelled a stuck turn and its retry prompt has been
accepted but remains incomplete, **WHEN** the direct parent requests
`delivery_mode="interrupt"`, **THEN** the request does not wait for the recovered turn to
complete. It either interrupts that current recovered turn and dispatches the parent message,
or, if it began against the superseded generation, leaves the parent message queued without
loss or duplicate dispatch.

**GIVEN** a parent's `delivery_mode="interrupt"` call's own cancel step genuinely fails
(not one of the tolerated "already idle"/"no active execution" cases) while a ready event
for that same, still-active turn is blocked behind the same guard, **WHEN** the ready event
is unblocked, **THEN** it completes the turn normally and delivers the parent's message
itself via the ordinary FIFO drain — the message is not left stranded.

## Out of scope

- Interrupting a session on behalf of anyone other than its direct parent (siblings,
  grandparents, unrelated tasks) — not supported at any `delivery_mode` value.
- Cancelling more than the target's current turn — a single `delivery_mode="interrupt"` call
  affects exactly one in-flight turn on one session.
- Retrying or deduplicating a failed interrupt attempt automatically — the caller decides
  whether to resend, and a resend queues a distinct message (queuing is not idempotent).
- A `delivery_mode` value that queues *without* the possibility of a later natural interrupt,
  or that interrupts without also queuing — the two are always coupled (interrupt implies
  queue-then-cancel-and-take as one atomic step). This interrupt mode remains the
  preferred control when a direct parent has replacement instructions. A parent
  that instead needs to halt child work without sending a replacement prompt
  uses the separate [Parent-Child Task Stop](parent-child-task-stop.md)
  capability.
