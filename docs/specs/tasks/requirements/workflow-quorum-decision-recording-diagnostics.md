---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Quorum diagnostics Requirements



## Overview



A task exposes enough live quorum state to distinguish an unmet threshold from a guard that cannot succeed.

## Requirements


### REQ-TASKS-QUORUM-DIAGNOSTICS-001: Quorum diagnostics



**Intent:** A task exposes enough live quorum state to distinguish an unmet threshold from a guard that cannot succeed.

#### Acceptance criteria

#### Distinguishing waiting from stuck

- **AC-TASKS-QUORUM-DIAGNOSTICS-001.1:** WHEN a guarded transition does not fire, THE SYSTEM SHALL record
  exactly one reason code from this closed set:
  `threshold_not_met`, `slate_empty`, `decision_store_unwired`,
  `participant_store_unwired`, `guard_variant_unrecognized`,
  `threshold_unrecognized`, `threshold_unsatisfiable`, `evaluation_error`, and
  `session_unresolvable`.
  `decision_store_unwired` and `participant_store_unwired` are separate because
  `evaluateWaitForQuorum` SHALL nil-check them separately — today it does not,
  the current code being a single combined
  `if e.decisions == nil || e.participants == nil { return false, nil }`, so
  splitting that branch in two is a REQUIRED change of this feature and not an
  existing property to be relied on. Left unsplit, the two codes are
  indistinguishable in practice and AC-TASKS-QUORUM-DIAGNOSTICS-001.1's taxonomy silently loses a member;
  `threshold_unsatisfiable` carries AC-TASKS-QUORUM-BINDING-001.5, the case where the threshold can
  never be met by any future decision and the card is stuck rather than waiting;
  `evaluation_error` carries a genuine error returned by
  `ListStepParticipants` or `ListStepDecisions`; `session_unresolvable` carries
  AC-TASKS-QUORUM-REEVALUATION-001.5. `threshold_unrecognized` carries AC-TASKS-QUORUM-DIAGNOSTICS-001.8, a threshold string
  the evaluator does not recognize at all. The set is closed rather than "at
  minimum" so that AC-TASKS-QUORUM-DIAGNOSTICS-001.3's expvar keys are enumerable and a new reason cannot
  appear unlabelled. More than one condition can hold at once; AC-TASKS-QUORUM-DIAGNOSTICS-001.7 fixes which
  single code is reported.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.2:** THE SYSTEM SHALL emit, each time a guarded transition does not fire
  **on the engine's transition-evaluation path**, a structured log record
  carrying the task id, step id, guard role,
  threshold, required-participant count, decisions-received count, the AC-TASKS-QUORUM-DIAGNOSTICS-001.1
  reason code, and an `error` field which is populated when and only when the
  reason is `evaluation_error`. The engine holds no logger today, so wiring one
  is part of this feature.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.3:** THE SYSTEM SHALL expose counts of not-fired guard evaluations under
  `/debug/vars`, keyed by the AC-TASKS-QUORUM-DIAGNOSTICS-001.1 reason, following the existing `workflow_*`
  expvar convention documented in the root `CLAUDE.md`. Like AC-TASKS-QUORUM-DIAGNOSTICS-001.2, this counts
  **engine-path evaluations only**: the AC-TASKS-QUORUM-DIAGNOSTICS-001.4 diagnostic endpoint evaluates
  guards read-only and SHALL emit no log record and increment no counter. Stated
  because AC-TASKS-QUORUM-DIAGNOSTICS-001.9 requires that endpoint to evaluate every guard live, so the two
  ACs otherwise overlap: were diagnostic reads counted, these counters would
  measure UI polling rather than workflow health, and a card left open in a
  browser would out-count a genuinely stuck one.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.4:** THE SYSTEM SHALL expose guard state for a task at
  `GET /api/v1/office/tasks/:id/quorum`, a sibling of the existing
  `GET /api/v1/office/tasks/:id/decisions` route
  (`office/dashboard/handler.go`) and carrying the same authorization as that
  route. The response SHALL contain one entry per guarded `on_turn_complete`
  transition configured at the task's current step, each with: `target_step_id`,
  `role`, `threshold`, `required_count`, `received_count`, `satisfied` (a
  boolean), `reason` (an AC-TASKS-QUORUM-DIAGNOSTICS-001.1 code, present if and only if `satisfied` is
  `false` — see AC-TASKS-QUORUM-DIAGNOSTICS-001.10), and `error` (populated only for `evaluation_error`). A
  task at a step with no guarded transition SHALL return an empty list, not a
  404. Entries are computed live per AC-TASKS-QUORUM-DIAGNOSTICS-001.9, which also defines the payload's
  top-level `reevaluation_blocked` field. The payload SHALL be a direct
  projection of the AC-TASKS-QUORUM-REEVALUATION-001.14 snapshot — that is the only path by which this
  handler may reach the slate machinery, since AC-TASKS-QUORUM-REEVALUATION-001.9 forbids it from carrying
  its own copy — preserving AC-TASKS-QUORUM-REEVALUATION-001.14's ordering. AC-TASKS-QUORUM-DIAGNOSTICS-001.11 defines how the card
  renders when entries disagree.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.5:** WHEN the task has no bound `workflow_step_id`, the AC-TASKS-QUORUM-DIAGNOSTICS-001.4 endpoint
  SHALL return an empty list rather than an error, so a diagnostic read never
  fails on the state it exists to diagnose.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.6:** WHEN the task's AC-TASKS-QUORUM-DIAGNOSTICS-001.4 entries aggregate to `awaiting` under AC-TASKS-QUORUM-DIAGNOSTICS-001.11,
  the Office task detail view SHALL render the task as awaiting decisions —
  showing `received_count` of `required_count` for the role of the entry that
  AC-TASKS-QUORUM-DIAGNOSTICS-001.11 selects — and SHALL NOT render it as failed or errored. When they
  aggregate to `stuck`, the view SHALL render a visually distinct stuck state
  naming the selected entry's AC-TASKS-QUORUM-DIAGNOSTICS-001.1 reason. This is the whole diagnostic gap:
  the two states are byte-identical today. AC-TASKS-QUORUM-DIAGNOSTICS-001.11 exists because AC-TASKS-QUORUM-DIAGNOSTICS-001.4 returns a
  LIST and `Office Default`'s Review configures two guards, so "the reason" is
  not well defined without an aggregation rule.

- **AC-TASKS-QUORUM-DIAGNOSTICS-001.7:** WHEN more than one AC-TASKS-QUORUM-DIAGNOSTICS-001.1 condition holds at once, THE SYSTEM SHALL
  report the first that applies in this order, which mirrors the order the code
  discovers them: `guard_variant_unrecognized`, `decision_store_unwired`,
  `participant_store_unwired`, `evaluation_error`, `slate_empty`,
  `threshold_unrecognized`, `threshold_unsatisfiable`, `threshold_not_met`.
  AC-TASKS-QUORUM-DIAGNOSTICS-001.1 requires exactly one code, and AC-TASKS-QUORUM-SLATE-001.6 and AC-TASKS-QUORUM-BINDING-001.5 both fire for an empty
  slate with any positive `n_approve:<N>`, since every positive N exceeds a slate
  of zero. `slate_empty` wins: it is the more actionable diagnosis, and
  `evaluateWaitForQuorum` already returns at `len(required) == 0` before the
  threshold is examined. The ninth code, `session_unresolvable`, is not in this
  ordering because it is not a guard-evaluation outcome at all — it is the
  recording-time skip of AC-TASKS-QUORUM-REEVALUATION-001.5, surfaced through AC-TASKS-QUORUM-DIAGNOSTICS-001.2/AC-TASKS-QUORUM-DIAGNOSTICS-001.3 and, on the
  AC-TASKS-QUORUM-DIAGNOSTICS-001.4 payload, through the separate field AC-TASKS-QUORUM-DIAGNOSTICS-001.9 defines. `slate_empty` is
  reachable only for approve-style thresholds, per AC-TASKS-QUORUM-SLATE-001.6 as scoped and AC-TASKS-QUORUM-SLATE-001.11; an
  `any_reject` guard over an empty slate reports `threshold_not_met`, because a
  seatless rejection can still arrive and so the card is waiting, not stuck.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.8:** WHEN the guard's threshold is a string that is neither one of the
  five recognized thresholds nor prefixed `n_approve:`, THE SYSTEM SHALL not fire
  the transition and SHALL report AC-TASKS-QUORUM-DIAGNOSTICS-001.1 reason `threshold_unrecognized`, which
  AC-TASKS-QUORUM-DIAGNOSTICS-001.6 renders as stuck. `applyThreshold` returns false from its final
  fall-through for such a value today, indistinguishably from
  `threshold_not_met`, so a mistyped threshold would render as "awaiting
  decisions" forever — the exact diagnostic gap this feature exists to close.
  `guard_variant_unrecognized` is reserved for a guard whose variant is not
  `wait_for_quorum` at all, matching `evaluateTransitionGuard`'s fail-closed
  branch.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.9:** THE SYSTEM SHALL compute AC-TASKS-QUORUM-DIAGNOSTICS-001.4's per-guard entries by evaluating
  each guarded `on_turn_complete` transition configured at the task's current
  step **live, at request time, and independently** — not by replaying the
  engine's transition loop, and not by reading a stored evaluation. Independent
  evaluation is required because `evaluateActions` short-circuits, evaluating a
  guard only while `targetStepID == ""`, so a replay cannot enumerate every
  guard the way AC-TASKS-QUORUM-DIAGNOSTICS-001.4 requires. A per-guard `reason` SHALL therefore never be
  `session_unresolvable`, which is a recording-time skip rather than an
  evaluation outcome; the AC-TASKS-QUORUM-DIAGNOSTICS-001.4 payload SHALL instead carry a top-level
  `reevaluation_blocked` boolean, defined by AC-TASKS-QUORUM-DIAGNOSTICS-001.12, and AC-TASKS-QUORUM-DIAGNOSTICS-001.11 SHALL treat it as
  stuck-distinct. The field SHALL be `false` when the task has no decisions at
  its current step, so an untouched task is never rendered as stuck.

- **AC-TASKS-QUORUM-DIAGNOSTICS-001.10:** THE SYSTEM SHALL set an AC-TASKS-QUORUM-DIAGNOSTICS-001.4 entry's `satisfied` to `true` when
  that guard's threshold is met at request time, and SHALL omit `reason`
  entirely for such an entry. AC-TASKS-QUORUM-DIAGNOSTICS-001.1's set is a taxonomy of why a guard did NOT
  fire and SHALL NOT gain a "satisfied" member, so AC-TASKS-QUORUM-DIAGNOSTICS-001.3's expvar keys stay
  enumerable. The state is reachable rather than theoretical: under AC-TASKS-QUORUM-REEVALUATION-001.5 a
  verdict is recorded, re-evaluation is skipped, and the task sits at the step
  with its quorum already met. AC-TASKS-QUORUM-REEVALUATION-001.5 is the whole of it. An earlier draft also
  claimed the AC-TASKS-QUORUM-CONCURRENCY-001.4 abandon as a second route; it is not one, and the claim is
  removed rather than left to send a builder hunting: AC-TASKS-QUORUM-CONCURRENCY-001.4 abandons PRECISELY
  when the task's `workflow_step_id` no longer equals the evaluated step — the
  race winner has already moved it — and AC-TASKS-QUORUM-DIAGNOSTICS-001.4 evaluates at the task's CURRENT
  step, so the abandoned guard is not in the returned list at all.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.11:** THE SYSTEM SHALL aggregate a task's AC-TASKS-QUORUM-DIAGNOSTICS-001.4 entries to exactly one
  card-level state: `stuck` when `reevaluation_blocked` is `true` or when ANY
  entry is unsatisfied with a `reason` other than `threshold_not_met`;
  otherwise `awaiting` when any entry is unsatisfied; otherwise `clear`. When
  the state is `stuck` the SELECTED entry is the first such entry in the step's
  configured `on_turn_complete` action order, matching AC-TASKS-QUORUM-REEVALUATION-001.6's
  first-transition-wins; when `awaiting`, it is the first unsatisfied entry in
  that same order. Stuck outranks awaiting because a stuck guard is the
  actionable diagnosis and MUST NOT be masked by a sibling that is merely
  waiting — a mistyped `n_approve:<N>` reporting `threshold_unsatisfiable`
  beside a healthy `any_reject` reporting `threshold_not_met` is the concrete
  case, and rendering that card as "awaiting decisions" forever is the exact
  defect AC-TASKS-QUORUM-DIAGNOSTICS-001.8 was added to close. Boundary: an EMPTY entry list aggregates to
  `clear`, which is the correct rendering for both a step with no guarded
  transition and the AC-TASKS-QUORUM-DIAGNOSTICS-001.5 unbound-step case — neither is stuck, and neither is
  waiting on a decision.
- **AC-TASKS-QUORUM-DIAGNOSTICS-001.12:** THE SYSTEM SHALL compute `reevaluation_blocked` live at request time
  as the conjunction of: the task has at least one non-superseded decision at
  its current step, AND the AC-TASKS-QUORUM-REEVALUATION-001.4 session query returns no row. It SHALL NOT be
  read from a stored flag.
  The first conjunct SHALL be evaluated as "`ListStepDecisions` returns a
  non-empty list for (task, current step)". That is equivalent, and it is the
  form the engine can actually express: a row is superseded only when a
  replacement is inserted for the same decider, so a step holding at least one
  row always holds at least one non-superseded row. Stating the equivalence
  matters because the engine's `DecisionInfo` projection carries no
  `superseded_at` — the same limitation AC-TASKS-QUORUM-VERDICT-001.4 turns into last-row-wins — so a
  literal reading of "non-superseded" would be a predicate the evaluator cannot
  compute.
  This is a CURRENT-STATE predicate, not a history of AC-TASKS-QUORUM-REEVALUATION-001.5 skips, and the
  difference is deliberate. A stored "the last recording skipped" flag would keep
  rendering a card as stuck after a session returned and the card became healthy,
  and reading a stored evaluation is the very thing AC-TASKS-QUORUM-DIAGNOSTICS-001.9 forbids for the
  per-guard entries. The live predicate also needs no ordering rule, no tiebreak
  and no new column, so the "most recent decision" ambiguity does not arise; two
  concurrent reads compute the same value from committed state.
  Naming: the field is `reevaluation_blocked`, NOT `last_reevaluation_skipped`,
  because it asserts something different from the original and a reader must not
  carry the historical reading across the rename.

## Out of scope



Behavior outside this acceptance-criteria group is defined by the core quorum requirement and the other quorum capability requirements.