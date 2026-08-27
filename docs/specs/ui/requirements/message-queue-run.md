---
status: active
system: ui
created: 2026-08-16
owners:
  - kandev
---
# Control Pending Message Auto-run Requirements

## Overview

The queue header presents **Run next** and bulk **Send Now** as separate ways to move pending work. **Run next** directly takes only one FIFO head, but normal turn completion then keeps taking later entries. The label describes one backend step rather than the user's real outcome: putting the queue in motion.

## Requirements

### REQ-UI-MESSAGE-QUEUE-RUN-001: Control Pending Message Auto-run

**Intent:** The queue header presents **Run next** and bulk **Send Now** as separate ways to move pending work. **Run next** directly takes only one FIFO head, but normal turn completion then keeps taking later entries. The label describes one backend step rather than the user's real outcome: putting the queue in motion.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-RUN-001.1:** The expanded queue header exposes one labeled **Auto-run** switch. It replaces header **Run next** and header bulk **Send Now**.
- **AC-UI-MESSAGE-QUEUE-RUN-001.2:** The switch has visible state-specific help:
- **AC-UI-MESSAGE-QUEUE-RUN-001.3:** ON: “Runs queued messages one at a time.”
- **AC-UI-MESSAGE-QUEUE-RUN-001.4:** OFF: “Finishes the current response, then queued messages wait.”
- **AC-UI-MESSAGE-QUEUE-RUN-001.5:** ON means the backend may start the FIFO head whenever the session is eligible and may continue with one distinct turn per entry after each prior turn completes.
- **AC-UI-MESSAGE-QUEUE-RUN-001.6:** OFF prevents every later automatic FIFO take. It does not cancel, truncate, or otherwise alter a turn that is already active or a queued handoff already accepted before OFF won the race.
- **AC-UI-MESSAGE-QUEUE-RUN-001.7:** Turning ON while the session is promptable attempts the current FIFO head immediately. When the session is busy or another lifecycle guard applies, ON is persisted and delivery resumes at the next eligible backend trigger.
- **AC-UI-MESSAGE-QUEUE-RUN-001.8:** The policy is backend-owned per session, defaults to ON, survives an empty queue, navigation, reload, and backend restart, and remains OFF until an explicit resume operation changes it.

## Migrated source detail

Decision:
[ADR-2026-08-16-server-owned-queue-auto-run](../../../decisions/2026-08-16-server-owned-queue-auto-run.md)

Implementation plan:
[Unify Queue Run Controls](../../../plans/message-queue-run-controls/plan.md)

## Why

The queue header presents **Run next** and bulk **Send Now** as separate ways
to move pending work. **Run next** directly takes only one FIFO head, but normal
turn completion then keeps taking later entries. The label describes one
backend step rather than the user's real outcome: putting the queue in motion.

Users also cannot ask the queue to stop after the current response. Explicit
Cancel parks pending work, but it cuts off the response and may complete a
workflow step. The queue needs a persistent run policy that can be changed
without cancelling the active turn.

## What

### Auto-run control

- The expanded queue header exposes one labeled **Auto-run** switch. It
  replaces header **Run next** and header bulk **Send Now**.
- The switch has visible state-specific help:
  - ON: “Runs queued messages one at a time.”
  - OFF: “Finishes the current response, then queued messages wait.”
- ON means the backend may start the FIFO head whenever the session is eligible
  and may continue with one distinct turn per entry after each prior turn
  completes.
- OFF prevents every later automatic FIFO take. It does not cancel, truncate,
  or otherwise alter a turn that is already active or a queued handoff already
  accepted before OFF won the race.
- Turning ON while the session is promptable attempts the current FIFO head
  immediately. When the session is busy or another lifecycle guard applies,
  ON is persisted and delivery resumes at the next eligible backend trigger.
- The policy is backend-owned per session, defaults to ON, survives an empty
  queue, navigation, reload, and backend restart, and remains OFF until an
  explicit resume operation changes it.
- The switch stays visible while queued work exists, including during
  clarification. It may change the future policy but never bypasses
  clarification, workflow-transition, cancellation, prompt-admission, or
  session-lifecycle safeguards.
- The switch is disabled while its mutation or another conflicting queue or
  cancellation operation is pending. Failure restores the authoritative state
  and shows a localized error.

### Targeted Send Now

- Every visible queue row, including the FIFO head, retains **Send Now**. No
  separate **Skip to next** action is introduced because Send Now already
  selects the message to skip to.
- A successful per-row Send Now is both a targeted priority override and an
  explicit resume. It turns Auto-run ON, dispatches the selected entry first,
  and then lets the preserved FIFO remainder continue as separate turns.
- Selecting a later row is a transient promotion, not a persisted reorder. For
  queued A, B, C, Send Now on B produces B, A, C while A and C keep their
  relative order.
- The first-party header no longer aggregates all queued rows into one prompt.
  Existing protocol clients may continue using `message.queue.send_now` with
  `scope: "all"`; an accepted request also turns Auto-run ON.
- The replacement-turn and restoration guarantees in
  [Send Queued Messages Now](message-queue-send-now.md) remain authoritative.

### Cancel and other queue controls

- Auto-run OFF is the non-destructive way to finish the current response and
  hold the backlog. Explicit Cancel remains the immediate-stop action and keeps
  its configured workflow-completion behavior.
- When explicit Cancel leaves pending entries, backend queue state becomes OFF
  before cancellation completes. The displayed switch therefore matches the
  parked result. Internal cancellation and Send Now's replacement cancellation
  do not turn Auto-run OFF.
- Reorder, edit, merge, Remove, Clear all, pin, and collapse retain their
  existing behavior. Clearing or draining the final row does not reset the
  saved Auto-run policy.

## API Surface

New WebSocket action:

```text
message.queue.auto_run.set
```

Request:

```json
{
  "session_id": "session-id",
  "enabled": false
}
```

Successful response:

```json
{
  "session_id": "session-id",
  "auto_run": false,
  "dispatched": false
}
```

`dispatched` is `true` only when this ON request also reserved and launched a
promptable FIFO head. A valid ON request still succeeds with `false` when the
queue is empty, the current turn is busy, clarification is pending, or another
normal lifecycle guard defers delivery. OFF always returns `false`.

The existing `message.queue.get` response and
`message.queue.status_changed` payload gain:

```json
{
  "auto_run": true
}
```

The field is always present. Missing persisted state is projected as `true`.

Existing actions remain compatible:

```text
message.queue.drain
  request:  { session_id: string }
  response: { session_id: string, drained: boolean }

message.queue.send_now
  request:  { session_id: string, scope: "entry", entry_id: string }
  response: { session_id: string, dispatched: true, sent_count: 1 }
```

A successful drain enables Auto-run before returning. An accepted Send Now
claim enables Auto-run in the same queue transaction that claims its exact
selection. Pre-claim validation, conflict, and queue-change errors leave the
previous policy unchanged.

All mutations require access to `session_id`. Authorization happens before
queue reads or state changes and uses the existing non-enumerating
session-not-found response.

## Data Model

Queue-owned state uses a dedicated table in both supported SQL dialects:

```sql
CREATE TABLE queue_session_state (
    session_id TEXT PRIMARY KEY,
    auto_run   INTEGER NOT NULL DEFAULT 1
);
```

The Postgres-compatible definition uses the repository's normal boolean
binding conventions. The memory repository stores the same value under its
existing mutex. Absence means ON, so upgrades require no backfill and existing
sessions keep today's behavior.

Policy mutations and policy-aware head reservations take the existing
per-session admission and `queue_session_locks` synchronization. The state row
is separate from that lock row because one is product policy and the other is
cross-process coordination.

Workflow session transfer moves queued entries, pending move, and Auto-run
policy in the same transaction. The destination is ON only when both source
and destination were ON; otherwise pause wins. The old session's policy row is
removed. Ordinary queue snapshot/restore changes entries and pending moves but
does not roll back an independent policy choice.

## State and Concurrency

```text
                         toggle OFF
auto-run ON  ---------------------------------->  auto-run OFF
     |                                                |
     | eligible + queued                              | current turn may finish
     v                                                | later entries wait
distinct FIFO turn --ready--> next distinct turn      |
     ^                                                |
     +---------- toggle ON or accepted Send Now <-----+
```

The backend linearizes an automatic reservation against OFF in one queue
transaction:

- If OFF commits first, the automatic reservation observes OFF and leaves the
  FIFO head pending.
- If the reservation commits first, that message may start. The acknowledged
  OFF state still prevents the following entry.

An accepted Send Now claim changes Auto-run to ON atomically with selection.
If its asynchronous prompt handoff later fails and restores the claimed rows,
Auto-run remains ON because the accepted user operation explicitly resumed the
queue. A failure before claim leaves the old policy untouched.

Auto-run governs all automatic FIFO takes, including user, agent, workflow,
server, lifecycle, and CI-automation entries. A paused automation entry remains
safely queued; the producer must not translate policy deferral into a delivery
failure. Exact targeted dispatch paths remain explicit: Send Now resumes by
contract, while unrelated direct prompts that do not consume the pending queue
are outside this policy.

## Permissions

Any user who can access the session may change its Auto-run policy or invoke a
row's Send Now action. The browser supplies only session ID, desired boolean,
and, for Send Now, persisted entry identity. It never supplies task ownership,
message content, provenance, or sender identity.

## Failure Modes

- **Policy persistence fails:** no success is reported, no optimistic state is
  retained, and the client refetches queue status.
- **ON cannot dispatch immediately:** the request still persists ON and returns
  `dispatched: false`; the next eligible backend trigger retries normally.
- **OFF races a head reservation:** the transaction ordering described above
  decides whether that head is already current. OFF always governs later rows.
- **Send Now fails before claim:** the prior Auto-run value and queue remain
  authoritative.
- **Send Now fails after accepted claim:** its existing restoration path
  restores the exact entries; Auto-run stays ON.
- **Explicit Cancel leaves a backlog:** cancellation parks it and publishes OFF
  in the same authoritative status stream.
- **A clarification or workflow guard is active:** ON remains visible but does
  not force a prompt through the guard.
- **A session transfer combines policies:** OFF wins so moved work never starts
  due only to an identity change.

## Responsive and Mobile Behavior

- Desktop and phone keep the existing inline queue panel and its single
  internal scroll owner. No drawer or mobile-only control path is introduced.
- The visible **Auto-run** label, state, and helper may wrap to a second header
  line on narrow screens. Clear all, collapse, and the desktop-only pin keep
  their existing hierarchy, and the panel creates no document-level horizontal
  overflow.
- The switch and its label form one effective coarse-pointer target at least 44
  by 44 CSS pixels. It is reachable without hover and uses the shared Switch
  primitive's keyboard, focus, checked, disabled, and screen-reader semantics.
- Every row's Send Now remains always discoverable on coarse pointers with its
  existing touch-sized target. Desktop may retain hover/focus disclosure.
- Desktop and mobile share queue state, mutation logic, errors, and WebSocket
  reconciliation. Responsive code changes only sizing and wrapping.

## Scenarios

- **GIVEN** Auto-run is ON with queued A, B, and C, **WHEN** the current turn
  completes, **THEN** A, B, and C run as three separate FIFO turns without
  another queue-control click.
- **GIVEN** a response is active with A, B, and C queued, **WHEN** the user
  turns Auto-run OFF before the next reservation wins, **THEN** the active
  response finishes and A, B, and C remain pending.
- **GIVEN** Auto-run is OFF and the session is promptable, **WHEN** the user
  turns it ON, **THEN** A starts immediately and later entries continue one at
  a time.
- **GIVEN** Auto-run is OFF with A, B, and C queued behind an active response,
  **WHEN** the user selects Send Now on B, **THEN** Auto-run turns ON, B replaces
  the active turn without ordinary Cancel side effects, then A and C run as
  distinct turns in that relative order.
- **GIVEN** Auto-run is OFF, **WHEN** the queue becomes empty and the browser
  reloads or Kandev restarts, **THEN** the next queued entry remains held and
  the panel reports OFF until the user resumes it.
- **GIVEN** explicit Cancel leaves pending entries, **WHEN** cancellation
  settles, **THEN** the entries remain pending and Auto-run reports OFF.
- **GIVEN** clarification is pending with queued work, **WHEN** the user turns
  Auto-run ON, **THEN** the switch reports ON but no queued prompt starts until
  clarification is resolved and the session becomes eligible.
- **GIVEN** paused queued work transfers to another workflow session, **WHEN**
  the transfer commits, **THEN** the destination remains OFF and no transferred
  entry starts solely because its session ID changed.
- **GIVEN** a phone viewport with queued work, **WHEN** the user taps Auto-run
  OFF and reloads, **THEN** the switch remains OFF, the backlog is held, all
  controls are touch-sized, and the page has no horizontal overflow.
- **GIVEN** an existing protocol client sends Send Now with `scope: "all"`,
  **WHEN** the backend accepts the request, **THEN** its aggregate replacement
  behavior remains compatible and Auto-run becomes ON for later arrivals.

## Out of Scope

- Scheduling a future run time, rate limiting, or pausing after an arbitrary
  number of additional entries.
- A global, workspace, or task-wide Auto-run preference. Policy is per session.
- Stopping or rolling back a turn or queue handoff already accepted before OFF
  won the reservation race.
- Combining queued rows into one prompt through the first-party queue UI.
- Removing the backward-compatible Send Now all-entry or drain protocols.
- Changing queue capacity, merge, reorder, edit, Remove, Clear all, pin, or
  collapse semantics.
