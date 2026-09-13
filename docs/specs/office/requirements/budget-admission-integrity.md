---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office: Budget Admission Integrity Requirements

## Overview

[Office: Pre-Launch Budget Enforcement](budget-enforcement.md) specifies which
admission gate stops a run and what disposition the run gets. That contract
rests on two properties its own criteria never state: that every applicable
policy was actually evaluated, and that evaluating them changed nothing.
Neither holds in the code this capability replaces.

The pre-launch evaluator iterates the applicable policies and, when one
policy's spend query fails, logs the failure and continues, returning a partial
result set and no error. Its caller therefore cannot distinguish "no policy
blocked this run" from "the policy that would have blocked it was never
evaluated". Separately, the per-policy evaluation is not a query: on the same
pre-launch call path it writes `budget.alert` and `budget.exceeded` activity
entries and can flip an agent to `paused` — side effects
[costs.md](costs.md) owns and which
[budget-enforcement.md](budget-enforcement.md)'s Out of scope disclaims.

This document owns those two properties, completeness and inertness, together
with the two identifier-resolution failures that reach the same fail-open
outcome by a different route. It does not decide which gate blocks a run; that
stays in the companion document.

## Terminology

This document uses this capability's shared vocabulary, defined in the [Office
glossary](../glossary.md) under **Budget enforcement** — in particular
*attended run*, *unattended run*, *evaluator fault*, *applicable policy*,
*admission gate*, *evaluation window*, and *priced spend*.

- **Admission evaluation:** the whole of the work the five admission gates of
  `AC-OFFICE-BUDGET-001.14` perform for one run, from provenance classification
  through to the launch or non-launch decision.
- **Unevaluated policy:** an applicable policy for which the system could not
  produce both a priced-spend figure and a pricing-degradation determination.
  A policy the system deliberately declines to evaluate under
  `AC-OFFICE-BUDGET-002.5`, `-002.11` or `-002.14` is **not** an unevaluated
  policy: those criteria decide its disposition, whereas this term names a
  policy whose disposition could not be decided. The term names an *attempted*
  evaluation that failed, never one the system chose not to begin: because
  `AC-OFFICE-BUDGET-006.1` requires every applicable policy be attempted, "not
  yet reached" is not a state an applicable policy can be left in, and a policy
  the ordering of `AC-OFFICE-BUDGET-001.9` did not select is therefore not an
  unevaluated policy.

## Requirements

### REQ-OFFICE-BUDGET-006: The admission decision is complete and inert

**Intent:** A ceiling that is evaluated against some of the applicable policies
is not a ceiling, and an admission test that mutates state is not a test. Both
failures are silent, and both survive every criterion in the companion
documents, because those criteria specify what each gate decides and never
specify that the inputs to that decision were all present or that reaching it
cost nothing.

The completeness half matters most on the path this capability exists to fix.
`REQ-OFFICE-BUDGET-001`'s intent is that no Office run launches without a
ceiling having been evaluated; a partial result set admits the run while
reporting no error at all, so the promise fails without any observable symptom.
The inertness half matters because `AC-OFFICE-BUDGET-001.9` stops at the first
blocking policy. Any side effect produced per policy therefore starts depending
on how far down the ordered list the admission decision happened to walk, which
would make an operator-visible alert and an agent's paused state a function of
policy creation order rather than of spend.

**User story:** As an operator running Office unattended, I want the ceiling to
be computed from every policy that applies and to change nothing on its way to a
decision, so that a launch means the ceiling held rather than that part of it
went missing.

#### Acceptance criteria

- **AC-OFFICE-BUDGET-006.1:** When any applicable policy selected under
  `AC-OFFICE-BUDGET-001.15` is an unevaluated policy, the system shall treat
  that as an evaluator fault and dispose of the run under
  `AC-OFFICE-BUDGET-001.3`, and shall not decide admission from the policies it
  did evaluate. Admitting a run on a partial set reproduces the fail-open this
  capability exists to remove, because the unevaluated policy may be the one
  that blocks; and because the failure is per policy rather than per call, it
  produces no error for `AC-OFFICE-BUDGET-001.3` to fire on unless this
  criterion makes one. Completeness therefore fixes the evaluation model: the
  system shall attempt every applicable policy before any of them decides
  admission, and shall apply `AC-OFFICE-BUDGET-001.9`'s first-in-order selection
  only to a fully evaluated set. Without that two-phase reading the two criteria
  contradict each other — stopping at the first blocking policy would leave a
  later erroring policy undetected, which is exactly the partial set this
  requirement exists to reject, while treating an unvisited policy as
  unevaluated would make every multi-policy run a fault. This lifts to the
  policy loop the pattern `AC-OFFICE-BUDGET-001.14` already fixes within a
  single policy, where the degradation determination is made even for a policy
  whose limit test has already decided to block.
- **AC-OFFICE-BUDGET-006.2:** The admission evaluation shall not write a
  `budget.alert` or a `budget.exceeded` activity entry, and shall not change any
  agent's status. Those side effects belong to the post-event evaluation path
  governed by [costs.md](costs.md), which continues to produce them on its own
  schedule, unchanged and unconstrained by this capability. This criterion holds
  whatever the admission outcome: in particular, an applicable policy whose
  action is `pause_agent` and whose limit is reached shall block the run under
  `AC-OFFICE-BUDGET-001.7` **without** pausing the agent as part of that
  decision. The agent's pause remains the post-event path's to perform when the
  next cost event lands. Stated so the change is a decision rather than a
  side effect of `AC-OFFICE-BUDGET-001.9`'s stop at the first blocking policy,
  which would otherwise make the set of alerts fired and agents paused depend on
  how many policies the admission walk happened to visit.
- **AC-OFFICE-BUDGET-006.3:** When the run's project identifier cannot be
  determined because its lookup failed, the system shall treat that as an
  evaluator fault and dispose of the run under `AC-OFFICE-BUDGET-001.3`, rather
  than evaluating the run as one that carries no project identifier. A failed
  lookup and a run that genuinely has no project are different states:
  `AC-OFFICE-BUDGET-001.15` makes every `project`-scoped policy inapplicable to
  the second, so applying that rule to the first lets a transient failure
  silently remove a whole class of ceiling with no operator-visible symptom. A
  run carrying no task, or a task that resolves successfully to no project, is
  the second state and is not a fault. `AC-OFFICE-BUDGET-006.7` classifies every
  outcome of the resolution, including the two this criterion's error/no-project
  split does not by itself decide.
- **AC-OFFICE-BUDGET-006.4:** When a run deferred by any cause this capability
  introduces other than the evaluator error itself — the workspace-lookup error
  branch of `AC-OFFICE-BUDGET-001.13`, an unevaluated policy
  (`AC-OFFICE-BUDGET-006.1`), or a failed project lookup
  (`AC-OFFICE-BUDGET-006.3`) — has already reached `MaxRetryCount`, the system
  shall fail the run and record an operator-visible activity entry naming that
  cause. The entry shall not name an evaluator error.
  `AC-OFFICE-BUDGET-001.4` is written for a run scheduled for retry by
  `AC-OFFICE-BUDGET-001.3` and requires its entry name the evaluator error,
  which is false for all three; because those three defer *under*
  `AC-OFFICE-BUDGET-001.3`'s terms, without this criterion each reaches
  `MaxRetryCount` with a disposition that is either unstated or mislabelled.
  They reach it on the ordinary path rather than an exotic one: the retry
  backoff schedule exhausts `MaxRetryCount` well before the staleness bound
  `AC-OFFICE-BUDGET-001.18` applies, so a sustained outage arrives here and
  never there.
- **AC-OFFICE-BUDGET-006.7:** The run's project identifier shall be resolved at
  the start of gate 4 of `AC-OFFICE-BUDGET-001.14`, after gate 3 has succeeded,
  so that a run whose evaluator errors and whose project lookup would also fail
  has one determinate disposition rather than one depending on which lookup the
  implementation happens to run first. Resolution has four outcomes and each has
  a stated disposition, mirroring the error/empty split
  `AC-OFFICE-BUDGET-001.13` makes for the workspace. (a) The payload is
  well-formed and carries no task identifier, or the task resolves and carries
  no project: the run genuinely has no project, no `project`-scoped policy
  applies to it under `AC-OFFICE-BUDGET-001.15`, and this is not a fault.
  (b) The lookup returns an error: an evaluator fault under
  `AC-OFFICE-BUDGET-006.3`, deferred and retried. (c) The payload cannot be
  parsed: the system shall not read this as outcome (a), because a payload it
  cannot parse is no evidence that no task identifier was present; it shall
  cancel the run rather than retry or fail it, and record an operator-visible
  activity entry naming the unparseable payload. (d) The lookup succeeds and the
  named task does not exist: the system shall likewise cancel the run, naming
  the missing task. Cases (c) and (d) are permanent conditions no retry can
  clear, so deferring them would spend the run's whole retry budget on something
  that cannot resolve — the reasoning `AC-OFFICE-BUDGET-001.5` already applies
  to an absent evaluator. A run cancelled here shall not re-enter the queue.
- **AC-OFFICE-BUDGET-006.5:** Each state this requirement introduces shall be
  distinguishable, by the queryable activity-entry field
  `AC-OFFICE-BUDGET-005.3` establishes, from `budget_blocked` and from every
  other state that criterion enumerates: an evaluator fault raised by an
  unevaluated policy (`AC-OFFICE-BUDGET-006.1`), one raised by a failed project
  lookup (`AC-OFFICE-BUDGET-006.3`), and a deferral failed at `MaxRetryCount`
  under `AC-OFFICE-BUDGET-006.4`, itself distinguishable by which of that
  criterion's three causes produced it. The first two share
  `AC-OFFICE-BUDGET-001.3`'s disposition with the evaluator fault proper and
  shall still be distinguishable from it and from each other, because the three
  have different operator remedies. An entry concerning an unevaluated policy —
  whether the deferral under `AC-OFFICE-BUDGET-006.1` or its failure under
  `AC-OFFICE-BUDGET-006.4` — shall name the policy it could not evaluate; every
  other entry named here shall state that no ceiling was determined, per
  `AC-OFFICE-BUDGET-005.5`. The two cancellations of `AC-OFFICE-BUDGET-006.7` —
  an unparseable payload (c) and a task that does not exist (d) — are further
  states on these same terms, distinguishable from each other and from the
  failed-lookup deferral of (b), because an operator repairs a malformed payload
  and a deleted task differently.
- **AC-OFFICE-BUDGET-006.6:** The system shall expose counters for each state in
  `AC-OFFICE-BUDGET-006.5`, in the same `/debug/vars` surface and under the same
  labelling and counting rules as `AC-OFFICE-BUDGET-005.4` — which means the two
  deferral counters count deferral *attempts* rather than runs, under that
  criterion's rule for every deferral counter, while a run failed at
  `MaxRetryCount` increments one counter per cause named in
  `AC-OFFICE-BUDGET-006.4`, not one counter shared across the three, each
  counting runs once per run. Sharing one counter would satisfy this criterion
  while defeating `AC-OFFICE-BUDGET-006.5`'s requirement that the causes stay
  distinguishable. The two cancellations of `AC-OFFICE-BUDGET-006.7` likewise
  carry one counter each, counting runs once per run.

## Out of scope

- **Changing the post-event evaluation path.** `AC-OFFICE-BUDGET-006.2` removes
  two side effects from the *admission* path only. When `budget.alert` and
  `budget.exceeded` fire, what `alert_threshold_pct` means, and when a
  `pause_agent` policy pauses an agent all remain governed by
  [costs.md](costs.md), on the post-event path, unchanged.
- **Unpausing an agent paused by the post-event path.** A known separate defect,
  named in [budget-enforcement.md](budget-enforcement.md)'s Out of scope and not
  fixed here.
- **Retrying an unevaluated policy in isolation.** `AC-OFFICE-BUDGET-006.1`
  defers the whole run rather than re-evaluating the one policy that failed.
  A per-policy retry inside a single admission decision would need its own
  budget and its own bound, and would sit inside the gate sequence
  `AC-OFFICE-BUDGET-001.14` fixes.
- **Distinguishing which underlying failure made a policy unevaluated.** A
  timeout, a closed database and a malformed stored row all produce one state
  here. The activity entry names the policy, per `AC-OFFICE-BUDGET-006.5`; it
  does not classify the cause.
- **Retention or pruning of the activity entries this capability writes.**
- **The system design for this capability.** Where completeness is enforced and
  how the admission path is separated from the post-event path belong in the
  paired system-design document authored before implementation.
