---
status: draft
system: office
created: 2026-09-17
owners:
  - kandev
---

# Office Routine Arming Startup Scan Requirements

## Overview

The install this capability exists for is unattended: nobody opens the routines
page for weeks, so the read-path surface of
[Office Routine Arming Visibility](routine-arming-visibility.md) leaves a dead
loop undetected exactly where detection matters most. This document owns the
read-only scan that runs once at Office startup and reports what the classifier
found, to logs and to counters.

It classifies nothing of its own: schedule state, the unarmed cron trigger list,
and the dispatch grace are all defined by that document, and this one consumes
them. It was authored inside it and moved here unchanged in identity to stay
inside the requirement-file size limit: the requirement is still
`REQ-OFFICE-ROUTINE-ARMING-003` and its criteria are still numbered
`AC-OFFICE-ROUTINE-ARMING-003.1` onward, so every reference to them elsewhere
still resolves.

## Terminology

This document uses the terms defined in
[Office Routine Arming Visibility § Terminology](routine-arming-visibility.md),
unchanged: *fire*, *intent*, *schedule state*, *armed*, *schedulable*, *dispatch
grace*, *trigger order*, *unarmed cron trigger*, and *surface*.

## Requirements

### REQ-OFFICE-ROUTINE-ARMING-003: Surface unarmed routines at startup

**Intent:** The install this capability exists for is unattended. Nobody opens
the routines page for weeks, so a UI-only surface leaves the dead loop undetected
exactly where detection matters most.

**User story:** As an operator reading logs or metrics after an upgrade, I want a
routine that is switched on and cannot fire to have left a trace.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-ARMING-003.1:** During Office startup the system shall
  classify every routine it can enumerate, in every workspace it can enumerate,
  and report the result. The scan shall read only after Office startup
  reconciliation has returned — the pass that gives every trigger-less routine a
  `manual` trigger — whether or not that pass succeeded, so that a routine the
  scan reports as `unscheduled_no_trigger` is one reconciliation left without a
  trigger rather than one the scan happened to read first. The ordering shall be
  established by an explicit completion signal handed to the scan by the startup
  sequence that ran reconciliation, and by nothing else: not a timer, not a retry,
  and not an inspection of trigger rows. The scan is either invoked with that
  signal or invoked without it. Invoked without it, the scan shall classify
  nothing, emit a warning-level record, and increment the skipped counter of
  AC-OFFICE-ROUTINE-ARMING-003.7 with a reason distinguishing this cause both from
  enumeration failure and from per-routine classification failure. The startup
  path that exists today always supplies the signal, so that branch is not reached
  in production; it exists so that a caller which cannot establish the ordering is
  refused rather than reporting a routine as `unscheduled_no_trigger` before
  reconciliation has given it a trigger, and it is reached in a test by invoking
  the scan without the signal.
- **AC-OFFICE-ROUTINE-ARMING-003.2:** The scan shall iterate workspaces ordered
  by `workspaces.created_at` ascending, breaking ties by `workspaces.id`
  ascending, and within each workspace shall iterate routines ordered by
  `office_routines.created_at` ascending, breaking ties by `office_routines.id`
  ascending, so that two scans over unchanged data report in the same order.
- **AC-OFFICE-ROUTINE-ARMING-003.3:** For a routine whose intent is `active` and
  whose schedule state is `trigger_disabled`, `trigger_unscheduled` or
  `trigger_invalid`, the system shall emit a warning-level record naming the
  workspace, the routine, the schedule state, and every unarmed cron trigger in
  trigger order.
- **AC-OFFICE-ROUTINE-ARMING-003.4:** For a routine whose intent is not `active`,
  the system shall emit an informational record stating that the routine is
  deliberately off, and shall not emit a warning.
- **AC-OFFICE-ROUTINE-ARMING-003.5:** For a routine whose intent is `active` and
  whose schedule state is neither one of those in
  AC-OFFICE-ROUTINE-ARMING-003.3 nor one of those in
  AC-OFFICE-ROUTINE-ARMING-003.14 nor `unknown`, the system shall emit an
  informational record and no warning.
- **AC-OFFICE-ROUTINE-ARMING-003.6:** The system shall publish a counter of scan
  observations labelled by workspace, intent, and schedule state.
- **AC-OFFICE-ROUTINE-ARMING-003.7:** When the scan cannot classify a routine it
  enumerated, the system shall increment a skipped counter labelled by reason, so
  that a scan failing closed is visible rather than silent.
- **AC-OFFICE-ROUTINE-ARMING-003.8:** When the scan cannot enumerate the
  workspace list at all, it shall increment that skipped counter with a reason
  distinguishing enumeration failure from per-routine classification failure,
  emit a warning-level record, and return without classifying anything.
- **AC-OFFICE-ROUTINE-ARMING-003.9:** A routine whose schedule state is `unknown`
  shall emit an informational record naming the workspace, the routine, and the
  `unknown` state, whatever its intent, and shall not emit a warning. An unreadable
  row is not evidence of a broken routine. That record is the only one emitted for
  such a routine: it stands in place of the record
  AC-OFFICE-ROUTINE-ARMING-003.4 would otherwise require when the intent is not
  `active`, so every routine the scan reaches produces exactly one record. The
  routine shall increment the skipped counter of AC-OFFICE-ROUTINE-ARMING-003.7
  once, and shall also be counted once by the observation counter of
  AC-OFFICE-ROUTINE-ARMING-003.6 under the `unknown` schedule state. Incrementing
  both is intended, not double counting: the two answer different questions — how
  many routines were observed in each state, and how many the scan could not
  decide.
- **AC-OFFICE-ROUTINE-ARMING-003.10:** Office startup shall complete without
  waiting for the scan to finish, and no scan outcome, including a database
  error, shall fail startup or change its result.
- **AC-OFFICE-ROUTINE-ARMING-003.11:** The scan shall write nothing, per
  AC-OFFICE-ROUTINE-ARMING-001.7, and shall not enable, disable, create, or
  delete a trigger.
- **AC-OFFICE-ROUTINE-ARMING-003.12:** Running the scan twice over unchanged data
  shall emit the same records, in the same order, and shall leave the database
  unchanged. Two records are the same when they carry the same level, the same
  workspace, the same routine, the same schedule state, and the same unarmed cron
  triggers in the same order; wall-clock timestamps, durations, and fields the
  logging layer adds are excluded from the comparison, as are counters, which
  accumulate. The equality is asserted only where classification does not depend
  on where the classification instant falls relative to the dispatch grace: a
  routine is comparable between two scans when each of its cron triggers is on
  the same side of the grace at both classification instants of
  AC-OFFICE-ROUTINE-ARMING-001.12, where a null `last_fired_at` counts as outside
  the grace at every instant. A trigger inside the grace at one scan and
  outside it at the other changes schedule state with no database change, by the
  design of rule 2, and is excluded from the comparison rather than counted as a
  violation. The equality is likewise asserted only between two scans that each
  enumerated the workspace list and every workspace's routines successfully and
  classified every routine they enumerated: a scan in which
  AC-OFFICE-ROUTINE-ARMING-003.8, AC-OFFICE-ROUTINE-ARMING-003.13, or a
  per-routine classification failure occurred emits a different set of records with
  no database change, by the design of those criteria, and is excluded from the
  comparison. This criterion constrains classification, not the database's
  willingness to answer.
- **AC-OFFICE-ROUTINE-ARMING-003.14:** For a routine whose intent is `active` and
  whose schedule state is `unscheduled_manual_only` or `unscheduled_no_trigger`,
  the system shall emit a warning-level record naming the workspace, the routine,
  and the schedule state, and stating that the routine has no schedule. The record
  shall carry no unarmed cron trigger list, because a routine in either state owns
  no cron trigger. This is the state
  [Office Routine Arming Visibility](routine-arming-visibility.md) names as the
  sharper failure — switched on, presenting as on, incapable of firing — which is
  why it warns rather than informs. The scan cannot tell a routine an operator
  fires by hand from one whose schedule was never created, and this criterion does
  not ask it to: either way the routine will not fire on a schedule, the operator
  is the only one who can say which was meant, and the cost of saying so is one
  warning per such routine per startup.
- **AC-OFFICE-ROUTINE-ARMING-003.13:** When the scan enumerates the workspace
  list but then fails to enumerate the routines of one workspace, it shall retain
  every record already emitted for earlier workspaces, increment the skipped
  counter of AC-OFFICE-ROUTINE-ARMING-003.7 once for that workspace with the
  enumeration-failure reason, emit a warning-level record naming it, and continue
  to the next workspace in the order of
  AC-OFFICE-ROUTINE-ARMING-003.2. It shall not retry that workspace within the
  same scan and shall not abandon the scan.

## Out of scope

- **Classifying a routine.** The classification table, the unarmed cron trigger
  list, and the dispatch grace belong to
  [Office Routine Arming Visibility](routine-arming-visibility.md). This document
  reports what that classifier returns and defines no state of its own.
- **Repair of any kind.** The scan writes nothing, per
  AC-OFFICE-ROUTINE-ARMING-003.11. It does not re-arm, and the operator-initiated
  re-arm action is deferred entirely — see that document's `## Out of scope`.
- **Running the scan again.** This is one pass at startup. A periodic re-scan, an
  operator-invoked re-scan, and an API that returns the scan's findings are each
  separate work; the read-path response of
  REQ-OFFICE-ROUTINE-ARMING-002 already reports the same values on demand.
- **Operator notification.** Inbox items, mail, and paging are separate work. The
  scan emits records and counters and stops.
- **What startup reconciliation itself does.** AC-OFFICE-ROUTINE-ARMING-003.1
  orders the scan after it and reports the result; it does not change which
  triggers reconciliation creates.
