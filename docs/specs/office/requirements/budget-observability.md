---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office: Budget Enforcement Observability Requirements

## Overview

[Office: Pre-Launch Budget Enforcement](budget-enforcement.md) specifies when a
run is admitted against a spend ceiling, and can stop a run for several
distinct reasons. This document owns making which reason fired legible to an
operator. It does not change admission behavior.

The failure this capability exists to remove was silent. A ceiling that stops a
loop without saying which of several things happened reproduces the same
blindness one layer up.

## Terminology

This document uses this capability's shared vocabulary, defined in the [Office
glossary](../glossary.md) under **Budget enforcement** — in particular *run
provenance*, *attended run*, *unattended run*, *evaluator fault*,
*pricing-degraded window*, and *admission gate*.

## Requirements

### REQ-OFFICE-BUDGET-005: Each inert or blocking state is individually observable

**Intent:** The failure this capability exists to remove was silent. A ceiling
that stops a loop without saying which of several things happened reproduces
the same blindness one layer up. `runs.outcome` already distinguishes
`budget_blocked` from the other terminal shapes; the states this spec adds
need the same treatment rather than being folded into it.

#### Acceptance criteria

- **AC-OFFICE-BUDGET-005.1:** A run blocked because a policy limit or the
  built-in default's limit was reached shall carry outcome `budget_blocked`.
  That outcome shall mean the limit was actually reached, and no other state in
  this capability shall use it.
- **AC-OFFICE-BUDGET-005.2:** A run deferred by an evaluator fault
  (`AC-OFFICE-BUDGET-001.3`) shall not carry outcome `budget_blocked`, because
  no ceiling was determined to have been reached.
- **AC-OFFICE-BUDGET-005.3:** Each of the following shall be distinguishable
  from `budget_blocked` and from each other by a field on its activity entry
  that is queryable without parsing free text: a block because no evaluator was
  wired (`AC-OFFICE-BUDGET-001.5`); a deferral because the workspace lookup
  errored and a cancellation because the lookup yielded no workspace, which
  `AC-OFFICE-BUDGET-001.13` gives different dispositions and which shall
  therefore be distinguishable from each other and not collapsed into one
  unresolvable-workspace state; a deferral by evaluator fault
  (`AC-OFFICE-BUDGET-001.3`), distinguishable from the workspace-lookup deferral
  even though the two share a retry path; a cancellation of a stale deferral
  (`AC-OFFICE-BUDGET-001.18`); and a block by a pricing-degraded window
  (`AC-OFFICE-BUDGET-004.3`, in
  [budget-measurement-integrity.md](budget-measurement-integrity.md)). A
  queryable activity-entry field is the uniform mechanism; an outcome value is
  required only where `AC-OFFICE-BUDGET-005.7` names one.
  `AC-OFFICE-BUDGET-006.5` adds five further states to this list, on the same
  terms.
- **AC-OFFICE-BUDGET-005.4:** The system shall expose counters, in the same
  `/debug/vars` surface as the existing Office metrics, for: runs blocked by
  limit, runs deferred by evaluator fault, runs blocked by absent evaluator,
  runs deferred by a workspace-lookup error, runs cancelled as having no
  resolvable workspace, runs cancelled as stale deferrals,
  runs blocked by pricing degradation, runs admitted against the built-in
  default, and runs admitted against a pricing-degraded window
  (`AC-OFFICE-BUDGET-004.8`). Each counter shall be labelled by run provenance,
  using the provenance `AC-OFFICE-BUDGET-007.1` produced. Each counter shall
  count runs, once per run, with one exception: every deferral counter shall
  count deferral *attempts*, matching `AC-OFFICE-BUDGET-005.6`'s
  one-entry-per-attempt rule, so a run deferred four times before failing
  increments it four times. Without that exception stated, the same run could be
  counted once or four times and neither reading would be wrong.
  The counter for runs admitted against a pricing-degraded window shall increment
  only for a run whose final disposition was launch, and not once per
  `AC-OFFICE-BUDGET-004.8` entry. That entry is written per non-blocking degraded
  policy, and a later policy in `AC-OFFICE-BUDGET-001.9`'s order — or the
  built-in default at gate 5 — may still block the run, so counting entries would
  report a blocked run as admitted.
- **AC-OFFICE-BUDGET-005.5:** Every activity entry this capability writes that
  acted on a policy or on the built-in default shall name the policy identifier
  it acted on, or state explicitly that the ceiling was the built-in default.
  An entry written before any ceiling was determined — the evaluator-fault
  deferral, the absent-evaluator cancellation, both workspace-resolution
  outcomes of `AC-OFFICE-BUDGET-001.13`, the stale-deferral cancellation, the
  failed-project-lookup deferral (`AC-OFFICE-BUDGET-006.3`) and a deferral failed
  at `MaxRetryCount` under `AC-OFFICE-BUDGET-006.4`, and both cancellations of
  `AC-OFFICE-BUDGET-006.7` —
  shall instead state explicitly that no ceiling was determined, and shall not
  name a policy. The deferral an unevaluated policy raises
  (`AC-OFFICE-BUDGET-006.1`) is the single exception, and so is its failure at
  `MaxRetryCount`: no ceiling was determined in either, but the policy that could
  not be evaluated is known and shall be named, because it is the only pointer an
  operator has to the row to inspect.
- **AC-OFFICE-BUDGET-005.6:** When the same run is deferred more than once —
  whether by repeated evaluator faults, by repeated workspace-lookup errors
  (`AC-OFFICE-BUDGET-001.13`), or by the faults `AC-OFFICE-BUDGET-006.1` and
  `AC-OFFICE-BUDGET-006.3` raise — the system shall record one activity entry per
  deferral attempt, each naming the attempt number and which cause produced it,
  and shall not coalesce them into a single entry. This rule covers every
  deferral this capability schedules, so no deferral cause is left to a builder's
  choice between one entry per attempt and one per run.
- **AC-OFFICE-BUDGET-005.7:** A run blocked by `AC-OFFICE-BUDGET-004.3` whose
  priced spend did not reach the applicable limit shall carry the
  `runs.outcome` value `budget_unmeasurable`, reserved for a block caused by
  unmeasurable spend rather than by a limit being reached, and shall not carry
  `budget_blocked`.
  Adding that value extends the Office run-outcome vocabulary owned by
  [task-delivery-ledger](../../task-delivery-ledger/spec.md), whose enumeration
  shall be updated in the same change rather than gaining an undeclared value.
  Updating the enumeration alone is not sufficient: that document also states,
  unqualified, that a run blocked by the pre-execution budget check finishes
  with `budget_blocked`, both as an acceptance statement for the Office run
  outcome and as a row in its mapping table from the `checkBudget` code site.
  Both become false for a `budget_unmeasurable` block, and both remain live
  canon. Every statement in that document that maps a pre-execution budget block
  to `budget_blocked` shall be narrowed to the limit-reached case in the same
  change, so no reader of the ledger alone concludes that every budget block
  carries `budget_blocked`. The same applies to any statement in that document
  that fixes the *cardinality* of the enumeration rather than mapping a block to
  a value — it states that the writer writes one of five values on the finished
  path — which this addition also makes false, and which a change that updated
  only the enumeration and the mapping statements would leave stale.

## Out of scope

- **Exactly-once delivery of activity entries and counters.** Every Office
  expvar counter is best-effort by construction; a crash between a block and
  its counter increment loses the increment and does not reverse the block.
  `AC-OFFICE-BUDGET-005.4` requires the counters exist and are labelled, not
  that they are transactional with the run's state change.
- **A dashboard, alert, or digest over these counters and entries.** This
  document requires the states be individually queryable; presenting them is
  governed by [costs.md](costs.md) and the inbox surfaces.
- **Retention or pruning of the activity entries this capability writes.**
- **Renaming or restructuring the existing `budget_blocked` outcome or the
  `run_budget_blocked` activity entry.** `AC-OFFICE-BUDGET-005.1` constrains
  what they mean; it does not rename them.
