---
id: coordinator-automatic-classes
title: Raisable action classes and eligibility settings
status: draft
system: coordinator
owners:
  - kandev
created: 2026-11-07
last_updated: 2026-11-07
---

# Raisable action classes and eligibility settings Requirements

## Overview

The 2026-11-07 amendment to [the first automatic action class](automatic.md):
a manager may raise `resume` and `start_agent` to `automatic` as well as
`create_task`, and the gates that earn the raise become bounded
per-coordinator settings. This revisits D13 on the maintainer's decision to
earn the raise from one to two days of approvals instead of thirty; the
phase 3 ADR's 2026-11-07 amendment records it. Terms are those of
[automatic](automatic.md#terminology).

## Requirements

### REQ-COORDINATOR-AUTOMATIC-005: The raisable classes (amended)

**Intent:** `resume` and `start_agent` join `create_task` as classes a manager
may raise, on the maintainer's 2026-11-07 decision to earn the raise from one
to two days of approvals instead of thirty. This revisits D13; the decision is
recorded in the phase 3 ADR's 2026-11-07 amendment. The class `start_agent` is
not a proposal kind: it is the modifier carried by a proposal whose target
step starts an agent on enter (the proposal's `starts_agent` flag).

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-005.1:** The raisable set shall be exactly
  `create_task`, `resume` and `start_agent`. The class routes
  (`GET .../classes/:class/eligibility`, `POST .../classes/:class/reviews`)
  shall accept these three and answer 400 naming `class` for any other value.
- **AC-COORDINATOR-AUTOMATIC-005.2:** For `resume`, decided rows shall be the
  coordinator's activity rows of class `resume`. For `start_agent`, decided
  rows shall be the coordinator's decided activity rows, of any kind class,
  whose proposal has `starts_agent` set. A row of a `starts_agent` proposal
  also counts for its kind's class, so `create_task` evidence is unchanged.
  The decided-row outcomes and the exclusion of rows with authorization
  `automatic` are those of [Terminology](automatic.md#terminology).
- **AC-COORDINATOR-AUTOMATIC-005.3:** A proposal shall be approved
  automatically only when every class it needs reads `automatic`: its kind's
  class and, when `starts_agent` is set, `start_agent` as well. A `starts_agent`
  `move` proposal needs `move`, which is not raisable, so it always waits for
  a manager. A `starts_agent` `create_task` needs `create_task` and
  `start_agent`.
- **AC-COORDINATOR-AUTOMATIC-005.4:** Raising a class shall never happen
  without an explicit manager save. Changing the eligibility settings shall
  not raise any class, and tightening them shall not lower a class that is
  already `automatic`. A class review only records evidence: it never
  re-evaluates eligibility and never lowers a class. An `automatic` class
  leaves `automatic` only through a lowering trigger
  ([AC-COORDINATOR-AUTOMATIC-004.1](automatic.md#req-coordinator-automatic-004-lowering),
  `-004.2`, the raiser-invalid lowering of `AC-COORDINATOR-AUTOMATIC-007.4`, [REQ-COORDINATOR-AUTOMATIC-008](automatic-lowering.md#req-coordinator-automatic-008-lowering-for-classes-that-cannot-be-undone)).
- **AC-COORDINATOR-AUTOMATIC-005.5:** The activity list (the log) shall accept
  `class=start_agent` and then return the coordinator's rows, of any outcome
  and any kind class, whose proposal has `starts_agent` set, in the list's
  existing order (newest first, `created_at` descending with `id` descending
  as the tiebreak) and with its existing cursor and page size. Rows with a
  null `proposal_id` never match. This is the list that **Review the last
  <window>** opens for `start_agent`
  ([AC-COORDINATOR-AUTOMATIC-002.3](automatic.md#req-coordinator-automatic-002-earning-the-raise));
  the per-class counts in the log summary are unchanged.
- **AC-COORDINATOR-AUTOMATIC-005.6:** While `start_agent` reads `automatic`
  and `create_task` does not, the permission settings shall say that
  `start_agent` takes effect only for agent-starting task creations that
  `create_task` also automates.

### REQ-COORDINATOR-AUTOMATIC-006: Per-coordinator eligibility settings

**Intent:** The gates are numbers a manager can tune per coordinator within
bounds, with today's values as the defaults.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-006.1:** Each coordinator shall hold five
  eligibility settings with these defaults and inclusive bounds: evidence
  window 720 hours (24 to 2160); minimum decisions 20 (3 to 500);
  approved-unedited floor 90 percent (50 to 100); review max age 168 hours
  (1 to 720); daily automatic limit 10 (1 to 100). A coordinator that never
  set them, including every coordinator existing before this change, shall
  behave exactly as before, including `unedited_rate` being unmet when there are no decided rows.
- **AC-COORDINATOR-AUTOMATIC-006.2:** The settings shall be written with the
  coordinator's permission settings, in the same locked write, with the same
  configuration revision bump as a policy change and the audit record of
  AC-COORDINATOR-AUTOMATIC-006.9. A change to the eligibility settings alone
  writes no `coordinator_class_changes` row, because it changes no class.
  A value outside its bounds, not a JSON integer literal (`20.5` and `20.0`
  are both refused), or an unknown settings field shall be refused with 400
  naming `policy.eligibility.<field>` in the error's `field` (or
  `policy.eligibility` for a member that is not an object), with code
  `invalid_setting`, storing nothing. The first error is reported: actions
  errors, then a non-object member, unknown fields (name ascending), missing
  fields, non-integers, then out-of-bounds values (the last three in field table
  order). A coordinator principal shall be refused as for the policy.
- **AC-COORDINATOR-AUTOMATIC-006.3:** The settings apply to all three
  raisable classes of that coordinator. The raise gate shall evaluate the
  eligibility settings stored before the save, so one save cannot loosen
  them and raise a class in the same write.
- **AC-COORDINATOR-AUTOMATIC-006.4:** The eligibility response shall include
  the settings in force. The condition tokens on the wire shall stay
  `history_30d`, `volume`, `unedited_rate`, `no_undo` and `reviewed_7d`
  whatever the settings; the UI shall label `history_30d` and `reviewed_7d`
  from the configured values (for example "history: 1 day").
- **AC-COORDINATOR-AUTOMATIC-006.5:** The settings page shall let a manager
  edit the five values, show the bounds, and state that the daily limit is
  one count per coordinator across all classes. The settings response (the
  same payload as the policy) shall carry the effective `eligibility` object
  with all five values, defaults filled in. The page shall send `eligibility`
  only when a value differs from the one loaded, and shall show a 400 on a
  field next to that field. A window label shall read "N days" when the hours
  are a multiple of 24 and "N hours" otherwise (36 hours reads "36 hours"). Where the page withholds the raise lists because
  phase 3 is not effective, it shall show the five values read-only (inputs
  disabled, never sent) with a note that they apply when automatic is
  available; a 400 on `policy.eligibility` shall be shown as an error above the
  group.

- **AC-COORDINATOR-AUTOMATIC-006.6:** With evidence window 24 hours, minimum
  decisions 3 and the defaults otherwise, a coordinator whose earliest
  `resume` decided row is at least 24 hours old, with at least 3 `resume`
  decided rows in the last 24 hours all approved without edits, no failed or
  system-lowered automatic `resume` in the window and a class review recorded
  within 168 hours, shall be eligible for `resume`. Note: the earliest-row
  condition needs a decision at or before the window start, so unless the
  earliest row falls exactly on the window start the example needs at least
  four decided rows in total.
- **AC-COORDINATOR-AUTOMATIC-006.7:** Every writer of a coordinator's policy
  shall preserve the stored eligibility settings unless the request carries
  an `eligibility` object. This covers a settings save without the member or
  with `"eligibility": null`, every system lowering, and any later policy
  writer. A saved `eligibility` object shall name all five fields (as
  `policy.actions` must name every action) and replaces the stored object as a
  whole; one that omits a field is refused with 400 naming
  `policy.eligibility.<field>`. A save whose `eligibility` equals the stored
  object, and no other change, shall not bump the configuration revision.
  "Equal" compares effective values, so an absent object equals an explicit
  object holding the defaults: saving the defaults over a coordinator that
  never set them changes nothing, bumps nothing and writes no record.
  "Configuration revision" in this requirement is the coordinator's
  `policy_revision`.
- **AC-COORDINATOR-AUTOMATIC-006.8:** The class review record's
  `window_start` shall be the review time minus the evidence window in force
  at that time. A review shall count toward the review condition for the
  review max age in force when the condition is evaluated, whatever the
  evidence window was when it was recorded.
- **AC-COORDINATOR-AUTOMATIC-006.9:** A save that changes the effective
  eligibility settings shall, in the same locked transaction as the policy
  write, append one row to `coordinator_eligibility_changes` holding the
  coordinator, the saving manager (null when the request carries no resolvable
  user, as with authentication off), the before and after effective objects and
  the new `policy_revision`. A save that changes nothing appends nothing. The
  row is the durable audit record; no route reads it in this change. A save
  that changes only the eligibility settings shall not reset or archive the
  coordinator's conversation (the agent never reads these values); a save
  that also changes actions, watches or projects does so as it does today.
  An eligibility-only save that changes the effective settings still
  publishes `coordinator.updated` as any settings change does. A stored
  `eligibility` field that is missing or outside its bounds when read takes its
  default for that field alone (logged at warn); the other fields keep their
  stored values, and the policy stays readable.
- **AC-COORDINATOR-AUTOMATIC-006.10:** Recording a class review shall read the
  evidence window and count the decided rows (for all three classes, including
  the `start_agent` join) inside the coordinator lock that the settings save
  takes, so a review and a concurrent save are ordered and a
  review's `window_start` and row count come from one settings snapshot. A
  failure to take the lock, count the rows or insert the review shall answer
  the existing 503 decision-log-unavailable error and record no review. A
  repeated review request appends another review row; reviews carry no
  idempotency key, and the newest review within the review max age satisfies
  the condition.

- **AC-COORDINATOR-AUTOMATIC-006.11:** While phase 3 is not effective, a
  settings save whose body carries a non-null `eligibility` member shall be
  refused with 400 naming `policy.eligibility`, storing nothing; an absent or
  null member is accepted as today and leaves the stored object untouched. The
  settings response shall still carry the effective `eligibility` object
  (defaults filled in), so the page can display it. The class routes,
  including `GET /coordinators/:cid/classes/:class/eligibility`, stay absent
  (404) with phase 3 off, as today; raising and lowering stay phase 3 only.

### REQ-COORDINATOR-AUTOMATIC-007: Automatic resume and start_agent

**Intent:** An automatic resume or agent start is the approval a manager
would have given, but it runs an agent unattended, so containment and the
cost ceiling gate it as they gate an unattended turn.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-007.1:** When the coordinator calls
  `propose_resume_kandev` or `propose_task_kandev`, validation passes, and
  every class the proposal needs reads `automatic`, the system shall store the
  proposal and approve it through the same approve path as a manager, with no
  edits, decider `automatic` and the raising manager as actor, using the same
  conditional claim as the `create_task` path. The result shall carry
  `{proposal_id, status}` and `task_id` when the kind creates a task. The
  claim shall match only a proposal that is `pending` and has never been
  claimed automatically; a proposal a manager has touched or that an earlier
  attempt failed is never claimed by the automatic path.
- **AC-COORDINATOR-AUTOMATIC-007.5:** When `propose_resume_kandev` returns an
  already open proposal (the existing deduplication of the kind path: same
  coordinator, kind and target, status `pending`, `approving` or `failed`), the
  system shall make no automatic attempt: no admission read, no claim, no
  count, no lowering, and no note. The tool result shall be the existing one,
  including its `deduplicated` flag, with the open proposal's current status.
  Automatic approval is tried only when a proposal is created
  (`AC-COORDINATOR-PAUSE-002.3`). `propose_task_kandev` has no deduplication
  (each call stores a new proposal and returns no `deduplicated` flag), so it
  has no such case and this change adds none.
- **AC-COORDINATOR-AUTOMATIC-007.2:** Before the claim, for a proposal that
  needs `resume` or `start_agent`, the system shall run the unattended-turn
  admission checks for autonomy, containment and the cost ceiling. When any
  fails, the proposal shall stay `pending`, nothing shall count toward the
  daily limit, and the tool result shall carry the note
  "automatic action held (<reason>); a manager will decide" where
  `<reason>` is the failing admission reason token only (no detail text), or
  `read_error` when the admission result's detail is `read_error` (the
  coordinator, autonomy or containment input could not be read). A spend that
  cannot be read or measured is the admission reason `spend_unmeasured`, and
  that is its token. The admission read
  writes nothing, counts nothing toward any limit and starts no turn. Pause
  is re-read under the coordinator lock immediately before the claim, for
  every class, as `AC-COORDINATOR-PAUSE-002.3` requires; a Pause committed
  before that read leaves the proposal `pending` with the paused note and
  counts nothing. Admission and the cost ceiling are read once per proposal
  before the lock and are not re-read under it: a ceiling crossing that lands
  after a proposal's read does not stop that proposal, so the number of
  claims that complete after such a crossing is bounded by the daily limit,
  not by one.
- **AC-COORDINATOR-AUTOMATIC-007.3:** An automatic approval of a `resume` or
  `start_agent` proposal shall count toward the daily limit and the Pause,
  raiser-validity, pending-claim and lower-wins rules defined in the
  [automatic design](../system-design/automatic.md) (the Automatic approval
  section) and `AC-COORDINATOR-AUTOMATIC-003` and `-004.1` exactly as
  `create_task` does.
- **AC-COORDINATOR-AUTOMATIC-007.4:** When the classes a proposal needs were
  raised by different managers, every raiser shall be validated without
  stopping at the first invalid one, each invalid one lowers its own class,
  and the proposal stays `pending` with one unavailable note and one
  `raiser_invalid` count. A raiser whose check errored lowers nothing. When
  all raisers are valid, the claim's actor shall be the raiser of the most
  recent raise (class name ascending breaks a tie), and under the lock each
  needed class shall still read `automatic` with the same raise (the same
  class change row), not merely the same user.

## Out of scope

- Raising `move`, `message`, `stop`, `improvement` or merge classes. A
  `starts_agent` `move` therefore stays manual, so the stuck-between-steps
  case is automatic only when it is a `starts_agent` `create_task`; raising
  `move` is its own decision.
- A new route to lower a class: the settings save is the lower.
- Per-class eligibility settings, and workspace-wide defaults.
- GitLab MR-thread wakes (phase 4) and any change to containment. An
  automatic `resume` or `start_agent` is held, not enabled, where the
  coordinator's executor is not isolated or authentication is off.
- Making `resume` or `start_agent` undoable.
- Showing a manager why a raised class is being held (containment, ceiling,
  Pause). The reason reaches the coordinator's tool result and the
  `coordinator_automatic_held_total` metric; the settings page carries only the
  static containment note. A last-hold surface is a later decision.
- A route or page that reads `coordinator_eligibility_changes`.
- Observing the later outcome of a deferred or cold resume launch, or of the
  agent start that follows an automatic `starts_agent` create. They lower
  nothing; a manager lowers explicitly.
- Running a binary older than this amendment against a coordinator that
  stores `resume` or `start_agent` as `automatic` or tuned eligibility: such a
  binary refuses saves and drops the settings. Roll forward only; lower both
  classes first when rolling back.
