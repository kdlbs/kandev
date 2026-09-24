---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office Launch Budgets Requirements

## Overview

[Office Unattended Launch Safety](unattended-launch-safety.md) bounds how many Office
agent processes may run **at one instant**. This document bounds how many may start
**over time**. A ceiling limits parallelism; it does not stop a loop launching
serially forever, and the two are enforced against different sources: a ceiling counts
live occupancy, a budget counts history.

The two requirements here ship together because the second cannot be built without the
first. `runs.claimed_at` is cleared whenever a run is retried or recovered, so a budget
counted from it silently loses exactly the launches a runaway loop produces. The
durable launch record exists to give the budget something honest to count, and it has
no other consumer.

These requirements keep the `REQ-OFFICE-LAUNCH-SAFETY-002` and
`REQ-OFFICE-LAUNCH-SAFETY-005` identifiers they were authored under. The identifiers
are stable so that acceptance criteria cited elsewhere keep resolving; only the file
they live in changed, when the limits document reached its size ceiling and the
instant-versus-time split turned out to be the honest seam.

Office owns this contract for the same reason it owns the limits: the launch is a run
row transition, and Office owns that row.

## Terminology

- **Launch:** the transition of a run row from `queued` to `claimed`, as defined by
  [Office Unattended Launch Safety](unattended-launch-safety.md).
- **Launch budget:** the maximum number of launches permitted for one scope within a
  rolling time window.
- **Launch record:** one durable, append-only row recording that a launch happened.
- **Unreadable gate input**, **deferral** and **refusal** are defined by
  [Office Unattended Launch Safety](unattended-launch-safety.md) and used here with
  that meaning.

## Requirements

### REQ-OFFICE-LAUNCH-SAFETY-002: Durable launch record

**Intent:** Make the number of launches in a window a fact that survives the run
row's later transitions. `claimed_at` is cleared whenever a run is retried or
recovered, so a count taken from it silently loses exactly the launches a runaway
loop produces.

**User story:** As an operator, I want the launch count for a period to reflect
every process that actually started, so that a crash-looping agent cannot hide its
launches from the budget by being retried.

#### Acceptance criteria

- **AC-OFFICE-LAUNCH-SAFETY-002.1:** When a run transitions to `claimed`, the
  system shall append one durable launch record naming the run identifier, the
  workspace, the causation identifier, the routine the run is attributable to (or
  empty when none), whether the run is human-rooted, and the claim timestamp.
- **AC-OFFICE-LAUNCH-SAFETY-002.2:** A launch record shall never be modified or
  deleted by a retry, a recovery sweep, a routing re-dispatch, a run's completion,
  or a run's failure.
- **AC-OFFICE-LAUNCH-SAFETY-002.3:** When one run is claimed more than once,
  because it was retried or recovered between claims, the system shall append one
  launch record per claim, so that repeated launches of the same run are counted as
  the repeated launches they are.
- **AC-OFFICE-LAUNCH-SAFETY-002.4:** The claim transition and its launch record
  append shall occur in one transaction. When the append fails, the claim shall be
  rolled back and the run shall remain `queued`.
- **AC-OFFICE-LAUNCH-SAFETY-002.5:** The launch budgets in
  REQ-OFFICE-LAUNCH-SAFETY-005 shall be counted from these records, never from
  `runs.claimed_at`.
- **AC-OFFICE-LAUNCH-SAFETY-002.6:** Launch records older than the longest
  configured budget window shall be eligible for deletion. Deleting a record can
  only permit more launches, never fewer, so pruning shall never be able to
  silently tighten a limit.

### REQ-OFFICE-LAUNCH-SAFETY-005: Launch budget per period

**Intent:** Bound launch volume over time, not just at an instant. A ceiling limits
how many run at once; it does not stop a loop launching serially forever.

**User story:** As an operator, I want a cap on agent launches per workspace and per
routine per hour, so that an unattended loop has a spend ceiling in time as well as
in parallelism.

#### Acceptance criteria

- **AC-OFFICE-LAUNCH-SAFETY-005.1:** The system shall enforce a workspace launch
  budget defaulting to `120` launches per rolling `60` minute window, and a
  per-routine launch budget defaulting to `20` launches per rolling `60` minute
  window, each overridable by operator configuration. When a configured budget
  resolves to a value less than `1`, the system shall use the documented default for
  that budget, log the rejected value at warn level, and start normally. A configured
  `0` shall not mean unlimited, on the same reasoning as
  AC-OFFICE-LAUNCH-SAFETY-001.5.
- **AC-OFFICE-LAUNCH-SAFETY-005.2:** A launch shall be counted against a budget at
  the moment the run transitions to `claimed`, using the launch record required by
  REQ-OFFICE-LAUNCH-SAFETY-002 and its claim timestamp as the window position.
- **AC-OFFICE-LAUNCH-SAFETY-005.3:** When a budget is exhausted, the system shall
  defer the run as defined in AC-OFFICE-LAUNCH-SAFETY-001.7 rather than refusing or
  failing it, so that the queued work survives until capacity returns.
- **AC-OFFICE-LAUNCH-SAFETY-005.4:** A run that is deferred for budget shall not
  have its retry count incremented and shall not be counted as a failure.
- **AC-OFFICE-LAUNCH-SAFETY-005.5:** When a budget counter is unreadable as defined
  in Terminology, the system shall defer the run and record the failure as in
  AC-OFFICE-BACKPRESSURE-003.3. This is deliberately the opposite of the existing
  pre-execution cost budget check, which proceeds on a read error.
- **AC-OFFICE-LAUNCH-SAFETY-005.6:** A run that is human-rooted, as defined by
  AC-OFFICE-RUN-CAUSATION-001.13, shall be exempt from the workspace and routine
  launch budgets at every depth in its chain, and shall still be subject to every
  ceiling in REQ-OFFICE-LAUNCH-SAFETY-001. Priority class shall not be used as a
  proxy for this test.
- **AC-OFFICE-LAUNCH-SAFETY-005.7:** A run shall be counted against the per-routine
  budget when, and only when, it carries a routine attribution as defined by
  AC-OFFICE-RUN-CAUSATION-001.14. A run carrying none shall be exempt from the
  per-routine budget and shall remain subject to the workspace budget and to every
  ceiling.
- **AC-OFFICE-LAUNCH-SAFETY-005.8:** A rolling window shall include a prior launch
  when its claim timestamp is strictly later than the window start, so a launch
  exactly at the boundary has already left the window. Every rolling window and every
  age comparison defined by this document and its siblings, including the self-trigger
  window and the promotion age of AC-OFFICE-BACKPRESSURE-002.1, shall use one declared
  clock, the same one that stamps the launch record, and shall apply that same strict
  boundary. Application time and database time shall not both be used, so two gates
  cannot disagree about whether an instant is inside a window.

## Out of scope

- **Whether a run may be claimed at all.** The ceilings, the depth limit and the
  self-trigger suppression are owned by
  [Office Unattended Launch Safety](unattended-launch-safety.md). A budget here can
  only defer a run that every gate there already admitted.
- **Launch-record retention policy.** AC-OFFICE-LAUNCH-SAFETY-002.6 makes records
  prunable and safe to prune in one direction; the schedule and horizon are separate
  work.
- **Per-workspace configured overrides.** Counting scope is per workspace, but the
  numeric values come from instance-level configuration. A stored per-workspace
  override is a later change.
- **Fail-closed cost budget check.** The existing pre-execution cost budget proceeds
  when its checker errors. Changing that is separate work.
  AC-OFFICE-LAUNCH-SAFETY-005.5 governs only the launch-count budgets defined here,
  so the two checks differ on purpose: an unreadable cost check risks overspend a
  human can see and reverse, while an unreadable concurrency check risks the
  unbounded fan-out this capability exists to prevent.
- **Cost as a budget dimension.** These budgets count launches, not money. A spend
  ceiling is the cost budget's contract, not this one's.
- **Operator notification channels.** Counters and durable operator-visible records
  are required by
  [Office Launch Backpressure](launch-backpressure.md); who is paged, on what
  channel, within what time, is separate.
