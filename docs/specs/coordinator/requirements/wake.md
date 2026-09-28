---
id: coordinator-wake
title: Wake on its tasks' events
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Wake on its tasks' events Requirements

## Overview

In phases 1 and 2 a coordinator acts only when a manager sends it a message.
Phase 3 lets a manager turn on **autonomy** for one coordinator: the
coordinator is then woken when one of its own tasks needs attention, once per
episode, and runs an **unattended turn** in its existing conversation. A
level-triggered backstop recovers any wake an event path dropped. A wake never
creates, replaces or repoints the conversation: when the conversation is busy
the wake waits, and when it is unavailable the wake is held and shown.

Unattended turns start only when [containment](containment.md) is in place and
spend is under the [cost ceiling](spend.md). This document owns what wakes a
coordinator and how a wake becomes a turn; those two documents own the
conditions that hold it.

## Terminology

- **Autonomy:** a per-coordinator setting, off by default, that allows
  unattended turns. Only a manager changes it, through the coordinator
  settings routes.
- **Own task:** a task created by approving one of the coordinator's
  proposals (the proposal's `task_id`), while that task exists, is not
  archived and is not ephemeral. Tasks the coordinator only reads are not its
  own tasks.
- **Episode:** one occurrence of a condition on an own task, identified by the
  tuple (coordinator, task, kind, episode key). The kinds and keys are:

  | Kind | Condition | Episode key |
  | --- | --- | --- |
  | `question` | the task's primary session has a pending clarification bundle | the bundle's pending id |
  | `permission` | the task's primary session has a pending permission request | the request's pending id |
  | `stall` | the coordinator holds a current stall record for the task | the record's `last_event_at` |
  | `error` | the task's primary session has an active error | the error's stamp |
  | `completed` | the task's state is `COMPLETED` | the literal `completed` |

- **Current stall record:** a stall record whose detection time is not
  earlier than the task's last activity, the same currency test Needs you
  applies in `AC-COORDINATOR-NEEDS-YOU-001.2`. A stall record left behind
  after the task resumed is not current, so it is no condition.
- **Wake:** the stored record of one episode for one coordinator. A wake is
  `pending`, `delivered` (it was included in an unattended turn) or
  `superseded` (its condition ended before delivery).
- **Unattended turn:** a turn of the coordinator's conversation started by
  wake delivery rather than by a person. It starts when delivery commits it
  and ends when the session turn its message started ends; a later turn of
  the same session, such as a manager's queued message, is never part of it.
- **Backstop:** a periodic pass that re-derives every own task's current
  episodes from stored state, so a dropped event delays a wake but never loses
  it.
- **Admission:** the ordered checks that decide whether pending wakes may be
  delivered now. The first failing check is the **hold reason**.
- Other terms are defined in the [system README](../README.md#terms).

## Mockup

The phase 3 view of the source mockup (`mockup-v2.1`, outside this repository)
predates the current phase numbering and shows none of the wake states; the
plan's ASCII previews UI-01 and UI-02 are the reference for this document's
user-facing criteria.

## Requirements

### REQ-COORDINATOR-WAKE-001: One wake per episode

**Intent:** A coordinator with autonomy on learns about each episode on its
own tasks exactly once.

#### Acceptance criteria

- **AC-COORDINATOR-WAKE-001.1:** When an event reports a condition in the
  episode table on an own task of a coordinator with autonomy on, the system
  shall store one `pending` wake for that episode.
- **AC-COORDINATOR-WAKE-001.2:** When the same episode is reported again, by a
  redelivered event, a backend restart, the backstop or any combination, the
  system shall not store a second wake and shall not change the existing
  wake's status.
- **AC-COORDINATOR-WAKE-001.3:** A condition on a task that is not an own task
  of the coordinator, on the coordinator's own conversation task, or on any
  task while the coordinator's autonomy is off, shall store no wake.
- **AC-COORDINATOR-WAKE-001.4:** When a coordinator already holds 200 `pending`
  wakes, the system shall store no further wake for it, count the refusal in
  metrics, and store the episode later through the backstop once the
  coordinator holds fewer than 200. Concurrent recording shall not take the
  count past 200. Wakes returned to `pending` by
  `AC-COORDINATOR-WAKE-005.3` are not newly stored and may take it past 200
  until delivery reduces it.

### REQ-COORDINATOR-WAKE-002: Level-triggered backstop

**Intent:** No event path is trusted to arrive; stored state is the source of
truth.

#### Acceptance criteria

- **AC-COORDINATOR-WAKE-002.1:** Every 60 seconds, for every coordinator with
  autonomy on, the system shall re-derive every current episode of its own
  tasks from stored task, session and stall state, and store a wake for each
  episode that has none.
- **AC-COORDINATOR-WAKE-002.2:** Given an own task whose condition arose while
  every event for it was dropped, the system shall store its wake within one
  backstop period of the condition becoming readable from stored state.
- **AC-COORDINATOR-WAKE-002.3:** When a backstop pass fails to read one
  coordinator's own tasks, the system shall log the failure, skip that
  coordinator for the pass, store nothing for it and continue with the next.
- **AC-COORDINATOR-WAKE-002.4:** When autonomy is turned on, the next backstop
  pass shall store wakes for the episodes that already exist on the
  coordinator's own tasks.

### REQ-COORDINATOR-WAKE-003: Busy is not unavailable

**Intent:** A wake never forks the coordinator into a second conversation.

#### Acceptance criteria

- **AC-COORDINATOR-WAKE-003.1:** No wake, backstop pass or delivery shall
  create a conversation task, archive one, change `conversation_task_id`, or
  start a session other than the current conversation's primary session.
- **AC-COORDINATOR-WAKE-003.2:** While the conversation's primary session is
  starting, running a turn, has a pending action or has a queued message, the
  system shall leave pending wakes `pending` with hold reason
  `conversation_busy`, and shall attempt delivery again when that session
  becomes idle or at the next backstop pass, whichever is first.
- **AC-COORDINATOR-WAKE-003.3:** When the coordinator has no current
  conversation task, or its primary session is failed, cancelled, completed or
  absent, the system shall leave pending wakes `pending` with hold reason
  `no_conversation` or `conversation_unavailable`, and shall deliver them only
  after a manager has opened a usable conversation.

### REQ-COORDINATOR-WAKE-004: Autonomy setting and admission

**Intent:** A manager decides whether a coordinator may act unattended, and
the system says why it is not acting.

Mockup:

- Plan UI-04: the Autonomy section of the coordinator settings.

#### Acceptance criteria

- **AC-COORDINATOR-WAKE-004.1:** A manager shall be able to turn autonomy on
  or off for one coordinator. Turning it on shall be refused with 400 naming
  `cost_ceiling` when the coordinator has no declared ceiling. A reader, and
  a coordinator principal on any transport, shall be refused and change
  nothing.
- **AC-COORDINATOR-WAKE-004.2:** Before each delivery the system shall run
  admission in this order and stop at the first failure, whose reason is the
  hold reason: autonomy on (`autonomy_off`); containment verified
  (`containment`, with the failing condition); spend measurable
  (`spend_unmeasured`); spend below the ceiling (`ceiling_reached`); a current
  conversation (`no_conversation`); its primary session usable
  (`conversation_unavailable`); that session idle (`conversation_busy`); and
  at least five minutes since the previous unattended turn ended
  (`cooldown`).
- **AC-COORDINATOR-WAKE-004.3:** When autonomy is turned off, the system shall
  mark every `pending` wake of that coordinator `superseded` and start no
  unattended turn for it, including a delivery that passed admission before
  the change but had not yet started its turn; a running unattended turn
  shall finish normally, and until it ends the ceiling stop, the recovery of
  its message and its settle shall keep applying to it as they do while
  autonomy is on.
- **AC-COORDINATOR-WAKE-004.4:** While `features.coordinatorPhase3` or
  `features.coordinator` is off, the system shall store no wake, run no
  backstop, start no unattended turn, and hide the Autonomy section; stored
  settings and wakes shall be kept.

### REQ-COORDINATOR-WAKE-005: The unattended turn

**Intent:** Pending wakes become one bounded turn that says what happened and
that no person started it.

#### Acceptance criteria

- **AC-COORDINATOR-WAKE-005.1:** When admission passes, the system shall mark
  every pending wake whose condition still holds as delivered, oldest first by
  creation time then id, at most 20 per turn, mark every pending wake whose
  condition has ended `superseded` (all of them, not only those before the
  twentieth delivered one), and start one unattended turn whose
  message lists the delivered wakes. When no pending wake still holds, it
  shall start no turn. A condition still holds only when stored state shows
  the same episode the wake recorded (the same pending question or
  permission, the same error, the same stall, or the task still completed),
  judged once when delivery reads the pending wakes; a condition that ends
  after that read is still delivered, and the message tells the agent to read
  current state.
- **AC-COORDINATOR-WAKE-005.2:** A coordinator shall never have more than one
  unattended turn open, under concurrent delivery attempts from events, the
  backstop and settings changes.
- **AC-COORDINATOR-WAKE-005.3:** When the turn's message cannot be sent, or the
  process stops after wakes were marked delivered and before the turn started,
  the system shall return those wakes to `pending` and record the turn as
  failed or interrupted; no wake shall be lost or delivered twice. When the
  conversation became busy after admission, the send shall be refused rather
  than queued behind the other turn, and the wakes shall wait for the next
  delivery. A send
  whose outcome is unknown (for example a timeout) shall count as sent when
  the turn's message is stored, and shall not be sent again.
- **AC-COORDINATOR-WAKE-005.4:** An unattended turn shall use the same Kandev
  tool surface and permission policy as an attended turn, except as
  [containment](containment.md) narrows it.
- **AC-COORDINATOR-WAKE-005.5:** In the transcript, an unattended turn's
  message shall render as "Woken by" followed by the number of events, with an
  expandable list naming each event's kind and task, visibly distinct from a
  manager's message.
- **AC-COORDINATOR-WAKE-005.6:** Given an own task that stalls and then shows
  new activity before its stall wake is delivered, the system shall mark that
  wake `superseded` and include it in no unattended turn.

### REQ-COORDINATOR-WAKE-006: Seeing what autonomy is doing

**Intent:** A manager can tell at a glance whether the coordinator is acting
unattended and, if not, why.

Mockup:

- Plan UI-01: the autonomy strip on Needs you.
- Plan UI-02: the held item.

#### Acceptance criteria

- **AC-COORDINATOR-WAKE-006.1:** While autonomy is on, Needs you shall show a
  strip with the autonomy state (Active or Held with its reason), the time of
  the last unattended turn, the number of pending wakes, and the
  coordinator's spend against its ceiling.
- **AC-COORDINATOR-WAKE-006.2:** While the hold reason is `containment`,
  `spend_unmeasured`, `ceiling_reached`, `no_conversation` or
  `conversation_unavailable` and at least one wake is pending, Needs you shall
  show one item of kind `autonomy` naming the reason and what fixes it, with
  reference time equal to the oldest pending wake's creation time. The
  transient reasons `conversation_busy` and `cooldown` shall not produce an
  item.
- **AC-COORDINATOR-WAKE-006.3:** The `autonomy` item shall count toward the
  coordinator's attention count and not toward the Inbox count.
- **AC-COORDINATOR-WAKE-006.4:** When the autonomy state cannot be read, the
  strip shall say "Autonomy state unavailable" with Try again, and Needs you
  shall show its other items.

## Out of scope

- Waking on tasks the coordinator does not own, on phase 2's Watches, on a
  schedule, or on a heartbeat. A later change may add Watches as a wake source.
- Waking a coordinator whose autonomy is off, and any wake that starts a task's
  agent. A wake starts only the coordinator's own conversation turn.
- Re-waking for the same episode, including a task completed a second time
  after being reopened.
- Automation runs and the automation replacement fork; this design does not use
  the automation runner.
- Recovering a conversation that failed; a manager reopens it (copilot).
