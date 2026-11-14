---
id: coordinator-automatic-lowering
title: Lowering for raisable classes that cannot be undone
status: draft
system: coordinator
owners:
  - kandev
created: 2026-11-07
last_updated: 2026-11-07
---

# Lowering for raisable classes that cannot be undone Requirements

## Overview

Split out of [raisable action classes](automatic-classes.md) for file size;
the requirement ID is unchanged. Terms are those of
[automatic](automatic.md#terminology).

## Requirements

### REQ-COORDINATOR-AUTOMATIC-008: Lowering for classes that cannot be undone

**Intent:** An agent run cannot be un-run, so `resume` and `start_agent` are
never undoable; failure and manager action lower them instead.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-008.1:** The system shall not make a `resume`
  row undoable, and shall add no undo for an agent start. A `create_task` row
  stays undoable as before, whether or not its proposal sets `starts_agent`,
  and undoing the task of an automatic `starts_agent` create lowers
  `create_task` and `start_agent`
  ([AC-COORDINATOR-AUTOMATIC-004.2](automatic.md#req-coordinator-automatic-004-lowering)).
- **AC-COORDINATOR-AUTOMATIC-008.2:** When an automatic approval of a `resume`
  proposal ends `failed` (an agent start error, or a refusal at execution),
  the system shall lower `resume` with the reason "automatic action failed".
  When one that needs `start_agent` ends `failed`, it shall lower
  `start_agent` with that reason and leave `create_task` as the existing rules
  leave it. A resume that is accepted but deferred is not a failure, and a
  failure of a deferred or cold launch after the proposal settled `approved`
  is not observed and lowers nothing. A stale claim settled
  `outcome_unknown` and an execute deadline settled `outcome_unknown` count
  as `failed` for lowering. A stale `resume` claim is settled and never
  executed a second time; a stale automatic `create_task` claim is recovered
  as the create path always recovers it (re-using the claim, idempotent by
  external id), and if that recovery ends `failed` it lowers like any other
  failed automatic approval. Every site that settles an automatic attempt
  `failed` shall trigger the lowering. An agent-start failure after an
  automatic `starts_agent` create has created its task does not fail the
  proposal and lowers nothing.
- **AC-COORDINATOR-AUTOMATIC-008.3:** When a manager rejects, returns with a
  condition (a `pending` proposal only; a reply with a condition is not
  available on a `failed` one), or approves with edits a pending or failed
  proposal that needs
  `resume` or `start_agent` while that class reads `automatic`, the system
  shall lower that class with the reason "manager overrode an automatic
  class". An approval is "with edits" when the row it settles (`approved` or `failed`)
  has `edited` set, which a failed row records as an approved one does; a body
  equal to the stored spec is not an edit, and a plain approval does not lower.
  An approve lowers after its settling commit, not after its claim, so a
  claim that never settles (a crash) lowers nothing until a stale settlement
  writes the row. A manager's explicit
  lower through the settings save is the manual trigger
  (`AC-COORDINATOR-AUTOMATIC-004.1`). The proposals that need a class are
  those of `AC-COORDINATOR-AUTOMATIC-005.3`, so rejecting an agent-starting
  `move`, or a proposal that was held or created before the raise, lowers
  `start_agent` too (intended: the manager is declining an agent start the
  class would otherwise have run unattended, and the reason string is the same
  for every such lowering). Only the override lowering is reported: a lowering made by the failure of a
  stale automatic claim that the approve itself settled is not named in the
  approve response. A decision lowers at most one class
  (a `resume` proposal needs `resume`; a `starts_agent` proposal needs
  `start_agent` as its only raisable agent class). When it lowered a class, the
  reject, return and approve responses shall carry `lowered_class` (a string,
  `resume` or `start_agent`; the member is absent when nothing was lowered or
  the lowering was a guarded no-op) and the page shall tell the manager so
  with the class's display label (for example "Start agent lowered to requires
  approval"), never the wire token. The proposals that need a class are those
  of the stored proposal row at decision time; an edit never changes which
  classes were needed.
- **AC-COORDINATOR-AUTOMATIC-008.4:** For `resume` and `start_agent` the
  `no_undo` condition shall be unmet when, in the evidence window, an
  automatic approval of that class ended `failed`, or the class was lowered
  by the system (a class change with no user) for any reason other than
  "manager overrode an automatic class" or "raising manager no longer a
  manager". A manager's own lower, a lower made because a manager overrode a
  proposal, and a lower made because the raiser lost the role, do not make it
  unmet. Shrinking
  the evidence window shrinks the history this condition reads. For `create_task` the condition stays as in
  `AC-COORDINATOR-AUTOMATIC-002.1`.
- **AC-COORDINATOR-AUTOMATIC-008.5:** Each lowering shall be one idempotent
  system class change; a lower of a class that no longer reads `automatic`
  shall write nothing. A failed lowering shall be retried by the backstop
  tick when its trigger left durable evidence: an automatic `failed` activity
  row of the class (008.2), a rejected or returned decided row, or a decided row with
  `edited` set and any outcome (`approved` or `failed`), of a proposal
  needing the class (008.3), or an undone automatic task
  (004.2), in each case in the last 24 hours and dated strictly after the
  class's newest change to `automatic`, so a trigger that preceded a re-raise
  never lowers the new raise. The trigger's date is the proposal's claim time
  (`automatic_at`) for a failure, the decided row's `created_at` for an
  override, and the row's `undone_at` for an undo; a date equal to the raise's
  counts as preceding it. The strictly-after rule applies to the immediate lowering a
  trigger performs as well as to the backstop: the lowering re-reads the
  newest change to `automatic` inside the coordinator lock and writes nothing
  when the trigger's date does not follow it. The 24-hour limit is the
  backstop query's window only; the immediate lowering has no age limit. A raiser-invalid lowering
  instead writes nothing when the newest change to `automatic` is no longer
  the raise whose raiser was checked. A failed raiser-invalid lowering is
  logged and counted and is not retried; the next automatic attempt
  re-validates the raiser. The backstop shall visit every coordinator that
  has ever had a class change to `automatic`, whatever its autonomy state, and
  act only on classes that still read `automatic`.
