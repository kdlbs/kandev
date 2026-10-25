---
status: draft
system: office
created: 2026-08-25
owners:
  - kandev
---

# Step Entry Sequence Execution Requirements

## Overview

A workflow step can declare a sequence of actions to run when a task arrives at
it. The engine compiles that sequence. In production it is never dispatched:
every route by which a task arrives converges on a handler that executes a
small set of session-shaped actions inline and never asks the engine to handle
a step-entry trigger. A step that declares anything else has its declaration
silently ignored.

This requirement is separated from the capability that discovered it
(`review-participant-seats.md`, which needs a participant seat written before a
step's fan-out reads the slate) because the contract is not Office's. It
governs how every workflow's declared entry sequence is executed, and the
blast radius of getting it wrong reaches every shipped workflow: four of the
compiled action kinds are already executed inline today, so a naive dispatch
runs them twice and launches or prompts agents a second time.

The requirement is stated once, here, and the capability that depends on it
references it rather than assuming it.

## Terminology

- **Arrival** - one committed change of a task's workflow step. A step change
  that is rolled back is not an arrival, and re-issuing a move to the step the
  task already stands on is not an arrival, because the step does not change.
  Creating a task directly into a step is an arrival. A change that leaves the
  task standing at no step at all - detaching it from its workflow - is not an
  arrival, because the task enters nothing. The route by which the change was
  requested does not enter the definition.
- **Step entry** - one arrival together with the system executing that step's
  declared entry sequence for it. A task that leaves a step and later arrives
  at it again has made a **second, distinct entry**; each entry runs the entry
  sequence again. Entry is the unit that at-most-once criteria are counted
  over.
- **Entry identity** - the value that distinguishes one step entry from
  another for deduplication. Two deliveries carrying the same entry identity
  are the same entry; two arrivals at the same step carrying different entry
  identities are different entries even when every other field matches.
- **Redelivery** - a second dispatch of an entry that already ran, carrying
  that entry's identity. A retry, a replayed event or a crash-recovery
  re-dispatch is a redelivery, not a new entry.

## Requirements

### REQ-OFFICE-STEP-ENTRY-001: A declared entry sequence executes

Every capability that declares work on step entry assumes that work runs when a
task arrives. That assumption does not hold today, so it is a requirement
rather than a premise.

#### Acceptance criteria

- **AC-OFFICE-STEP-ENTRY-001.1:** Every arrival of a task at a step shall
  execute every action that step declares for step entry, in declared order,
  including actions whose effect is on participants, decisions or queued runs
  rather than on the arriving session, and including arrivals at which the task
  has no live session. No route by which a task's workflow step changes shall
  be exempt.
- **AC-OFFICE-STEP-ENTRY-001.2:** That execution shall carry an entry identity,
  so that a redelivery of the same entry is recognized as such and any
  at-most-once criterion has a defined unit to be counted over.
- **AC-OFFICE-STEP-ENTRY-001.3:** A step whose entry sequence declares no
  action, or only actions already handled today, shall behave exactly as it
  does now.
- **AC-OFFICE-STEP-ENTRY-001.4:** When one action in an entry sequence fails,
  the failure shall be recorded and the remaining actions shall still be
  attempted, except where another acceptance criterion states otherwise.
- **AC-OFFICE-STEP-ENTRY-001.5:** Each action a step declares for step entry
  shall take effect exactly once per arrival. An action the existing step-entry
  handling already executes shall not additionally be executed by the execution
  path this requirement introduces.
- **AC-OFFICE-STEP-ENTRY-001.6:** The continue-on-error behavior of
  AC-OFFICE-STEP-ENTRY-001.4 shall apply to step entry only. Every other
  trigger shall keep the abort-on-first-failure behavior it has today.
- **AC-OFFICE-STEP-ENTRY-001.7:** The entry identity of
  AC-OFFICE-STEP-ENTRY-001.2 shall be fixed by the same write that commits the
  step change, not by a separate count-then-use read. Two dispatches of one
  arrival shall carry the same identity, two arrivals of the same task at the
  same step shall carry different identities, and no identity shall be assigned
  for a step change that does not commit.
- **AC-OFFICE-STEP-ENTRY-001.8:** Exactly one component shall execute a step's
  declared entry sequence for a given arrival. No other production path shall
  additionally execute that sequence for the same arrival, including the path
  that moves a task between workflows, and a test shall assert this over the
  routes by which a task can arrive rather than over the kinds of action a step
  can declare.
- **AC-OFFICE-STEP-ENTRY-001.9:** The execution shall be synchronous with the
  write that commits the step change, running after that write commits and
  before the committing caller returns, so that an observer who has seen the
  step change has also seen the entry sequence run. An execution lost because
  the process failed between the commit and the call shall not be recovered;
  such a task is recovered by arriving again.
- **AC-OFFICE-STEP-ENTRY-001.10:** Recognition of a redelivery shall rest on
  durable state keyed by entry identity, not on state held only for the
  lifetime of a process. A redelivery arriving after a restart shall still be
  recognized as a redelivery.

## Out of scope

Each exclusion below is a contract, not an omission.

- **Every trigger other than step entry.** `on_turn_start`,
  `on_turn_complete` and `on_exit` keep the dispatch and the failure semantics
  they have today. This requirement exists so that declared entry work runs; it
  is not a licence to redesign how the workflow engine is driven.
- **New action kinds.** This requirement governs the execution of the kinds the
  engine already compiles for step entry. A capability introducing a new kind
  owns that kind's own contract.
- **Retroactively running entry sequences for tasks already standing at a
  step.** A task that has already arrived has already had its entry sequence
  executed or skipped; it is recovered by arriving again. There is no sweep.
- **Changing what a step declares.** Which actions a workflow's steps declare
  is workflow configuration, owned by whichever capability or operator writes
  it.
- **Recovering an execution lost to a process failure.** AC-OFFICE-STEP-ENTRY-001.9
  trades that guarantee away deliberately, for a synchronous call with no
  queue, no table and no polling interval. The window is the duration of one
  function call, and the recovery is the same as for every other stranded task
  above: arrive again.
