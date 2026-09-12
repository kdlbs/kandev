---
status: draft
system: office
specification_version: 1
owners:
  - kandev
---

# Office glossary

Shared vocabulary for Office specifications. Terms are grouped by the capability
that defines them; a capability's requirement documents reference this file
rather than restating a definition, so one term cannot drift between documents.

## Budget enforcement

Defined by [Office: Pre-Launch Budget Enforcement](requirements/budget-enforcement.md)
and used by [Budget Admission
Integrity](requirements/budget-admission-integrity.md), [Built-In Default Spend
Ceiling](requirements/budget-default-ceiling.md), [Budget Measurement
Integrity](requirements/budget-measurement-integrity.md) and [Budget Enforcement
Observability](requirements/budget-observability.md).

- **Run provenance:** the classification of a run as *attended* or
  *unattended*, derived from `runs.reason`.
- **Attended run:** a run whose reason is in the attended allowlist
  (`AC-OFFICE-BUDGET-007.2`). A person plausibly caused it and is plausibly
  waiting on it.
- **Unattended run:** any run that is not attended, **including any reason the
  allowlist does not recognize**.
- **Subcent:** one hundredth of a United States cent, i.e. `1 USD = 10_000`
  subcents. Already the storage unit of `office_cost_events.cost_subcents` and
  `office_budget_policies.limit_subcents`.
- **Priced spend:** the sum of `cost_subcents` over the cost events a policy's
  scope selects (`AC-OFFICE-BUDGET-002.15`) and whose `occurred_at` falls in the
  evaluation window, which by construction counts an unpriced event as zero.
- **Unpriced event:** a cost event with `cost_source = 'unpriced'`. A NULL
  `cost_source` is **not** an unpriced event — it is a legacy row written
  before the cost contract added the column, and it carries a real cost.
- **Pricing-degraded window:** an evaluation window containing at least one
  unpriced event.
- **Evaluator fault:** the pre-execution budget evaluator returning a non-nil
  error. A missing evaluator is a distinct state, not a fault.
- **Evaluation window:** the half-open interval `[cutoff, now)` a policy's
  period selects, with `cutoff` computed in UTC.
- **Admission gate:** one of the five ordered checks in
  `AC-OFFICE-BUDGET-001.14` that can stop a run before launch.
- **Applicable policy:** a policy selected for a run by
  `AC-OFFICE-BUDGET-001.15`.

> **Polarity warning for implementers.** `IsPeriodicTasklessWake` classifies
> the ambiguous legacy reason `routine_dispatch` as **non**-periodic, because
> for the idle-skip gate the safe answer is "do not skip". For budget
> enforcement the safe answer is the opposite: the same literal is
> **unattended**, because the safe answer here is "do not spend". Reusing
> `IsPeriodicTasklessWake` for provenance would invert this and fail open on
> exactly the legacy cron rows it is meant to catch.
