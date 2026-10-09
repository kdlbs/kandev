---
status: draft
system: office
created: 2026-09-17
owners:
  - kandev
---

# Office Routine Arming Visibility Requirements

## Overview

An Office routine fires on its own only when a cron trigger reaches the
scheduler. Several independent states stop that happening, and none is reported
anywhere: the operator sees a routine badged "On" and cannot tell a running loop
from one somebody deliberately turned off, or from one silently dead.

This capability makes the distinction **visible** and repairs nothing.
Operator-initiated repair was specified alongside it and deferred to a separate
card; see `## Out of scope`. Until that lands, an operator shown a broken
schedule still repairs it by hand.

The live install at spec time has zero armed cron triggers anywhere: one routine
is `paused` with a disabled `cron` trigger and renders "Off", and one is `active`
with only a `manual` trigger, so it renders **"On"**, has no schedule at all, and
does not fire. The second is the sharper failure: switched on, presenting as on,
incapable of firing.

Every term in this contract is an Office primitive: `office_routines.status`,
`office_routine_triggers`, and the dispatch tick in `internal/office/routines`.

This document owns the classification and the read path. The startup scan that
reports the same classification to logs and counters on an unattended install is
`REQ-OFFICE-ROUTINE-ARMING-003`, which lives in
[Office Routine Arming Startup Scan](routine-arming-startup-scan.md) to stay
inside the requirement-file size limit. The two ship together.

## Prior art

No external knowledge base was reachable, so the prior art below is in-repo.

[Office Stall Visibility](stall-visibility.md) is the same shape: a state no
detector reports, made visible without granting anything permission to recover
it. This document reuses its `Surface` definition, its detection/recovery split,
and its structured-log-plus-`expvar` convention, and differs in one way: stall
recovery is forbidden on principle, whereas here recovery is legitimate and
merely deferred, which is why `## Out of scope` names a successor card rather
than closing the question.

## Terminology

- **Fire:** the scheduler tick claims a trigger and dispatches a routine run. A
  routine that fires and finds no work has fired.
- **Intent:** the operator's on/off choice, held in `office_routines.status`.
  `active` means run this routine; any other value means deliberately off.
- **Schedule state:** whether the routine's triggers can fire it on a schedule,
  computed from the trigger rows alone, without reference to intent.
- **Armed:** schedule state `armed` — some cron trigger can fire the routine on
  a schedule with no further operator action.
- **Schedulable:** a cron trigger for which the shared scheduler can compute a
  next occurrence. The expression and timezone must be valid, and the
  expression must name at least one real calendar slot. The scheduler's
  day-of-month/day-of-week OR rule and DST handling apply here too. A trigger
  that fails these checks is not schedulable, and no amount of re-arming makes
  it fire.
- **Dispatch grace:** a named duration of 60 seconds, separating a trigger
  mid-dispatch from one permanently stuck. See
  AC-OFFICE-ROUTINE-ARMING-001.10.
- **Trigger order:** `created_at` ascending, ties broken by `id` ascending. Every
  ordered output over triggers in this document uses it.
- **Unarmed cron trigger:** a cron trigger of the routine that cannot currently
  fire, carrying the reasons why. See AC-OFFICE-ROUTINE-ARMING-001.8.
- **Surface:** report a state to an operator without changing routine, trigger,
  or run state.

Intent and schedule state are two switches, and this document owns the
classification and reporting of the second one. Classification reads trigger
rows only. Dispatch uses the routine status separately: the cron path checks
the status before it claims a slot, and the manual and webhook paths reject a
non-firing status. This document reports both values and does not change those
dispatch gates.

## Requirements

### REQ-OFFICE-ROUTINE-ARMING-001: Classify a routine's schedule state

**Intent:** Every unarmed routine is unarmed for exactly one reason, and the
reason determines what the operator must do next. A single boolean cannot carry
that, and a state computed from intent cannot separate deliberate from broken.

**User story:** As an operator, I want each routine to report why it will or will
not fire, so I can tell a paused loop from a dead one.

Schedule state is a total function of the routine's trigger rows alone. The first
matching rule wins; the ordering is the contract, not an implementation detail.

| # | Condition on the routine's triggers | State |
|---|---|---|
| 0 | The trigger rows could not be read | `unknown` |
| 1 | Some `cron` trigger has `enabled = true` and `next_run_at` non-null | `armed` |
| 2 | Some `cron` trigger has `enabled = true`, `next_run_at` null, and `last_fired_at` no older than the dispatch grace | `armed` |
| 3 | Some `cron` trigger has `enabled = true` and is not schedulable | `trigger_invalid` |
| 4 | Some `cron` trigger has `enabled = true` | `trigger_unscheduled` |
| 5 | At least one `cron` trigger exists | `trigger_disabled` |
| 6 | Some `webhook` trigger has `enabled = true`, and no `cron` trigger exists | `event_only` |
| 7 | At least one trigger exists, none of kind `cron` | `unscheduled_manual_only` |
| 8 | No trigger rows exist | `unscheduled_no_trigger` |

Rule 2 exists because the scheduler clears `next_run_at` when it claims a trigger
and restores it a few statements later; without it a healthy routine that happens
to be mid-dispatch classifies as broken. Rules 3 and 4 therefore describe an
enabled cron trigger whose `next_run_at` has been null for longer than that,
which is a permanent stall. Rule 3 precedes rule 4 so the more actionable
diagnosis wins when a routine owns an unschedulable enabled cron trigger
alongside a merely unscheduled one. Rule 5 is reached only when no cron trigger is
enabled, because rules 1 to 4 between them match every routine that has one.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-ARMING-001.1:** The system shall assign every routine
  exactly one schedule state from the table above, applying the rules in the
  order given and stopping at the first match.
- **AC-OFFICE-ROUTINE-ARMING-001.2:** The system shall compute schedule state
  without reading `office_routines.status`, so that a routine's schedule state is
  identical whether its intent is `active` or not.
- **AC-OFFICE-ROUTINE-ARMING-001.3:** A cron trigger with `enabled = true` whose
  `next_run_at` is in the past shall match rule 1. A past-due trigger is due, not
  broken. This says how rule 1 reads a past instant and does not override the
  table: the same trigger with `enabled = false` reaches rule 5.
- **AC-OFFICE-ROUTINE-ARMING-001.4:** A cron trigger with `enabled = true` whose
  `next_run_at` is exactly the classification instant of
  AC-OFFICE-ROUTINE-ARMING-001.12 shall match rule 1, on the same terms.
- **AC-OFFICE-ROUTINE-ARMING-001.5:** When reading the routine's trigger rows
  fails, the system shall report `unknown` and shall not report `armed`. A row
  the system cannot decode is a row it could not read.
- **AC-OFFICE-ROUTINE-ARMING-001.6:** The system shall produce the same schedule
  state for the same trigger rows on SQLite and on PostgreSQL.
- **AC-OFFICE-ROUTINE-ARMING-001.7:** Classification shall write no routine,
  trigger, or run state.
- **AC-OFFICE-ROUTINE-ARMING-001.8:** Alongside the schedule state, the system
  shall report the routine's unarmed cron triggers: every cron trigger that is
  `enabled = false`, or is not schedulable, or is `enabled = true` and
  schedulable with a null `next_run_at` and a `last_fired_at` either null or
  older than the dispatch grace. Each entry shall carry the trigger's `id` and
  **every** one of those three reasons that applies to it, ordered as the reasons
  are listed in this criterion; a single entry may carry more than one. The list
  shall be in trigger order.
- **AC-OFFICE-ROUTINE-ARMING-001.9:** A routine may be `armed` and still report a
  non-empty unarmed cron trigger list: rule 1 matches on one trigger while
  another remains broken.
- **AC-OFFICE-ROUTINE-ARMING-001.10:** The dispatch grace shall be a single named
  compile-time constant of 60 seconds, used identically by classification and by
  the startup scan. It shall not be operator-configurable: no environment
  variable, configuration key, or runtime flag shall expose it.
- **AC-OFFICE-ROUTINE-ARMING-001.11:** A cron trigger with `enabled = true`, a
  null `next_run_at`, and a null `last_fired_at` shall not match rule 2. A
  trigger that has never fired is not mid-dispatch. Schedule state is the
  routine's and is decided by first match across all its triggers, so such a
  trigger sets the routine's state under rule 3 or rule 4 only when no sibling
  matched an earlier rule; a sibling matching rule 1 leaves the routine `armed`
  and puts this one on the unarmed cron trigger list, per
  AC-OFFICE-ROUTINE-ARMING-001.9.
- **AC-OFFICE-ROUTINE-ARMING-001.12:** One classification of one routine shall
  read that routine's trigger rows once and derive both the schedule state and
  the unarmed cron trigger list from that single read, and shall compare every
  `last_fired_at` against one instant captured once for that classification, so
  that no two triggers of a routine are judged against different clocks. The
  bounded read is the one the classification derives its answer from: when a batch
  read fails and the per-routine re-read of AC-OFFICE-ROUTINE-ARMING-001.13
  supplies the rows, that re-read is the single read, and the instant stays the one
  captured for the classification the re-read belongs to rather than being
  re-captured per routine. A failed read derives nothing and does not count against
  this criterion.
- **AC-OFFICE-ROUTINE-ARMING-001.13:** A trigger-read failure shall make
  `unknown` exactly those routines whose own trigger rows were not read. When one
  query reads the trigger rows of several routines and that query fails, or one
  row in its result cannot be decoded, the system shall re-read each routine of
  that batch individually and report `unknown` only for those whose individual
  read also fails, so one undecodable row makes at most its own routine
  `unknown`, never every routine in the workspace. That re-read shall be
  attempted at most once per routine per classification.

### REQ-OFFICE-ROUTINE-ARMING-002: Report intent and schedule state on routine reads

**Intent:** The operator's only view of a routine today is a badge driven by
intent alone, which is why a routine with no schedule renders as "On". Both
values must arrive in one response, so the surface cannot show one without the
other.

**User story:** As an operator opening the routines list, I want to see both
whether a routine is switched on and whether it can actually fire.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-ARMING-002.1:** The routine list response and the single
  routine response shall each carry, per routine, the intent value, the schedule
  state from REQ-OFFICE-ROUTINE-ARMING-001, and the unarmed cron trigger list
  from AC-OFFICE-ROUTINE-ARMING-001.8.
- **AC-OFFICE-ROUTINE-ARMING-002.2:** The routines list shall distinguish a
  routine whose intent is `active` and whose schedule state is `armed` from one
  whose intent is `active` and whose schedule state is anything else.
- **AC-OFFICE-ROUTINE-ARMING-002.3:** The routines list shall distinguish a
  routine whose intent is not `active` from a routine whose intent is `active`
  but which cannot fire, so that deliberate and broken never render the same.
- **AC-OFFICE-ROUTINE-ARMING-002.4:** The surface shall distinguish a routine
  whose unarmed cron trigger list holds at least one schedulable entry from one
  whose unarmed entries are none of them schedulable, whatever its schedule
  state: the first needs only its triggers re-armed, the second needs its cron
  expression or timezone edited. A routine `armed` on one trigger and broken on
  another still carries the distinction. The distinction applies only where the
  unarmed cron trigger list is non-empty: over an empty list "none of them
  schedulable" holds vacuously, so a routine with an empty list shall render
  neither side of it rather than be told to edit an expression it has no problem
  with. Neither rendering shall offer an action that performs the repair;
  see `## Out of scope`.
- **AC-OFFICE-ROUTINE-ARMING-002.5:** For a routine whose schedule state is
  `unscheduled_manual_only` or `unscheduled_no_trigger`, the surface shall report
  that the routine has no schedule.
- **AC-OFFICE-ROUTINE-ARMING-002.6:** A routine whose schedule state is `unknown`
  shall render as undetermined rather than as armed or as broken.
- **AC-OFFICE-ROUTINE-ARMING-002.7:** The routine list response shall be produced
  with a number of database queries that does not grow with the number of
  routines in the workspace. This bounds the path where every read succeeds; the
  per-routine re-read AC-OFFICE-ROUTINE-ARMING-001.13 requires after a failed
  batch read is exempt, at most one extra read per routine of that batch, because
  isolating the bad routine is worth more than the query count on a path that is
  already failing.
- **AC-OFFICE-ROUTINE-ARMING-002.8:** All copy added by this capability shall be
  routed through the translation layer and present in every shipped locale.
- **AC-OFFICE-ROUTINE-ARMING-002.9:** Every distinction this requirement demands
  shall be carried by text, an icon, or an accessible label, never by colour
  alone.
- **AC-OFFICE-ROUTINE-ARMING-002.10:** The surface shall render a distinct label
  for each of these groups: `armed`; `trigger_invalid`, `trigger_unscheduled` or
  `trigger_disabled`; `event_only`; `unscheduled_manual_only` or
  `unscheduled_no_trigger`; and `unknown`. Two routines in different groups shall
  never render the same label.
- **AC-OFFICE-ROUTINE-ARMING-002.11:** A routine whose schedule state is
  `event_only` shall not be reported as having no schedule and shall not be
  directed to create a trigger. It fires when its event arrives.
- **AC-OFFICE-ROUTINE-ARMING-002.12:** Intent and schedule state shall be carried
  in the same response for the same routine, and neither shall be omitted because
  the other was unavailable: a routine whose triggers could not be read carries
  its intent alongside schedule state `unknown`. The pair is not required to be a
  transactional snapshot. Intent is read from the routine row and schedule state
  from the trigger rows, so a write landing between those reads can yield a pair
  never simultaneously true in the database. That is accepted, not prevented:
  nothing here acts on the pair, and the next read reports the new value.

## Out of scope

Each exclusion is a contract: a later change that wants one needs its own
requirement.

- **The startup scan.** Not excluded from the capability, only from this file:
  `REQ-OFFICE-ROUTINE-ARMING-003` is in
  [Office Routine Arming Startup Scan](routine-arming-startup-scan.md) and is
  part of the same delivery.
- **Repair of any kind.** Nothing here enables, disables, creates, or deletes a
  trigger, and nothing here changes intent. Not automatically, and not on an
  operator's request either — see the two entries below.
- **The operator-initiated re-arm action. Deferred to task
  `b0382916-da13-44a5-85be-064ae6a9533c`.** A requirement for it was written
  alongside this document and cut before Build, so this surface reports a broken
  schedule and offers nothing that repairs it. What the successor needs: an
  explicit action over one named routine, setting `enabled = true` and a fresh
  `next_run_at` on exactly that routine's unarmed cron triggers that are
  schedulable, in trigger order, in one transaction with no partial application;
  never creating a trigger, never inferring a cron expression, never
  re-scheduling an already-armed trigger, and never reading intent to decide what
  to repair. It must keep four rejections distinct: no cron trigger to repair
  (including `event_only`, which per AC-OFFICE-ROUTINE-ARMING-002.11 must not be
  told to create one); cron triggers present but none schedulable; caller not
  authorized for the owning workspace; no such routine. It consumes
  AC-OFFICE-ROUTINE-ARMING-001.8's unarmed list and this document's
  classification, so it depends on this document and not the reverse. Three
  contract defects in the cut text travelled to that card unfixed and are
  recorded there.
- **Changing intent gating.** Routine status gating is owned by the routine
  status requirement. The existing scheduler consumes a due cron slot when the
  routine is not allowed to fire, and the manual and webhook handlers reject
  non-firing statuses. This capability only reports the status and schedule
  state; it does not change those gates.
- **The coordinator install.** Keeping onboarding from duplicating the
  pre-installed routine is
  [Office Coordinator Install Idempotency](coordinator-install-idempotency.md).
- **Cron evaluation changes.** Cron parsing and next-occurrence calculation use
  the shared `NextCronTime` implementation. It follows `crontab(5)` OR semantics
  for restricted day-of-month/day-of-week fields, applies the documented DST
  policy, and returns `ErrUnsatisfiableCron` for an expression with no possible
  occurrence. The classifier reports that last case as not schedulable. Changes
  to those scheduler rules are outside this capability.
- **The routine list's own ordering.** This document pins the order of every
  trigger list it reports, and the startup scan document pins its own iteration
  order. The order routines appear in the list response is pre-existing behaviour
  and is not changed here.
- **Editing a schedule.** Changing a cron expression, timezone, webhook secret or
  signing mode is not classification, and would not be re-arming either: a
  trigger that is not schedulable cannot be repaired by re-arming it and needs
  its expression edited.
- **Disabling a trigger from the UI.** No endpoint sets
  `office_routine_triggers.enabled` to false today and this document adds none.
  Whether the column should be retired is not decided here.
- **Using `office_routines.last_run_at` as schedule state.** Dispatch updates
  this column through `TouchRoutineLastRun`; it is a run-history value, not an
  input to schedule-state classification.
- **Runs stranded in `claimed`,** and any reaper for them. A stranded run does not
  affect whether the next tick fires.
- **Operator notification.** Inbox items, mail, and paging are separate work.
- **Assignee validity.** A routine armed against a deleted or paused assignee
  agent is `armed` here. Whether the woken agent can act is another contract.
- **Trigger reconciliation against a config source.** Config sync creates routines
  without triggers and the infrastructure reconciler gives them a `manual` one,
  which is how the live install reached `unscheduled_manual_only`. This document
  makes that state visible and changes what neither creates.
- **Webhook delivery.** Whether an `event_only` routine's caller actually posts to
  it, and whether its signature verifies, is not observed here. `event_only` says
  a schedule is absent and an event trigger is enabled, nothing more.
