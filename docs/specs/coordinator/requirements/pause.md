---
id: coordinator-pause
title: Pausing a coordinator
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
---

# Pausing a coordinator Requirements

## Overview

Today the only way to stop an unattended coordinator is to turn autonomy off,
which discards its queue of pending wakes and its spend interlock. Pause is a
separate brake: it stops the coordinator from acting on its own, stops a
running unattended turn and a running dream, and keeps every pending wake so
that Resume carries on where it stopped. It does not touch the coordinator's
settings, and a manager's own chat with the coordinator keeps working.

## Terminology

- **Paused:** a coordinator state a manager sets and clears, stored with the
  time and the manager. It is independent of autonomy.
- **Act on its own:** start an unattended turn, start a dream, or approve a
  proposal automatically.
- Other terms are defined in [wake](wake.md#terminology).

## Requirements

### REQ-COORDINATOR-PAUSE-001: Pause and Resume

**Intent:** A manager can stop the coordinator acting on its own at once, and
start it again without losing its queue.

#### Acceptance criteria

- **AC-COORDINATOR-PAUSE-001.1:** While the phase 3.1 flag is effective, a
  manager shall be able to pause and resume one coordinator. Pausing an already
  paused coordinator, or resuming one that is not paused, shall change nothing
  and shall succeed. A reader and a coordinator principal on any transport
  shall be refused with 403 and change nothing. A request for a coordinator
  that does not exist or is not visible to the caller shall be refused with 404,
  and a request whose body has no boolean `paused` shall be refused with 400;
  each changes nothing.
- **AC-COORDINATOR-PAUSE-001.2:** The paused state shall be stored with the time
  and the manager who set it, survive a restart, be allowed while autonomy is
  off, and be deleted with the coordinator. When two managers pause and
  resume at once, each accepted request shall apply in commit order and the
  last committed request shall decide the state.
- **AC-COORDINATOR-PAUSE-001.3:** When a coordinator is resumed, the system
  shall deliver its pending wakes through the ordinary admission
  ([wake](wake.md#req-coordinator-wake-004-autonomy-setting-and-admission)),
  the five-minute cooldown included. Pause and Resume shall discard none: no
  wake is superseded while the coordinator is paused, and after Resume each
  wake meets the ordinary episode re-check of delivery, so a wake whose episode
  no longer holds is superseded then, for the reason it always was.

### REQ-COORDINATOR-PAUSE-002: What Pause stops

**Intent:** Paused means nothing happens without a person. A turn or dream that a pause finds running stops within one minute, and no new one starts after the pause commits.

#### Acceptance criteria

- **AC-COORDINATOR-PAUSE-002.1:** While a coordinator is paused, the system
  shall start no unattended turn and no dream for it, and shall leave every
  pending wake `pending`: the wake recorder and the backstop shall keep
  recording wakes and shall supersede none because of Pause.
- **AC-COORDINATOR-PAUSE-002.2:** When a coordinator is paused while an
  unattended turn is running, the system shall stop that turn as a ceiling stop
  does, settle it with the outcome `stopped_by_pause`, and return its wakes to
  `pending`. When it is paused while a dream is running, the system shall cancel
  the episode and set its review-result row `failed` with the reason
  `paused`.
- **AC-COORDINATOR-PAUSE-002.3:** While a coordinator is paused, a proposal
  that the automatic class would approve shall stay `pending` for a manager,
  with the note "Paused; a manager will decide", and shall count against the ten-per-day limit of the automatic class in no way. Resume shall not approve it automatically; automatic approval is tried only when a proposal is created.
- **AC-COORDINATOR-PAUSE-002.4:** A manager's message to a paused coordinator
  shall start a turn as it does when the coordinator is not paused, and the
  coordinator's proposals from it shall be created as usual.
- **AC-COORDINATOR-PAUSE-002.5:** When the paused state cannot be read, the
  system shall treat the coordinator as paused for every act-on-its-own
  decision, and log it. While the phase 3.1 flag is not effective this applies
  only to a coordinator the process knows to be paused; for any other
  coordinator an unreadable state is treated as not paused, so a flag-off boot
  behaves as phase 3 does.
- **AC-COORDINATOR-PAUSE-002.7:** When the phase 3.1 flag stops being effective while a coordinator is paused, the system shall keep it paused for every act-on-its-own decision and keep the stored state; only the route and the controls shall go. The autonomy strip and the Autonomy
  settings section shall then show a read-only "Paused" badge with the note
  "Resume needs the phase 3.1 features to be on", and no control.
- **AC-COORDINATOR-PAUSE-002.6:** Pause shall not change autonomy, the ceiling,
  the policy, Watches, the conversation or any pending wake, proposal or
  stored spend.

### REQ-COORDINATOR-PAUSE-003: Seeing and using Pause

**Intent:** The state is visible where autonomy is, and one action changes it.

#### Acceptance criteria

- **AC-COORDINATOR-PAUSE-003.1:** While the phase 3.1 flag is effective, the
  autonomy strip on Needs you shall show "Paused" with the manager's name
  and the time in place of Active or Held, keep the pending count, and offer a
  manager **Resume**; when the coordinator is not paused and autonomy is on it
  shall offer a manager **Pause**. A reader shall see the state and no
  control. The strip is shown only while autonomy is on, as today, in one of
  four states: Active, Active with a transient text, Held with a reason, or
  Paused, which takes precedence over the other three; with autonomy off the
  state and the control appear in the Autonomy settings section only.
- **AC-COORDINATOR-PAUSE-003.2:** The coordinator's Autonomy settings section
  shall show the state and the same control, also while autonomy is off, with
  the note that a paused coordinator keeps its queue and that turning autonomy
  off does not.
- **AC-COORDINATOR-PAUSE-003.3:** Pause and Resume shall take effect without
  a save bar and shall update every open view through `coordinator.updated`.
  While the request is in flight the control shall be disabled; when it
  fails the control shall show the failure and the previous state.
- **AC-COORDINATOR-PAUSE-003.4:** On a phone the control shall be a full-width
  button with a touch target of at least 44 pixels at the end of the autonomy
  strip's second line.

## Out of scope

- Pausing a routine (phase 4); the state is the one Pause routines will read.
- Scheduled or time-limited pause.
- Pausing every coordinator of a workspace at once.
- Stopping a manager's own turn, or hiding the coordinator.
- A log row in What it did for Pause and Resume.
