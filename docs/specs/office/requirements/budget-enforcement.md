---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office: Pre-Launch Budget Enforcement Requirements

## Overview

[Office: Cost Tracking & Budget Management](costs.md) establishes that budget
policies exist and that `notify_only`, `pause_agent` and `block_new_tasks` are
their actions. It does not say what happens when the mechanism itself is absent,
broken, or measuring the wrong window. All three currently resolve to "launch
the agent anyway" — no evaluator wired returns allowed, an evaluator error
returns allowed behind an explicit `// fail-open on error` comment, and
`office_budget_policies` holds zero rows on the reference install — so an Office
run has no spend ceiling under any circumstance.

Fail-open is right while a human is watching a run start: a transient fault
should not block work somebody is waiting on. That does not extend to an
unattended schedule. A `*/5` routine that fails open on an evaluator error is a
loop with no upper bound on cost, running while nobody is looking, whose only
feedback arrives on a bill.

That argument survives here in one place only. An evaluator that is entirely
**absent** is a deployment fact no run can change, so an attended run launches
against it (`AC-OFFICE-BUDGET-001.6`). An evaluator that **errors** is not: the
error may clear, so every run defers and retries regardless of provenance
(`AC-OFFICE-BUDGET-001.3`), and a fault outliving the retry budget fails the run
rather than launching it, again regardless of provenance
(`AC-OFFICE-BUDGET-001.4`). Deferring is what keeps a transient fault from
blocking attended work; this spec does not fail open on an evaluator error for
anybody.

This capability gives the unattended loop a ceiling enforced **before** launch
and makes the inert states observable instead of silent. Office owns the
contract because the states are defined by Office primitives: the run queue,
reason, outcome and cost ledger. It bounds spend; it does not price models,
change how cost events are produced, or recover an overspent workspace.

The capability spans six documents. This one owns the admission decision;
[Run Provenance Classification](budget-run-provenance.md) the attended/
unattended split every gate here reads; [Budget Admission
Integrity](budget-admission-integrity.md) that the decision sees every
applicable policy and changes nothing on its way to one; [Built-In Default Spend
Ceiling](budget-default-ceiling.md) the bound that ships enabled; [Budget
Measurement Integrity](budget-measurement-integrity.md) the number the ceiling
is compared against, a precondition for both; and [Budget Enforcement
Observability](budget-observability.md) which state fired.

## Prior art

**Receipt, internal wiki leg: NOT RUN, tooling unavailable.** Config resolved via
`@henry` to `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`,
`QMD_WIKI_COLLECTION=wiki`. Every read of that path returns `EPERM` (macOS TCC on
`~/Documents`), confirmed two ways: `index.md` and `hot.md` EPERM while a
nonexistent sibling returns not-found, so the vault is present and the denial is
access control. No `qmd`, `obsidian-wiki` or `obsidian-cli` on PATH, no QMD MCP
tool exposed. Re-run before treating this spec as settled: a recorded standing
position on fail-open gates would outrank the reasoning here.

**Receipt, vendor leg: SUBSTITUTED.** `saas-kb` and its `search_fsm_docs` are not
exposed in this session, so the `ai_sdlc` slice could not be queried; a public web
search ran instead. Vendor claims, not evidence. Self-hosted **OpenHands** ships
no built-in hard cost cap while its cloud product offers exactly the scheduled
runs this spec is about; **Devin** bills opaque ACU consumption with no published
pre-launch ceiling; provider caps are reported as advisory or too coarse to catch
a loop running for hours. The recurring practitioner finding is that
observability is not enforcement. One credible **dissenting** view holds
monitoring should precede hard caps, since a cap can stop a high-priority
workflow midstream — which is why the [built-in
default](budget-default-ceiling.md) gates unattended runs only.

**Receipt, in-repo prior art: READ.** `shared.IsPeriodicTasklessWake`
(`office/shared/runreasons.go`) already classifies provenance from `runs.reason`
with a deliberate safe default for the ambiguous legacy literal; this spec reuses
its shape and inverts its polarity, per the glossary's warning. [Office Stall
Visibility](stall-visibility.md) sets the precedent for a detect-only safety
capability. The Office run-outcome vocabulary
([task-delivery-ledger](../../task-delivery-ledger/spec.md)) already gives each
terminal shape a distinct `runs.outcome`, including `budget_blocked` — so this
spec extends that vocabulary rather than establishing it, correcting the
originating card's claim that the shape is not yet distinguishable.

**What we are doing differently.** Every vendor position found is either "no cap"
or "opaque cap". This spec makes the ceiling explicit, pins its window and
currency, ships it enabled for the unattended path only, and makes each inert
state individually observable rather than collapsed into one boolean.

## Terminology

This capability's shared vocabulary — *run provenance*, *attended run*,
*unattended run*, *subcent*, *priced spend*, *unpriced event*, *pricing-degraded
window*, *evaluator fault*, *evaluation window*, *admission gate*, *applicable
policy*, and the polarity warning about `IsPeriodicTasklessWake` — is defined
once in the [Office glossary](../glossary.md) under **Budget enforcement**, and
is normative for every document of this capability.

## Requirements

### REQ-OFFICE-BUDGET-001: Unattended runs have a pre-launch spend ceiling

**Intent:** Remove the fail-open paths so that no Office run launches without a
ceiling having been evaluated, while keeping a *transient* evaluator fault from
blocking work a person is waiting on — without letting a persistent one launch
an unbounded run.

**User story:** As an operator running Office unattended, I want a schedule to
stop launching when it has spent its ceiling or when the ceiling cannot be
evaluated, so that a runaway loop costs a bounded amount rather than an
unbounded one.

#### Acceptance criteria

- **AC-OFFICE-BUDGET-001.1:** The provenance every criterion below relies on
  shall be the one `REQ-OFFICE-BUDGET-007` in [Run Provenance
  Classification](budget-run-provenance.md) produces, computed once before any
  policy is read. No gate in this document shall recompute or revise it, so two
  gates evaluating the same run cannot disagree about whether it is attended.
- **AC-OFFICE-BUDGET-001.2:** Wherever this document says *attended* or
  *unattended* it means exactly `AC-OFFICE-BUDGET-007.2`'s classification of the
  run's reason, including that criterion's rule that an unrecognized value is
  unattended. No criterion here shall introduce a third state, and none shall
  treat an unclassifiable run as exempt from a gate.
- **AC-OFFICE-BUDGET-001.3:** When the evaluator reports an error, the system
  shall not launch the agent, shall release any task checkout the run holds,
  and shall schedule the run for retry, for both attended and unattended runs.
- **AC-OFFICE-BUDGET-001.4:** When a run scheduled for retry under
  `AC-OFFICE-BUDGET-001.3` has already reached `MaxRetryCount`, the system
  shall fail the run and record an operator-visible activity entry naming the
  evaluator error.
- **AC-OFFICE-BUDGET-001.5:** When no budget evaluator is wired and the run is
  unattended, the system shall not launch the agent, shall cancel the run rather
  than retry or fail it, and shall record an operator-visible activity entry
  stating that budget enforcement is unavailable. An absent evaluator is a
  deployment state and not a transient fault: it cannot resolve without an
  operator changing the deployment, so retrying under
  `AC-OFFICE-BUDGET-001.3` would spend the run's whole retry budget on a
  condition no retry can clear. A cancelled run shall not re-enter the queue, so
  the entries this criterion writes are bounded by the number of runs queued
  rather than by scheduler ticks.
- **AC-OFFICE-BUDGET-001.6:** When no budget evaluator is wired and the run is
  attended, the system shall launch the agent.
- **AC-OFFICE-BUDGET-001.7:** When an applicable policy's action is
  `pause_agent` or `block_new_tasks` and priced spend in its evaluation window
  is greater than or equal to its limit, the system shall not launch the agent
  and shall finish the run with outcome `budget_blocked`.
- **AC-OFFICE-BUDGET-001.8:** When an applicable policy's action is
  `notify_only`, the system shall launch the agent regardless of that policy's
  spend, for both attended and unattended runs, **except where
  `AC-OFFICE-BUDGET-004.3` in
  [budget-measurement-integrity.md](budget-measurement-integrity.md) blocks the
  run**. A `notify_only` action is a statement about a known spend figure; a
  pricing-degraded window has no trustworthy figure for it to be a statement
  about, so `notify_only` does not extend to that case.
- **AC-OFFICE-BUDGET-001.9:** When more than one policy applies, the system
  shall evaluate policies ordered by `created_at` ascending, tie-broken by
  `id` ascending, and shall block on the first policy that satisfies
  `AC-OFFICE-BUDGET-001.7` or `AC-OFFICE-BUDGET-004.3`. Every applicable policy
  shall be evaluated against one captured evaluation instant, so that two
  policies in the same run cannot observe different windows. Gate 5 of
  `AC-OFFICE-BUDGET-001.14` shall evaluate the built-in default against that
  same instant, so a policy and the default cannot fall on opposite sides of a
  UTC midnight within one run. This ordering selects which policy *decides*; it
  never limits which policies are *evaluated*. Every applicable policy shall
  have both its limit test and its pricing-degradation determination completed
  before this ordering selects one, per `AC-OFFICE-BUDGET-006.1`, so blocking on
  the first such policy cannot leave a later one unevaluated. The rule is
  internal to gate 4: the gate sequence of `AC-OFFICE-BUDGET-001.14` still stops
  at the first gate yielding a non-launch decision.
- **AC-OFFICE-BUDGET-001.10:** When two runs are evaluated concurrently against
  the same policy, each shall observe the spend committed at its own evaluation
  instant; the check is an admission test and not a reservation, so the ceiling
  can be exceeded by the cost of the runs already in flight when it was
  crossed. The system shall not hold a lock across launch to prevent this.
- **AC-OFFICE-BUDGET-001.11:** When a run is blocked or deferred by any
  criterion in this capability, the system shall not have launched an agent
  process, minted a runtime token, or assembled a prompt for that run.
- **AC-OFFICE-BUDGET-001.12:** When a run is blocked or deferred by any
  criterion in this capability — including a block by
  `AC-OFFICE-BUDGET-004.3` or by the built-in default — the system shall
  release the task checkout that run holds, scoped to that run as its owner,
  before the run reaches its terminal or retry-scheduled state. A run that
  never held a checkout shall not clear another run's checkout.
- **AC-OFFICE-BUDGET-001.13:** A run carries no workspace identifier of its
  own; its workspace is that of the agent instance the run names. Resolving the
  workspace therefore means resolving that agent instance, and this criterion
  constrains the disposition of that resolution wherever it already happens
  rather than adding a second one. When the workspace cannot be resolved, the
  system shall not launch the agent, for both attended and unattended runs. An
  unresolvable workspace shall not be evaluated as a workspace with no policies
  and no spend, and shall not change the run's provenance. The two ways
  resolution can fail have different dispositions. When the lookup returns an
  error, the run shall be deferred and retried on the same terms as an evaluator
  fault (`AC-OFFICE-BUDGET-001.3`, `AC-OFFICE-BUDGET-001.16`,
  `AC-OFFICE-BUDGET-001.18`, and at `MaxRetryCount`
  `AC-OFFICE-BUDGET-006.4`), an I/O failure being transient. When the lookup
  succeeds and yields no agent instance, or yields one carrying no workspace
  identifier, the run shall be cancelled rather than retried or failed, an
  orphaned run having no ceiling that could ever be evaluated. Each cancellation
  shall record its own operator-visible activity entry, and a cancelled run shall
  not re-enter the queue. Neither disposition shall queue a run for any other
  agent, per `AC-OFFICE-BUDGET-001.17`.
- **AC-OFFICE-BUDGET-001.14:** The admission gates shall be evaluated in this
  order, and the first gate that yields a non-launch decision shall determine
  the run's disposition, with no later gate evaluated: (1) workspace resolution
  (`AC-OFFICE-BUDGET-001.13`); (2) evaluator presence
  (`AC-OFFICE-BUDGET-001.5`, `AC-OFFICE-BUDGET-001.6`); (3) evaluator
  invocation (`AC-OFFICE-BUDGET-001.3`); (4) applicable policies in
  `AC-OFFICE-BUDGET-001.9` order, each tested for limit
  (`AC-OFFICE-BUDGET-001.7`) then for pricing degradation
  (`AC-OFFICE-BUDGET-004.3`); (5) the built-in default
  (`AC-OFFICE-BUDGET-003.1`), tested for limit then for pricing degradation in
  the same order as gate 4, per `AC-OFFICE-BUDGET-003.11`. A run that reaches
  the end of the sequence without a non-launch decision shall launch.
  This ordering is ordinal among *these* gates only: it does not reorder the
  checks Office already performs before launch and this capability does not
  govern, and gate 1 constrains the disposition of the workspace resolution
  wherever that resolution already occurs rather than requiring a second one.
  The run's project identifier is resolved at the start of gate 4, after gate 3
  has succeeded, per `AC-OFFICE-BUDGET-006.7`.
  "Tested for limit then for pricing degradation" at gates 4 and 5 fixes which
  criterion decides the disposition, not what gets computed: the
  pricing-degradation determination of `AC-OFFICE-BUDGET-004.2` shall be made for
  every policy this sequence evaluates, including one whose limit test has
  already decided to block, because `AC-OFFICE-BUDGET-004.6` requires the
  activity entry carry it in exactly that case.
- **AC-OFFICE-BUDGET-001.15:** A policy applies to a run when its scope matches
  that run: a `workspace`-scoped policy applies to every run in its workspace;
  an `agent`-scoped policy applies when its `scope_id` equals the run's agent
  instance identifier; a `project`-scoped policy applies when its `scope_id`
  equals the run's project identifier. When the run carries no project
  identifier, no `project`-scoped policy applies to it; when it carries no
  agent instance identifier, no `agent`-scoped policy applies to it. A policy
  shall never apply by both identifiers being empty. A failure to *determine*
  either identifier is not the same as the run lacking it: for the agent
  instance see `AC-OFFICE-BUDGET-001.13`, for the project
  `AC-OFFICE-BUDGET-006.3`.
- **AC-OFFICE-BUDGET-001.16:** A deferral under `AC-OFFICE-BUDGET-001.3` shall
  increment the same `runs.retry_count` the run's other retry classes use, so
  that `MaxRetryCount` bounds a run's total retries across all causes rather
  than granting evaluator faults their own budget, and shall schedule the retry
  no sooner than the first delay in the existing retry backoff schedule. The
  system shall not requeue a deferred run within the same scheduler tick.
- **AC-OFFICE-BUDGET-001.17:** Failing a run under `AC-OFFICE-BUDGET-001.4`
  shall not queue any new run, and in particular shall not queue an escalation
  run for another agent. A broken evaluator shall not cause an agent launch by
  any path.
- **AC-OFFICE-BUDGET-001.18:** When a run deferred under
  `AC-OFFICE-BUDGET-001.3` is too old to retry under the existing retry
  staleness bound, the system shall cancel it rather than retry or fail it, and
  shall record an operator-visible activity entry naming budget deferral as the
  cause of the cancellation.


### Companion requirements

`REQ-OFFICE-BUDGET-005` (each state individually observable) is specified in
[Budget Enforcement Observability](budget-observability.md), and
`REQ-OFFICE-BUDGET-007` (provenance is a total function of the run's reason) in
[Run Provenance Classification](budget-run-provenance.md), which
`AC-OFFICE-BUDGET-001.1` and `AC-OFFICE-BUDGET-001.2` bind these gates to.
`REQ-OFFICE-BUDGET-006` (the admission decision is complete and inert) is
specified in [Budget Admission Integrity](budget-admission-integrity.md) and
constrains the gates above: a partially evaluated policy set is an evaluator
fault (`AC-OFFICE-BUDGET-006.1`), and reaching any decision here writes no alert
entry and changes no agent's status (`AC-OFFICE-BUDGET-006.2`).

## Out of scope

- **Reserving budget across concurrent launches.** `AC-OFFICE-BUDGET-001.10`
  deliberately admits overshoot bounded by in-flight concurrency; a true
  reservation needs a durable counter decremented at launch and reconciled at
  completion. Named so the overshoot is a decision, not an omission.
- **Duplicate delivery of one run row to two consumers.** That single-consumer
  contract is the existing run-claim mechanism's. `AC-OFFICE-BUDGET-001.10`
  covers two *different* runs, not one run twice.
- **Exactly-once delivery of activity entries and counters.** Every Office expvar
  counter is best-effort; a crash between a block and its increment loses it and
  does not reverse the block.
- **Token-rate circuit breaking.** Watching consumption *rate* to catch a loop
  before it spends its ceiling is a complementary control; this spec bounds total
  spend per window only.
- **Per-run and per-routine ceilings.** A per-window workspace ceiling bounds
  total cost, which is what NFR-5 requires; two more scopes would multiply the
  policy-precedence surface for no additional bound. A future requirement can add
  them within this evaluation model.
- **Distinguishing an agent-caused event wake from a human-caused one.**
  `AC-OFFICE-BUDGET-007.2` classifies `task_comment`, `task_mentioned`,
  `task_children_completed` and `task_blockers_resolved` as attended, though an
  agent can cause all of them. Without a causation identifier the run row cannot
  tell them apart, and classifying them unattended would apply the built-in
  default to the bulk of ordinary event-driven work. A known gap; closing it
  depends on the causation-id work tracked as the runaway-chain card.
- **Repairing the inbox "Mark fixed" action.** `manual_resume_after_failure` is
  attended because only an operator click queues it, but that click is separately
  broken: the agent stays paused and the requeue is rejected and swallowed as a
  warning, so a budget test passing on that reason is not evidence the recovery
  flow works. Card `497b1f63`; not fixed here.
- **Unpausing an agent that a `pause_agent` policy paused.** The existing "mark
  fixed" path does not unpause. A separate known defect.
- **Currencies other than United States dollars.** The storage unit is fixed at
  subcents of USD.
- **Alert thresholds and the post-event evaluation path.**
  `alert_threshold_pct`, the `budget.alert` and `budget.exceeded` entries, and
  the agent-pause side effect of a post-event evaluation remain governed by
  [costs.md](costs.md). This spec constrains only the pre-launch admission
  decision, which `AC-OFFICE-BUDGET-006.2` requires produce none of them, so the
  two paths do not overlap.
- **Retention or pruning of the activity entries this capability writes.**
- **The system design.** The technical path, including where provenance
  classification lives and how the built-in default's setting is stored, belongs
  in a paired system-design document authored first.
