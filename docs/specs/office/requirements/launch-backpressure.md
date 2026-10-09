---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office Launch Backpressure Requirements

## Overview

[Office Unattended Launch Safety](unattended-launch-safety.md) and
[Office Launch Budgets](launch-budgets.md) decide whether a run may be claimed. This
document decides which claimable run goes first when capacity is scarce, and how a
launch that was blocked is reported.

Today the claim query orders by `requested_at` alone, with no tiebreak and no
notion of importance, so under saturation a cron fire can outrank a human approval
purely by being queued first, and two rows queued in the same millisecond have no
defined order at all. Nothing reports which gate blocked a launch, so a saturated
pool and a quiet one look identical from outside.

These are three separable capabilities and they are stated as three requirements
so they can be built and verified independently: ordering is a property of the
claim query, promotion is a property of waiting time, and attribution is a
reporting subsystem that observes both. Ordering is the only one the other two
depend on.

## Terminology

- **Priority class:** the coarse bucket that decides which queued run is claimed
  first when capacity is scarce.
- **Gate:** any one of the ceiling, depth and self-trigger checks defined by
  [Office Unattended Launch Safety](unattended-launch-safety.md) and the launch
  budgets defined by [Office Launch Budgets](launch-budgets.md), each of which can
  independently prevent a launch.
- **Refusal gate:** a gate evaluated at enqueue, which prevents a run row existing.
- **Deferral gate:** a gate evaluated at claim, which leaves an existing run `queued`.
  A queued run has passed every refusal gate, so the two sets are disjoint at the
  moment a deferral is attributed.
- **Actor** and **human-rooted** are defined by
  [Office Run Causation Chain](run-causation-chain.md) and used here with that
  meaning.

## Requirements

### REQ-OFFICE-BACKPRESSURE-001: Deterministic claim order

**Intent:** Make scarce capacity go to human-blocked work first and periodic work
last, in an order that is total and repeatable rather than dependent on row
arrival.

**User story:** As an operator, I want scarce capacity to go to human-blocked work
first and periodic work last, in a predictable order, so that saturation degrades
legibly.

#### Acceptance criteria

- **AC-OFFICE-BACKPRESSURE-001.1:** Every queued run shall resolve to exactly one
  priority class, ordered from most to least preferred: `human`, `recovery`,
  `event`, `periodic`.
- **AC-OFFICE-BACKPRESSURE-001.2:** The claim order shall be priority class
  ascending, then `runs.requested_at` ascending, then `runs.id` ascending. The
  `runs.id` tiebreak shall always be applied, so that the claim order is total and
  repeatable for any two rows.
- **AC-OFFICE-BACKPRESSURE-001.3:** Priority class shall be decided by exactly one
  rule, applied in this order and stopping at the first match: a human actor yields
  `human`; a wake reason that only a human can cause yields `human`; a run re-queued
  by retry, recovery, or routing re-dispatch yields `recovery`; otherwise the wake
  reason decides. No run shall be evaluated against a later rule once an earlier one
  has matched.
- **AC-OFFICE-BACKPRESSURE-001.4:** Every wake-reason constant the system defines,
  including reasons retained only to read historical rows, shall be enumerable from
  one declared registry. The system shall have a test that resolves every reason in
  that registry and fails when any of them resolves through the unmapped fallback of
  AC-OFFICE-BACKPRESSURE-001.6. Resolving to `event` by an explicit rule passes;
  resolving to `event` by falling through does not, which is what keeps the mapping
  from silently falling behind the reason set while leaving the fallback reachable for
  a reason that is not in the registry at all.
- **AC-OFFICE-BACKPRESSURE-001.5:** The mapping shall assign `periodic` only to
  reasons that represent an unattended, schedule-driven fire. A reason whose
  originating trigger cannot be recovered from the persisted row shall not be
  assigned `periodic`, matching the existing rule by which such a row is not treated
  as a skippable periodic wake.
- **AC-OFFICE-BACKPRESSURE-001.6:** When a wake reason maps to no known priority
  class, the system shall assign it `event` and increment a counter labelled with
  the unmapped reason, rather than dropping or reordering the run.
- **AC-OFFICE-BACKPRESSURE-001.7:** Priority class shall be persisted on the run at
  enqueue. When a run is re-queued by retry, the recovery sweep, or routing
  re-dispatch, the system shall re-stamp its class to `recovery` unless it is
  already `human`, so that the class a run is claimed on always reflects
  AC-OFFICE-BACKPRESSURE-001.3 rather than the class it was first queued with.
- **AC-OFFICE-BACKPRESSURE-001.8:** The registry shall be the single place a wake
  reason is declared, and the system shall have a test that fails when a wake-reason
  constant is declared outside it. Enumerating the reason set by listing the files
  that happen to declare reasons is not sufficient: that list has already been wrong,
  and a reason declared in an unlisted file would defeat
  AC-OFFICE-BACKPRESSURE-001.4 while the test still passed.

### REQ-OFFICE-BACKPRESSURE-002: Age-based priority promotion

**Intent:** Stop a low-priority run from waiting forever behind a steady stream of
higher-priority work, without letting waiting time manufacture human priority.

**User story:** As an operator, I want work that has been waiting a long time to
gain ground on newer, more important work, so that a saturated pool does not starve
its lowest class indefinitely.

#### Acceptance criteria

- **AC-OFFICE-BACKPRESSURE-002.1:** When a run has been queued for strictly longer
  than `15` minutes, the system shall promote it by exactly one priority class for
  ordering purposes. A run queued for exactly the promotion period shall not yet be
  promoted, matching the strict boundary and single declared clock required by
  AC-OFFICE-LAUNCH-SAFETY-005.8. Queued age shall be measured from
  `runs.requested_at`. The promotion period shall be overridable by operator
  configuration, and a configured value less than `1` minute shall be replaced by the
  default and logged at warn level.
- **AC-OFFICE-BACKPRESSURE-002.2:** Promotion shall never place a run in the `human`
  class. `recovery` is the highest class a promoted run can reach, so a `recovery`
  run's promotion is a no-op and a `human` run's promotion is a no-op.
- **AC-OFFICE-BACKPRESSURE-002.3:** Promotion shall change ordering only. It shall
  not exempt a run from any ceiling, budget, depth, or self-trigger gate, and shall
  not alter the class persisted on the run.
- **AC-OFFICE-BACKPRESSURE-002.4:** Promotion shall be applied at most once to a
  given run, so that waiting time cannot accumulate a run through several classes.
- **AC-OFFICE-BACKPRESSURE-002.5:** A run re-queued by retry, the recovery sweep, or
  routing re-dispatch shall keep its original `requested_at`, so its queued age
  continues to accrue across the re-queue rather than restarting. Such a run is
  re-stamped `recovery` by AC-OFFICE-BACKPRESSURE-001.7, and promotion from `recovery`
  is a no-op under AC-OFFICE-BACKPRESSURE-002.2, so carrying the older timestamp
  cannot promote it into `human`.

### REQ-OFFICE-BACKPRESSURE-003: Gate observability and deferral attribution

**Intent:** Make a blocked launch visible and attributable to the gate that blocked
it. A deferred run is byte-identical to a run that is merely waiting its turn, so
without this a saturated pool, a misconfigured ceiling, and a failing gate are
indistinguishable from a quiet system.

**User story:** As an operator, I want to see which gate is holding launches back
and how often, so that I can tell a working limit from a broken one without reading
the database.

#### Acceptance criteria

- **AC-OFFICE-BACKPRESSURE-003.1:** When an enqueue is refused by a gate, the system
  shall increment a counter labelled by that gate and emit a structured log entry
  naming the gate, the agent profile the wake was for, the wake reason, and the causing
  run identifier the request supplied. A refusal creates no run row, so the entry shall
  not name a run identifier; it shall name the causation identifier only when causation
  was resolved before the refusing gate ran, which it is not for the missing workspace
  of AC-OFFICE-RUN-CAUSATION-001.20. A field unavailable at the point of refusal shall
  be omitted rather than fabricated, defaulted, or filled with an identifier the system
  has not assigned. A refusal is decided per request, so it is always attributable
  exactly.
- **AC-OFFICE-BACKPRESSURE-003.2:** The gates shall have a defined precedence for
  attribution purposes, so that a run blocked by more than one gate is always
  attributed to the same gate. The precedence shall be a stated order over named
  gates, not the order predicates happen to appear in a query. There shall be two
  such orders, because the two kinds of decision see disjoint gate sets: a **refusal**
  precedence over the enqueue gates, and a **deferral** precedence over the claim
  gates. A refusal gate shall not appear in the deferral precedence: a run that exists
  in `queued` has by construction already passed every enqueue gate, so listing one
  there would name a gate that can never be the cause.
- **AC-OFFICE-BACKPRESSURE-003.3:** When a gate cannot be evaluated because an input
  cannot be read, the system shall increment a distinct counter labelled by that
  gate, so that a gate failing closed is distinguishable from a gate that is
  correctly blocking work.
- **AC-OFFICE-BACKPRESSURE-003.4:** Attribution shall never gate a launch. A failure
  to determine which gate blocked a run shall leave the claim decision unchanged and
  shall not defer, refuse, or admit any run.
- **AC-OFFICE-BACKPRESSURE-003.5:** When the same gate has failed closed on `3`
  consecutive evaluations, the system shall write a durable operator-visible record
  naming the gate, the workspace, and the consecutive-failure count. The threshold
  shall be overridable by operator configuration, and a configured value less than `1`
  shall be replaced by the default and logged at warn level.
- **AC-OFFICE-BACKPRESSURE-003.6:** When a claim attempt returns no run while
  eligible queued runs exist, the system shall attribute the deferral for the
  highest-priority queued run under the **effective** claim order: the order of
  AC-OFFICE-BACKPRESSURE-001.2 with the age promotion of
  REQ-OFFICE-BACKPRESSURE-002 applied, which is the order the claim query itself
  uses. Ranking by the persisted class alone would name a different run than the one
  that would actually have gone next, precisely when promotion is active. A tick that
  claims some runs and then stops on a saturated gate reaches this criterion on its
  first empty attempt, so partial saturation is attributed and not silently dropped.
- **AC-OFFICE-BACKPRESSURE-003.7:** A deferral is decided by a set-based claim query
  that selects no row rather than by a per-row verdict, so deferrals shall be counted
  per claim attempt rather than per blocked run: whenever a claim attempt returns no
  row while eligible queued runs exist, the system shall increment a counter labelled
  by the gate that AC-OFFICE-BACKPRESSURE-003.6 attributes, and emit the same
  structured log entry. Counting per blocked run is explicitly not required, because
  the claim query cannot report one.
- **AC-OFFICE-BACKPRESSURE-003.8:** The consecutive-failure count of
  AC-OFFICE-BACKPRESSURE-003.5 shall be held per `(workspace, gate)` pair and shall
  survive a process restart, so that a gate failing across a restart loop still
  escalates. It shall be reset by one successful evaluation of that same gate, and by
  nothing else. A restart, an empty queue, an evaluation of a
  different gate, and a claim that succeeded on some other gate's row shall all leave
  the count unchanged, so that a gate which is genuinely stuck cannot have its
  escalation deferred indefinitely by unrelated activity.
- **AC-OFFICE-BACKPRESSURE-003.9:** The record of AC-OFFICE-BACKPRESSURE-003.5 shall
  be written at most once per hour per `(workspace, gate)` pair, not once per
  workspace. Keyed on the workspace alone, one failing gate's record would suppress a
  second, independently failing gate in the same workspace for up to an hour, which
  inverts the purpose of the record.
- **AC-OFFICE-BACKPRESSURE-003.10:** A gate is **successfully evaluated** when the
  system obtained its input, whether or not the gate then permitted the launch. A
  readable gate that blocks a launch is a success for the purposes of
  AC-OFFICE-BACKPRESSURE-003.8, because that count exists to detect a gate that cannot
  be read rather than one that is correctly holding work back; a saturated pool shall
  therefore never escalate as a failing gate. A claim attempt whose statement completes
  without error successfully evaluates every deferral gate that statement contains, so
  the set-based query is not required to report per-gate outcomes. A gate not evaluated
  in an attempt shall have its count left unchanged, which is the rule
  AC-OFFICE-BACKPRESSURE-003.8 already states for unrelated activity.
- **AC-OFFICE-BACKPRESSURE-003.11:** The attribution of AC-OFFICE-BACKPRESSURE-003.6 is
  a diagnostic snapshot taken after the claim attempt and is not serialized with it, so
  concurrent activity may change occupancy, or claim the selected run, between the two.
  Attribution shall therefore be best-effort: the system shall not be required to name
  the blocking gate exactly under concurrency, and a test shall not assert exact
  attribution against a concurrent claim. This does not weaken
  AC-OFFICE-BACKPRESSURE-003.4. Attribution never gates a launch, so a stale label
  costs one misleading counter increment and never a denied or admitted run.

## Out of scope

- **Whether a run may be claimed at all.** Owned by
  [Office Unattended Launch Safety](unattended-launch-safety.md). This document
  assumes the gates exist and orders the runs that pass them.
- **What the actor is.** Owned by
  [Office Run Causation Chain](run-causation-chain.md). This document consumes the
  actor and the human-rooted flag; it does not define or derive either.
- **Preemption.** Nothing here stops or reorders a run that is already `claimed`. A
  higher-priority arrival waits for a slot rather than taking one.
- **Per-workspace fair sharing.** The order here is global within an instance. A
  scheme that guarantees each workspace a share of a saturated pool is a different
  contract and is not implied by the workspace ceiling.
- **Operator notification channels.** This document requires counters and durable
  records. Who is paged, on what channel, within what time, is separate.
- **Starvation bounds.** AC-OFFICE-BACKPRESSURE-002.1 makes a waiting run gain
  ground; it does not promise an upper bound on wait time under sustained
  saturation. A guaranteed bound needs admission control, not ordering.
