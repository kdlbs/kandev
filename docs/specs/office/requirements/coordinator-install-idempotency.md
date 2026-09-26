---
status: draft
system: office
created: 2026-09-17
owners:
  - kandev
---

# Office Coordinator Install Idempotency Requirements

## Overview

Onboarding pre-installs a `Coordinator heartbeat` routine. The installer decides
whether that routine is already present by matching the workspace, the assignee
agent, the canonical routine name, **and** the presence of a cron trigger whose
expression equals the canonical one. That last clause is the problem: trigger
presence and trigger expression are mutable, so a routine that was installed
successfully can stop matching itself later.

Two paths reach that state on an install running today. A half-finished install
leaves a routine row with no trigger at all, because trigger creation is a
second write that can fail after the routine is created and whose error is
warn-logged by both callers rather than rolled back. And a coordinator trigger
whose expression stops equalling the canonical one leaves a routine that no
longer matches itself. No endpoint edits a trigger in place — the trigger routes
are list, create, and delete — so that state is reached by deleting the
coordinator's trigger and creating a replacement carrying a different expression,
or by editing the row directly. Which path was taken matters less than the
result, because identity must not depend on a value that any of them can change.
Either way, re-running onboarding sees no match and creates a second coordinator
routine, so the workspace ends up with two heartbeats or, more often, two
routines neither of which fires.

The match is *not* sensitive to `enabled` today — the installer never reads that
column. This document's job is to keep it that way while removing the two
mutable clauses it does match on. Making identity depend on `enabled`, which a
reader coming from the card's "disabled trigger" framing might reasonably assume
is already the case or is the fix, would **add a third duplication path** rather
than close one, and is forbidden by AC-OFFICE-COORDINATOR-INSTALL-001.1.

This document does not define or repair a schedule. It consumes the
classification of
[Office Routine Arming Visibility](routine-arming-visibility.md) — schedule state
is defined there, and AC-OFFICE-COORDINATOR-INSTALL-001.5 requires the install to
report it — so this document cannot ship before that one.

## Terminology

This document uses the terms defined in
[Office Routine Arming Visibility § Terminology](routine-arming-visibility.md):
*fire*, *armed*, *intent*, and *schedule state*. It adds one:

- **Canonical:** the routine name, cron expression, and timezone that onboarding
  itself supplies for the coordinator routine. Canonical values are the
  installer's own constants, not operator input.

## Requirements

### REQ-OFFICE-COORDINATOR-INSTALL-001: Match the coordinator routine on identity, not on mutable trigger state

**Intent:** The pre-installed coordinator routine is matched on name plus the
presence of a cron trigger carrying an exact expression. A routine whose trigger
creation failed, or whose expression was edited, therefore stops matching itself
and the installer creates a second coordinator routine. Identity must not depend
on mutable trigger state.

**User story:** As an operator re-running onboarding on an upgraded install, I
want one coordinator routine, with its schedule completed rather than duplicated.

#### Acceptance criteria

- **AC-OFFICE-COORDINATOR-INSTALL-001.1:** The coordinator routine's identity shall be
  the workspace, the assignee agent, and the canonical routine name, and shall
  not depend on any trigger's `enabled` value, `next_run_at`, cron expression, or
  presence.
- **AC-OFFICE-COORDINATOR-INSTALL-001.2:** Installing the coordinator routine when a
  routine with that identity already exists shall return the existing routine and
  shall not create a second one, whatever its schedule state.
- **AC-OFFICE-COORDINATOR-INSTALL-001.3:** When more than one existing routine
  satisfies that identity, the install shall return the one with the earliest
  `office_routines.created_at`, breaking ties by `office_routines.id` ascending.
  It shall delete none of them and shall report that more than one was found. The
  routine it selects shall then be subject to
  AC-OFFICE-COORDINATOR-INSTALL-001.4 and AC-OFFICE-COORDINATOR-INSTALL-001.5
  exactly as a single match would be, so a duplicated half-finished install is
  completed rather than left permanently unable to fire. The routines it did not
  select shall be left entirely untouched, including their triggers.
- **AC-OFFICE-COORDINATOR-INSTALL-001.4:** Installing the coordinator routine when a
  routine with that identity exists with no cron trigger shall create the
  canonical cron trigger for it and return the existing routine, completing an
  install whose trigger creation previously failed. The created trigger shall
  carry the canonical cron expression, the canonical timezone, `enabled = true`,
  and a `next_run_at` equal to the first occurrence of the canonical expression
  evaluated in the canonical timezone strictly after a single instant captured
  once at the start of the install. A repaired trigger whose `next_run_at` is
  null is not a completed install: it classifies `trigger_unscheduled` and still
  cannot fire, which is the state this criterion exists to leave behind. When
  that next occurrence cannot be computed, the install shall be rejected and
  shall report the failure, leaving the existing routine in place; because the
  expression and timezone are the installer's own constants, that failure is a
  defect in the constants and no input can reach it. When that trigger creation fails, the install shall report
  the failure and shall leave the existing routine in place rather than deleting
  it, so that the next install reaches this same criterion and completes the
  schedule. This criterion is therefore re-entrant: running it again against a
  routine it already repaired takes the
  AC-OFFICE-COORDINATOR-INSTALL-001.5 branch and changes nothing.
- **AC-OFFICE-COORDINATOR-INSTALL-001.5:** Installing the coordinator routine when a
  routine with that identity exists with any cron trigger, enabled or disabled,
  canonical expression or not, shall leave every trigger unchanged, shall create
  no second trigger, and shall report the routine's schedule state.
- **AC-OFFICE-COORDINATOR-INSTALL-001.6:** Installing the coordinator routine shall
  never change an existing routine's `status`.
- **AC-OFFICE-COORDINATOR-INSTALL-001.7:** When the identity lookup itself fails, or
  when the read of the existing routine's triggers that decides between
  AC-OFFICE-COORDINATOR-INSTALL-001.4 and AC-OFFICE-COORDINATOR-INSTALL-001.5
  fails, the install shall be rejected and shall create nothing. An indeterminate
  read shall never be treated as "absent": neither as an absent routine, which
  duplicates the routine, nor as an absent trigger, which duplicates the
  trigger.
- **AC-OFFICE-COORDINATOR-INSTALL-001.8:** Two concurrent installs for the same
  workspace and assignee shall between them create at most one new coordinator
  routine and at most one new canonical cron trigger, whichever order the two
  callers interleave. This bounds what the install *creates*; it is not a
  uniqueness guarantee over the table. Duplicate routines a previous install
  already created remain, and are handled by
  AC-OFFICE-COORDINATOR-INSTALL-001.3; triggers that already exist remain, and
  are handled by AC-OFFICE-COORDINATOR-INSTALL-001.5.
- **AC-OFFICE-COORDINATOR-INSTALL-001.9:** No database uniqueness constraint shall
  be added to enforce
  AC-OFFICE-COORDINATOR-INSTALL-001.8. Upgraded installs already hold duplicate
  coordinator routines, so a unique index over the identity columns would fail to
  apply on exactly the installs this document exists to fix, and the alternative
  of deleting rows to make it apply is out of scope below. The guarantee shall
  instead be met by serializing the check-and-create against concurrent installs
  for the same workspace and assignee. The domain of that serialization shall be
  every installer call that can reach the same database, not merely every call
  within one process: the guarantee shall hold when two processes share a
  database, which rules out a purely in-process guard as the sole mechanism. It
  shall hold on both dialects named in
  AC-OFFICE-COORDINATOR-INSTALL-001.11, which rules out any mechanism available
  on only one of them. Acquisition shall not block indefinitely: it shall either
  acquire or fail within a single named compile-time bound of 5 seconds, and that
  bound shall not be operator-configurable — no environment variable,
  configuration key, or runtime flag shall expose it — because both of the
  installer's callers run on a user-facing onboarding path, where an unbounded
  wait is indistinguishable from a hang.
- **AC-OFFICE-COORDINATOR-INSTALL-001.10:** The reports required by
  AC-OFFICE-COORDINATOR-INSTALL-001.3 and
  AC-OFFICE-COORDINATOR-INSTALL-001.5 shall be a structured record plus a counter
  per condition, not an additional value returned from the install call. The
  routine the call already returns under
  AC-OFFICE-COORDINATOR-INSTALL-001.2 and
  AC-OFFICE-COORDINATOR-INSTALL-001.3 is unchanged; what this criterion forbids
  is carrying the *reports* out that way. Both of the installer's callers today
  discard everything the call returns except the error, and neither is being
  changed to inspect a richer return, so a report delivered as a return value
  would report to nobody. Each record shall name the workspace, the assignee
  agent, and the routine it concerns — for
  AC-OFFICE-COORDINATOR-INSTALL-001.3, the routine it selected, and the number of
  routines that matched. Each condition that applies shall be reported exactly
  once per install call. A call that is rejected shall still report every
  condition it had already detected before rejecting, in particular the
  duplicate-routines condition of AC-OFFICE-COORDINATOR-INSTALL-001.3, which
  describes state the call found in the database rather than work the call
  performed. These counters therefore count the calls in which a condition was
  observed, not the installs that completed, so a duplicated coordinator is
  reported on the runs that fail as well as the runs that succeed.
- **AC-OFFICE-COORDINATOR-INSTALL-001.11:** The install shall behave identically on
  SQLite and on PostgreSQL.
- **AC-OFFICE-COORDINATOR-INSTALL-001.12:** Installing the coordinator routine when
  no routine with that identity exists shall create one routine carrying that
  identity, with `status = 'active'`, together with its canonical cron trigger
  carrying every field required by AC-OFFICE-COORDINATOR-INSTALL-001.4, and shall
  return the created routine. When the routine insert fails, the install shall be
  rejected, shall create no trigger, and shall report the failure. When the
  routine insert succeeds and the trigger creation fails, the install shall leave
  the created routine in place and report the failure, so that the next install
  reaches AC-OFFICE-COORDINATOR-INSTALL-001.4 and completes the schedule; this is
  the half-finished install this document exists to make recoverable, and it is
  reached by the same path whether the first install failed here or before this
  document existed.
- **AC-OFFICE-COORDINATOR-INSTALL-001.13:** When the serialization required by
  AC-OFFICE-COORDINATOR-INSTALL-001.9 cannot be acquired, or the serialized
  section fails because a concurrent install held it, the install shall be
  rejected, shall create nothing, and shall report contention distinguishably
  from the failures of AC-OFFICE-COORDINATOR-INSTALL-001.7. It shall not retry
  within the same call. A rejected install leaves the routine the winning caller
  created, so the next install returns it under
  AC-OFFICE-COORDINATOR-INSTALL-001.2; contention therefore costs a warning, not
  a duplicate and not a lost coordinator. Reaching the acquisition bound of
  AC-OFFICE-COORDINATOR-INSTALL-001.9 without acquiring is contention and shall be
  reported as contention. Cancellation of the caller's context while waiting shall
  likewise reject the install and create nothing, and shall be reported
  distinguishably from both contention and the failures of
  AC-OFFICE-COORDINATOR-INSTALL-001.7, so that a shutdown part-way through
  onboarding is not recorded as a duplicate-install race.

- **AC-OFFICE-COORDINATOR-INSTALL-001.14:** The install shall reject a call whose
  workspace, assignee agent, or canonical routine name is empty, before performing
  the identity lookup, and shall create nothing. An empty value is not an
  identity: accepting one lets a malformed call match, or create, a routine that
  no real workspace or agent owns, and AC-OFFICE-COORDINATOR-INSTALL-001.1 makes
  those three values the whole of identity. This keeps the guard the installer
  applies today and extends it to the third component. The rejection shall be
  reported distinguishably from the read failures of
  AC-OFFICE-COORDINATOR-INSTALL-001.7, because nothing was read.
- **AC-OFFICE-COORDINATOR-INSTALL-001.15:** No failure in this requirement shall
  be retried inside the same install call: not the identity lookup or the trigger
  read of AC-OFFICE-COORDINATOR-INSTALL-001.7, not the trigger creation of
  AC-OFFICE-COORDINATOR-INSTALL-001.4, not the routine insert or trigger creation
  of AC-OFFICE-COORDINATOR-INSTALL-001.12, and not the serialization of
  AC-OFFICE-COORDINATOR-INSTALL-001.9 and
  AC-OFFICE-COORDINATOR-INSTALL-001.13. Each rejection returns to its caller, and
  the next install is what reaches the criterion that completes the work; that is
  the recovery model the rest of this requirement is built on, and an in-call
  retry would only lengthen a failure on an onboarding path. A rejected install
  shall surface its failure as an error, because the error is the only channel
  either caller reads. It may also return the routine it found or created
  alongside that error, as the installer does today: no criterion here depends on
  that value, so Build is free to keep or drop it.

## Out of scope

- **Defining or repairing a schedule.** The classification belongs to
  [Office Routine Arming Visibility](routine-arming-visibility.md). This document
  *consumes* it — AC-OFFICE-COORDINATOR-INSTALL-001.5 reports schedule state, so
  reporting is in scope and only the definition is not. It creates a canonical
  trigger where none exists and otherwise touches no trigger.
- **Repairing a coordinator routine that is present but unarmed.** An install
  reports the schedule state and stops. It does not enable a disabled trigger and
  does not re-schedule a trigger whose `next_run_at` is null, even its own
  canonical one, because it cannot tell an operator's deliberate choice from a
  defect. The operator-initiated action that would repair it is deferred; see
  that document's `## Out of scope`, which cites the successor card.
- **Merging or deleting duplicate coordinator routines** created by an earlier
  install. The install reports that more than one was found and deletes none;
  deciding which to keep needs an operator. This is what makes the uniqueness
  constraint of AC-OFFICE-COORDINATOR-INSTALL-001.9 unavailable, and the two
  decisions stand or fall together.
- **Correcting a coordinator trigger whose cron expression an operator edited.**
  Identity stops depending on the expression; the edited value is left alone.
- **Rolling back a half-finished install.** This document completes one on the
  next run rather than unwinding it, and does not change either caller's existing
  choice to warn-log a failed install and continue.
- **Which serialization mechanism is used.**
  AC-OFFICE-COORDINATOR-INSTALL-001.9 states the property, its domain, and the
  two constraints that narrow the choice; AC-OFFICE-COORDINATOR-INSTALL-001.13
  states what contention does. Which conforming mechanism Build picks is its
  decision. The three candidates are not interchangeable and this document does
  not offer them as though they were: a transaction at the default isolation
  level of either dialect does not by itself stop two callers both reading
  "absent" and both inserting, a PostgreSQL advisory lock has no SQLite
  equivalent, and an in-process guard fails the cross-process domain. Whatever is
  chosen must satisfy .9 and .13 as written.
- **Other pre-installed routines.** Onboarding installs one routine today. A
  second would need this contract restated for it, not inherited silently.
- **Whether the coordinator routine should exist at all,** and what its canonical
  cron expression should be. This document keeps the install idempotent; it does
  not revisit the values being installed.
