---
id: coordinator-goals
title: Goals and baselines
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Goals and baselines Requirements

## Overview

A manager gives a coordinator a goal, a milestone with a name, an optional
due date and exit criteria, so it can rank work against something. Needs you
shows a goal note. With no goal set, the note says so and offers Set a goal.
Baselines compare a few measures now with the moment the goal was set, and
say "No baseline" when there is nothing honest to compare.

## Terminology

- **Active goal:** a coordinator's goal whose status is `active`. A
  coordinator has at most one.
- **Met goal:** a goal a manager marked met; it is kept but no longer shown as
  the goal.
- **Measure:** one of Open tasks, Approved proposals (7 days) and Rejected
  proposals (7 days).
- **Baseline:** the value of each measure recorded when a goal becomes active.
- Other terms are defined in the [system README](../README.md#terms) and
  [permissions](permissions.md#terminology).

## Mockup

The goal section and goal note have no phase 2 screenshot. The ASCII previews
in the [plan](../../../plans/workspace-coordinator-p2/plan.md#ascii-ui-previews)
are the visual reference.

## Requirements

### REQ-COORDINATOR-GOALS-001: Setting a goal

**Intent:** The coordinator knows what the manager is trying to reach.

**User story:** As a workspace manager, I want to set a milestone with exit
criteria, so that the coordinator's advice points at it.

#### Acceptance criteria

- **AC-COORDINATOR-GOALS-001.1:** When a manager sets a goal with a name of 1
  to 120 characters after trimming, an optional due date and 0 to 10 exit
  criteria of 1 to 200 characters each after trimming, the system shall
  store it as the coordinator's active goal and return it. Other values
  shall be refused with 400 naming the field; the due date may be any
  calendar date.
- **AC-COORDINATOR-GOALS-001.2:** When the coordinator already has an active
  goal, setting a goal shall update that goal's name, due date and criteria,
  keep its baseline, and keep each criterion's done state when the criterion
  keeps its id. Where the request names a goal that is not the active goal,
  `AC-COORDINATOR-GOALS-001.10` applies first. A criterion id that is not one of the active goal's
  criteria, or that is repeated, shall be refused with 400 naming the field;
  a criterion left out shall be removed.
- **AC-COORDINATOR-GOALS-001.3:** When a manager checks or unchecks an exit
  criterion, the system shall store its done state and shall not mark the
  goal met. Concurrent changes to one goal shall apply in commit order and
  none shall be lost.
- **AC-COORDINATOR-GOALS-001.4:** When a manager marks the active goal met, the
  system shall set its status to `met` with `met_at` and `met_by`; the
  coordinator shall then have no active goal. Marking met acts only on the
  active goal: when there is none, the request shall return the most recent
  met goal unchanged with 200 and change nothing, and when the coordinator
  has never had a met goal it shall return 404 and change nothing. Where the
  request names the goal it was made from and that goal is not the active
  goal, `AC-COORDINATOR-GOALS-001.10` applies.
- **AC-COORDINATOR-GOALS-001.5:** When a manager sets a goal while the
  coordinator has no active goal, the system shall create a new goal with a
  new baseline (`AC-COORDINATOR-GOALS-003.1`); earlier met goals stay stored.
- **AC-COORDINATOR-GOALS-001.6:** When a coordinator conversation opens, its
  standing instructions shall include the active goal's name, due date and
  criteria with their done state as of that moment; with no active goal they
  shall say that no goal is set. Where the goal cannot be read, the standing
  instructions shall omit the goal section, log a warning and otherwise be
  unchanged, and the conversation shall proceed.
- **AC-COORDINATOR-GOALS-001.7:** When a manager changes the active goal's
  name, due date or criteria, or marks it met, or sets a new goal, the system
  shall archive the current conversation task and clear its reference.
  Checking or unchecking a criterion shall keep the conversation.
- **AC-COORDINATOR-GOALS-001.8:** A reader's goal write request (set, check or
  uncheck, mark met) shall be refused with 403 and change nothing; a reader's
  goal read shall succeed, and the Goal section shall show a reader the goal
  without controls.
- **AC-COORDINATOR-GOALS-001.9:** The Goal section shall show the milestone
  name, the due date, the exit criteria with checkboxes, **Mark milestone
  met** and the baseline measures; with no active goal it shall show an empty
  form with **Set goal**.

- **AC-COORDINATOR-GOALS-001.10:** Where a set-goal or mark-met request names
  the goal it was made from, and that goal is not the coordinator's active
  goal, the system shall refuse it with 409 and change nothing; the one
  exception is a mark-met request that names the coordinator's most recent
  met goal while there is no active goal, which returns that goal with 200
  and changes nothing. A set-goal request that
  names no goal is applied as in `AC-COORDINATOR-GOALS-001.2` and
  `AC-COORDINATOR-GOALS-001.5`; a mark-met request that names none, as in
  `AC-COORDINATOR-GOALS-001.4`.

### REQ-COORDINATOR-GOALS-002: Goal note on Needs you

**Intent:** Needs you says what the list is ranked against.

#### Acceptance criteria

- **AC-COORDINATOR-GOALS-002.1:** While the phase-2 flag is on and the
  coordinator has no active goal, Needs you shall show, above the list, a
  note that no goal is set and so the list is ordered by urgency alone, with
  **Set a goal** for a manager, which opens the coordinator's Goal section. A
  reader shall see the note without the button.
- **AC-COORDINATOR-GOALS-002.2:** While the coordinator has an active goal, the
  note shall show the goal's name, its due date (marked overdue when the date
  is before today in the viewer's time zone) and "N of M criteria met".
- **AC-COORDINATOR-GOALS-002.3:** When the coordinator's most recent goal is
  met and it has no active goal, the note shall say the milestone was met on
  its date and offer **Set the next goal** to a manager.

### REQ-COORDINATOR-GOALS-003: Baselines

**Intent:** A manager sees whether things moved since the goal was set,
without a false trend.

#### Acceptance criteria

- **AC-COORDINATOR-GOALS-003.1:** When a goal becomes active, the system shall
  record its baseline in the same transaction: Open tasks (watched tasks that
  are not archived, not completed, not ephemeral, not created by a
  coordinator and not created by an automation run, which excludes
  coordinator conversation tasks and hidden automation-run tasks), and the coordinator's Approved and Rejected proposals of the 7 days
  before, from the activity log.
- **AC-COORDINATOR-GOALS-003.2:** When the coordinator was created less than 7
  days before the goal became active, the baseline of the two proposal
  measures shall be recorded as none, and they shall show "No baseline".
- **AC-COORDINATOR-GOALS-003.3:** When the active goal is read, the system
  shall return each measure's current value, computed at read time over the
  same definition, next to its baseline; with no active goal it shall return
  no measures.
- **AC-COORDINATOR-GOALS-003.4:** Each measure shall show a direction (up or
  down) only when its current value differs from its baseline by 2 or more;
  otherwise it shall show "No direction yet". A measure without a baseline
  shows "No baseline" and no direction.

## Out of scope

- A trend line or daily history of measures.
- Goals shared across coordinators or workspaces.
- The coordinator setting or marking its own goal.
- Ranking Needs you by the goal; phase 2 orders Needs you as phase 1 does.
