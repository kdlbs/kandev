---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office: Budget Measurement Integrity Requirements

## Overview

[Office: Pre-Launch Budget Enforcement](budget-enforcement.md) specifies when a
run is admitted against a spend ceiling. That contract is only as strong as the
number the ceiling is compared against, and today that number is wrong in two
independent ways: a policy's declared period does not select the window it
names, and spend whose price is unknown is counted as zero. Both weaken
enforcement silently, and both must be closed before the daily default ceiling
in [Built-In Default Spend Ceiling](budget-default-ceiling.md) can ship.

This document owns the measurement. It does not change admission behavior,
which stays in the companion documents, and it does not price models.

## Terminology

This document uses this capability's shared vocabulary, defined in the [Office
glossary](../glossary.md) under **Budget enforcement** — in particular
*subcent*, *priced spend*, *unpriced event*, *pricing-degraded window*,
*attended run*, *unattended run*, and *evaluation window*.

## Requirements

### REQ-OFFICE-BUDGET-002: A policy means what it says

**Intent:** The period contract disagrees across four layers. The model enum
declares `daily`, `monthly`, `yearly`, and both handlers validate against it
(`office/costs/handler.go:144`, `:178`). The evaluator recognizes only
`monthly` and returns the zero time otherwise, which its consumers read as "no
filter", so a `daily` or `yearly` policy silently measures **lifetime** spend.
On the reference install lifetime spend is 281,106,390 subcents against a
busiest day of 23,813,441, so a daily policy would be permanently exceeded from
creation, with no error and no log. Meanwhile the create form offers only
`monthly` and `total`, and `total` is not in the enum, so choosing it returns
HTTP 400 — the UI's second of two options cannot save. Backend tests
(`office/costs/budgets_test.go:305`) exercise `total`, a value the product
rejects. A shipped daily default is unsafe until this is closed.

The same requirement also covers policy *validity*, because a malformed policy
disarms the ceiling by the same silent mechanism a wrong window does.
`limit_subcents` carries no CHECK constraint, so a non-positive limit is
storable, and an `agent`- or `project`-scoped policy with an empty `scope_id`
matches every run whose corresponding identifier is also empty. Criteria
`AC-OFFICE-BUDGET-002.6`, `-002.7` and `-002.11` close the write path and the
rows stored before it closed. They belong to this requirement rather than a
separate one because the failure mode is identical: a policy that appears to
bound spend and does not.

#### Acceptance criteria

- **AC-OFFICE-BUDGET-002.1:** When a policy's period is `daily`, its evaluation
  window shall start at the most recent UTC midnight at or before the
  evaluation instant.
- **AC-OFFICE-BUDGET-002.2:** When a policy's period is `monthly`, its
  evaluation window shall start at 00:00:00 UTC on the first day of the
  evaluation instant's UTC month.
- **AC-OFFICE-BUDGET-002.3:** When a policy's period is `yearly`, its evaluation
  window shall start at 00:00:00 UTC on 1 January of the evaluation instant's
  UTC year.
- **AC-OFFICE-BUDGET-002.4:** Every evaluation window shall be computed in UTC,
  and shall not shift with the host's local time zone or its daylight-saving
  transitions.
- **AC-OFFICE-BUDGET-002.5:** When a policy carries a period value the build
  does not recognize, the system shall not evaluate that policy as a lifetime
  window; it shall skip the policy and record an operator-visible activity
  entry naming the policy and the unrecognized value. A skipped policy shall
  not supersede the built-in default under `AC-OFFICE-BUDGET-003.4`, so
  skipping the only workspace-scoped `daily` policy restores the built-in
  ceiling rather than removing every bound.
- **AC-OFFICE-BUDGET-002.6:** When a policy is created or updated with a limit
  less than or equal to zero, the system shall reject the write with a
  validation error rather than storing a policy that blocks every run.
- **AC-OFFICE-BUDGET-002.7:** When a policy is created or updated with scope
  `agent` or `project` and an empty scope identifier, the system shall reject
  the write with a validation error, rather than storing a policy that matches
  every run whose corresponding identifier is also empty.
- **AC-OFFICE-BUDGET-002.8:** `total` shall be a declared period value whose
  evaluation window is unbounded below, so that the lifetime window is
  selected by name rather than by falling through from an unrecognized value.
- **AC-OFFICE-BUDGET-002.9:** Every period value the policy-management surface
  offers shall be accepted by the write API, and every declared period value
  shall be offered by that surface. Selecting any offered period and saving
  shall create a policy rather than return a validation error.
- **AC-OFFICE-BUDGET-002.10:** Priced spend for an evaluation window shall be
  summed over cost events whose `office_cost_events.occurred_at` falls in that
  window, not over an insertion or ingestion timestamp, so that a delayed write
  is counted against the period in which the spend occurred. `occurred_at` is
  `NOT NULL` in the stored schema, so no null-timestamp branch is required. A
  cost event whose `occurred_at` is after the evaluation instant shall be
  excluded from that evaluation, so a clock-skewed or back-dated write cannot
  be counted against a window that has not begun.
- **AC-OFFICE-BUDGET-002.11:** When a stored policy carries a limit less than or
  equal to zero, the system shall skip that policy and record an
  operator-visible activity entry naming the policy and its limit, in the same
  manner as `AC-OFFICE-BUDGET-002.5`, rather than blocking every run in the
  workspace. A policy skipped under this criterion shall not supersede the
  built-in default under `AC-OFFICE-BUDGET-003.4`, on the same reasoning
  `AC-OFFICE-BUDGET-002.5` states: a workspace-scoped `daily` policy carrying a
  blocking action and a non-positive limit would otherwise satisfy that
  supersession test while being skipped itself, leaving the workspace with no
  unattended ceiling at all — the exact failure this capability exists to
  remove. `AC-OFFICE-BUDGET-002.6` prevents new such rows; this criterion
  covers rows stored before it shipped.
- **AC-OFFICE-BUDGET-002.12:** A test shall fail the build when the set of
  period values the policy-management surface offers differs from the set the
  write API declares. The test shall derive both sets by reading their
  respective sources rather than comparing against a list maintained alongside
  them, so that adding a period value on either side cannot silently reproduce
  the defect `AC-OFFICE-BUDGET-002.9` exists to fix. The check shall live in the
  backend Go test suite, so that `make -C apps/backend test` is its single gate
  rather than the parity being split across two pipelines and enforced by
  neither. It shall derive the declared set from the `BudgetPeriod` enum's own
  validation, and the offered set by reading the policy-creation form component
  under `apps/web/app/office/workspace/costs/`, and shall fail on any value
  present in one set and absent from the other.
- **AC-OFFICE-BUDGET-002.13:** An activity entry required by
  `AC-OFFICE-BUDGET-002.5`, `AC-OFFICE-BUDGET-002.11` or
  `AC-OFFICE-BUDGET-002.14` shall be recorded at most once per policy per UTC
  day, rather than once per evaluation, so that a frequently scheduled routine
  re-evaluating one malformed policy does not write an unbounded stream of
  identical entries. The bound is per policy in aggregate, not per violated
  criterion: a policy violating more than one of those criteria shall produce one
  entry naming every violation detected, not one entry per violation. The bound
  is best-effort under concurrency, on the same reasoning as the counters — two
  runs evaluating the same policy concurrently may each observe that no entry
  exists yet and each write one, because `AC-OFFICE-BUDGET-001.10` forbids
  holding a lock across the decision. A duplicate entry is acceptable; this
  bound exists to stop a per-evaluation stream, not to guarantee uniqueness.
- **AC-OFFICE-BUDGET-002.14:** When a stored policy carries a scope type the
  build does not recognize, an action the build does not recognize, or a scope
  of `agent` or `project` with an empty scope identifier, the system shall skip
  that policy and record an operator-visible activity entry naming the policy
  and the offending field, in the same manner as `AC-OFFICE-BUDGET-002.5`, and
  that policy shall not supersede the built-in default under
  `AC-OFFICE-BUDGET-003.4`. Skipping is the disposition for every stored policy
  the build cannot evaluate as written; together with `AC-OFFICE-BUDGET-002.5`
  and `AC-OFFICE-BUDGET-002.11` this makes that rule total over the policy row,
  so no malformed field is left to a code path that silently treats the policy
  as bounding nothing. For the empty-scope-identifier case this is the
  observable form of an exclusion `AC-OFFICE-BUDGET-001.15` already requires:
  the two agree that the policy does not bound the run, and this criterion adds
  the operator-visible record. `AC-OFFICE-BUDGET-002.6` and
  `AC-OFFICE-BUDGET-002.7` prevent new such rows; this criterion covers rows
  stored before they shipped and rows written by any path that bypasses the
  write API. A policy skipped here is not an unevaluated policy under
  `AC-OFFICE-BUDGET-006.1`, whose disposition could not be decided; this one's
  is decided here.

- **AC-OFFICE-BUDGET-002.15:** A policy's evaluation shall select the cost
  events it measures by scope, and the same selection shall serve both the
  priced-spend sum of `AC-OFFICE-BUDGET-004.1` and the pricing-degradation
  determination of `AC-OFFICE-BUDGET-004.2`. A `workspace`-scoped policy, and
  the built-in default under `AC-OFFICE-BUDGET-003.11`, shall select every cost
  event incurred by an agent instance belonging to that workspace — the same
  derivation `AC-OFFICE-BUDGET-001.13` uses to give a run its workspace.
  Selection shall not depend on the continued existence of the event's task: an
  event carrying no task identifier, or one whose task row has since been
  deleted, shall still count against its workspace. Otherwise deleting a task
  would retroactively lower spend already incurred, and a taskless periodic wake
  — the unattended shape this capability most exists to bound — would be exempt
  from the ceiling. An `agent`-scoped policy shall select events whose agent
  instance equals its `scope_id`. A `project`-scoped policy shall select events
  by the project recorded on the cost event itself when the spend occurred, not
  by the current project of the event's task, so that reparenting a task does
  not move historical spend between ceilings. Where an event cannot be
  attributed to the scope at all it is outside that policy's measurement and is
  not an unevaluated policy under `AC-OFFICE-BUDGET-006.1`, which concerns a
  policy the system could not evaluate rather than an event it did not select.

### REQ-OFFICE-BUDGET-004: Unpriced spend cannot silently raise the ceiling

**Intent:** When no price can be resolved for a turn's tokens, Office records
the cost event with `cost_subcents = 0` and `cost_source = 'unpriced'`
(`office/service/prompt_usage_cost.go`, the three `CostSourceUnpriced`
branches of `resolveCostForUsage`), so tokens whose price is unknown are
invisible to every sum the ceiling is computed from. Enforcement therefore
weakens exactly when pricing breaks. Note that `cost_source` is the
price-resolution flag and is deliberately distinct from `estimated`, which is a
usage-authority flag. The two are independent, and the code is explicit about
it: `resolveCostForUsage` copies `estimated` from the reported usage on every
branch, the three unpriced branches included, so an unpriced row can carry
`estimated` either way and neither flag implies the other. Degradation must
therefore be determined from `cost_source`, never from `estimated`. The reference install
shows this is real but rare — 3 unpriced events in 4,672 — which is why the
rule below reacts to unpriced events only as the ceiling is approached, rather
than blocking on a single stray row.

#### Acceptance criteria

- **AC-OFFICE-BUDGET-004.1:** The ceiling comparison shall use priced spend, in
  subcents, denominated in United States dollars.
- **AC-OFFICE-BUDGET-004.2:** When evaluating a policy, the system shall also
  determine whether the evaluation window is pricing-degraded, counting only
  events whose `cost_source` is `unpriced` and treating a NULL `cost_source` as
  priced. The determination shall not read the `estimated` column.
- **AC-OFFICE-BUDGET-004.3:** When the evaluation window is pricing-degraded,
  priced spend is greater than or equal to 50 percent of the policy limit, and
  the run is unattended, the system shall not launch the agent. The 50 percent
  comparison shall be exact and shall not be computed by halving the limit with
  integer division: the threshold is reached when twice the priced spend is
  greater than or equal to the limit. Truncating an odd subcent limit would move
  the threshold down by half a subcent and fire the criterion early,
  deterministically and invisibly. This is the comparison every criterion in this
  document means by "50 percent of the policy limit". This criterion
  applies whatever the policy's action, and overrides
  `AC-OFFICE-BUDGET-001.8`'s admission of a `notify_only` policy: a
  `notify_only` action expresses a judgement about a known spend figure, and a
  pricing-degraded window has no trustworthy figure for it to be a judgement
  about.
- **AC-OFFICE-BUDGET-004.4:** When the evaluation window is pricing-degraded and
  priced spend is below 50 percent of the policy limit, this criterion shall not
  block the run, and the system shall record that the window was
  pricing-degraded under `AC-OFFICE-BUDGET-004.8`. Whether the run launches is
  then determined by the remaining gates of `AC-OFFICE-BUDGET-001.14`: a later
  policy in `AC-OFFICE-BUDGET-001.9`'s order, or the built-in default at gate 5,
  may still block it.
- **AC-OFFICE-BUDGET-004.5:** The system shall not substitute an assumed,
  averaged, or interpolated price for an unpriced event when computing spend.
- **AC-OFFICE-BUDGET-004.6:** A pricing-degraded block shall be distinguishable
  from a limit-exceeded block by a queryable field on the activity entry, per
  `AC-OFFICE-BUDGET-005.3`. Where the blocked run's priced spend did not reach
  the policy limit, it shall additionally carry the distinct run outcome
  required by `AC-OFFICE-BUDGET-005.7`; where the limit was also reached, the
  outcome is `budget_blocked` and the degradation is carried by the activity
  entry alone.
- **AC-OFFICE-BUDGET-004.7:** When the evaluation window is pricing-degraded,
  priced spend is greater than or equal to 50 percent of the policy limit, and
  the run is **attended**, this criterion shall not block the run, and the
  system shall record that the window was pricing-degraded under
  `AC-OFFICE-BUDGET-004.8`. Pricing degradation shall never block an attended
  run. Whether the run launches is determined by the remaining gates of
  `AC-OFFICE-BUDGET-001.14`; in particular a limit reached under
  `AC-OFFICE-BUDGET-001.7` still blocks the run at gate 4, which tests limit
  before degradation.

- **AC-OFFICE-BUDGET-004.8:** The record required by `AC-OFFICE-BUDGET-004.4`
  and `AC-OFFICE-BUDGET-004.7` shall be an operator-visible activity entry
  carrying the same queryable field `AC-OFFICE-BUDGET-005.3` uses to distinguish
  a pricing-degraded block, set to a value distinct from that block's, so an
  admitted degraded window and a blocked one cannot be confused by a query. One
  entry shall be written per policy whose window was pricing-degraded and which
  did not block, naming that policy identifier, or stating that the ceiling was
  the built-in default. It shall be recorded at most once per policy per UTC
  day, in the manner of `AC-OFFICE-BUDGET-002.13`, so that a frequently
  scheduled routine running in a persistently degraded window does not write an
  unbounded stream of identical entries.

## Out of scope

- **Admission behavior.** Whether a run launches, defers, or is blocked is
  specified in [Office: Pre-Launch Budget Enforcement](budget-enforcement.md);
  the criteria here that stop a run do so by feeding that document's gates.
- **Pricing lookup itself.** How a model's rate is resolved, and the
  `models.dev` cache behavior, remain governed by [costs.md](costs.md).
- **Repairing the unpriced write path.** Nothing here changes how an unpriced
  cost event is recorded; `cost_subcents = 0` with `cost_source = 'unpriced'`
  remains the stored shape, and this document only stops that shape from
  silently raising a ceiling.
- **Backfilling historical rows.** The reference install holds 1,135 cost
  events with a NULL `cost_source`, written before the column existed. They are
  treated as priced and are not re-derived.
- **Migrating existing policies.** No requirement here rewrites a stored policy
  whose period is currently interpreted as lifetime, or whose limit is
  non-positive; the operator-visible entries in `AC-OFFICE-BUDGET-002.5` and
  `AC-OFFICE-BUDGET-002.11` are the migration signal.
- **Making the 50 percent degradation threshold configurable.** It is a
  constant. A future requirement can make it a policy field within the same
  evaluation model.
