---
status: draft
system: office
created: 2026-09-16
owners:
  - kandev
---

# Office Self-Triggered Launch Suppression Requirements

## Overview

An Office agent can act in ways that wake an Office agent: it creates a task, sets an
assignment, writes a comment, or asks the runtime for a run outright. When the agent
it wakes is itself, nothing today stops that cycle from repeating.

This document bounds how often an agent may wake itself. It was carried out of
[Office Unattended Launch Safety](unattended-launch-safety.md), which still owns the
concurrency ceilings and the causation-depth limit and whose Overview names the
siblings that own the rest of this capability. The requirement kept its identifiers
across the move: `REQ-OFFICE-LAUNCH-SAFETY-004` and every `AC-OFFICE-LAUNCH-SAFETY-004.x`
mean here exactly what they meant there, so a citation from code, a design document or
a review finding resolves unchanged.

A self-trigger carve-out already exists in this repository for agent-authored
comments, and AC-OFFICE-LAUNCH-SAFETY-004.2 extends it rather than replacing it. The
two prior-art legs attempted for this capability, and what each of them searched, are
recorded in
[Office Unattended Launch Safety](unattended-launch-safety.md#prior-art); both were
unavailable rather than empty, and that receipt covers this document too.

Depth and self-trigger are genuinely separate bounds and separating the documents
makes that legible: depth stops a chain that keeps descending, this stops a chain that
keeps returning to the same agent at whatever depth. An agent that wakes itself once
per hour forever never exceeds any depth limit.

## Terminology

**Launch**, **refusal**, **deferral**, **unreadable gate input**, and **authoritative
enqueue API** are defined by
[Office Unattended Launch Safety](unattended-launch-safety.md) and are used here with
that meaning. **Causation chain**, **causation depth**, **actor** and **human-rooted**
are defined by [Office Run Causation Chain](run-causation-chain.md). **Wake reason**
and the registry that declares it are defined by
[Office Launch Backpressure](launch-backpressure.md).

- **Self-caused wake:** a wake whose persisted actor is an agent and whose actor
  identifier is the agent profile the wake would start. Both parts of that pair are
  required; AC-OFFICE-LAUNCH-SAFETY-004.7 states why.

## Requirements

### REQ-OFFICE-LAUNCH-SAFETY-004: Self-triggered launch suppression

**Intent:** Prevent an agent waking itself in a tight cycle. One carve-out exists
already: an agent's own comment does not wake that agent as assignee. Every other
self-directed path has no such guard.

**User story:** As an operator, I want an agent's own actions not to restart that
same agent unchecked, so that a single agent cannot spin on itself.

#### Acceptance criteria

- **AC-OFFICE-LAUNCH-SAFETY-004.1:** When a wake would be caused by an action whose
  actor is the same agent profile as the agent to be woken, the system shall treat
  that wake as caused by the actor's causing run and shall apply causation depth
  accordingly.
- **AC-OFFICE-LAUNCH-SAFETY-004.2:** The existing behavior by which an agent's own
  comment does not wake that agent as assignee shall be retained unchanged.
- **AC-OFFICE-LAUNCH-SAFETY-004.3:** The **per-reason** self-trigger allowance shall
  default to `3` and be overridable by operator configuration. When an allowance of
  `N` self-caused wakes for the same agent profile and the same wake reason have
  already been queued within a rolling `60` minute window, the system shall refuse the
  next such wake and every later one until the window has passed, and shall record the
  refusal as in AC-OFFICE-LAUNCH-SAFETY-003.5. An allowance of `N` permits `N` wakes
  in a window and refuses the `N+1`th. A configured allowance less than `1` shall be
  replaced by the default and logged at warn level. Because this allowance is keyed on
  the wake reason, an enqueue an agent requests through a runtime action shall name a
  reason that is a member of the declared registry of AC-OFFICE-BACKPRESSURE-001.4,
  not free text of its own choosing; one naming a reason outside it, the empty string
  included, shall be rejected with a distinguishable error naming the constraint. That
  rejection happens at the runtime-action boundary before the authoritative enqueue is
  called, so it is not one of the refusal gates ordered by
  AC-OFFICE-LAUNCH-SAFETY-003.10, records no idempotency key, and consumes neither
  allowance. It binds the agent-facing action only, leaving the unmapped-reason
  fallback of AC-OFFICE-BACKPRESSURE-001.6 reachable for reasons the system itself
  records and for historical rows.
- **AC-OFFICE-LAUNCH-SAFETY-004.8:** A second, **reason-independent** allowance shall
  bound the total self-caused wakes for one agent profile in the same rolling `60`
  minute window, whatever reasons they name. It shall default to `8`, be overridable
  by operator configuration, and a configured value less than `1` shall be replaced by
  the default and logged at warn level. When it is spent, the system shall refuse the
  next self-caused wake for that profile and every later one until the window has
  passed, recording the refusal as in AC-OFFICE-LAUNCH-SAFETY-003.5 and naming which
  of the two allowances refused it. The two are independent bounds evaluated in a
  fixed order, AC-OFFICE-LAUNCH-SAFETY-004.3 first and this one second, so a wake over
  both is refused once and recorded as a per-reason refusal. A total configured below
  the per-reason value is valid and shall be honored as configured, making the
  per-reason allowance unreachable; the system shall log that at warn level where the
  two are resolved rather than silently raising either. This allowance exists because
  the per-reason one is keyed on a value the requesting agent selects, so an agent
  varying its reason across the registry would otherwise multiply its own allowance by
  the size of the registry.
- **AC-OFFICE-LAUNCH-SAFETY-004.7:** Both self-trigger windows shall be counted from
  persisted run rows, matching the woken agent profile and the persisted actor of
  AC-OFFICE-RUN-CAUSATION-001.19 whose actor kind is `agent` and whose actor
  identifier is that same agent profile, positioned in the window by
  `runs.requested_at`. The per-reason window of AC-OFFICE-LAUNCH-SAFETY-004.3 matches
  the wake reason in addition; the total window of AC-OFFICE-LAUNCH-SAFETY-004.8
  matches no reason at all, so the two differ in that one predicate and nothing else.
  The kind is part of the match, so an identifier shared by a user and an agent
  profile cannot be counted as self-caused. Neither window shall be counted by joining
  a run to its parent's agent profile, which reports a two-agent cycle as
  non-self-caused and fails when the parent is pruned, and neither shall be held in
  process memory, which would reset the allowance on restart and would not be shared
  between processes.
- **AC-OFFICE-LAUNCH-SAFETY-004.9:** When either self-trigger count is an unreadable
  gate input, the system shall refuse the wake rather than queue it, and shall record
  the failure as described in AC-OFFICE-BACKPRESSURE-003.3. A shutdown-cancelled
  evaluation shall still refuse the wake, because a refused enqueue is refused whatever
  cancelled it, but shall not be recorded as a gate failure, so a restart is not
  mistaken for a gate failing closed. This is the same fail-closed direction the
  ceilings take in AC-OFFICE-LAUNCH-SAFETY-001.8 and the opposite of the adjacent cost
  budget; the asymmetry is deliberate and is not to be later reconciled. It differs
  from `001.8` in outcome only because these are refusal gates and those are deferral
  gates: an unreadable ceiling leaves a run queued for the next tick, while an
  unreadable allowance has no later tick to fall back on.
- **AC-OFFICE-LAUNCH-SAFETY-004.4:** A wake whose actor is a human user shall never
  be treated as self-caused, regardless of which agent is woken.
- **AC-OFFICE-LAUNCH-SAFETY-004.5:** Both self-trigger windows shall count wakes that
  were queued, whatever their later status, so that a self-trigger loop cannot reset
  its own allowance by completing or failing quickly. A wake that was refused shall
  not be counted by either. A wake merged into an existing run by coalescing shall be
  counted once by each, against the surviving run; AC-OFFICE-LAUNCH-SAFETY-003.10
  orders coalescing before these and every other refusal gate, so a request that
  merges is never refused for an allowance it does not consume.
- **AC-OFFICE-LAUNCH-SAFETY-004.6:** When the acting agent has more than one run in
  flight, the causing run shall be the run inside which the action was performed,
  taken from the runtime context that carried the action. When the action did not
  originate inside a run, the resulting wake shall be a root as defined by
  AC-OFFICE-RUN-CAUSATION-001.2 rather than being attributed to an arbitrary
  in-flight run.

## Out of scope

- **Concurrency ceilings and causation depth.** Owned by
  [Office Unattended Launch Safety](unattended-launch-safety.md), together with the
  terminology this document borrows and the refusal-gate seam, ordering and
  idempotency rules of AC-OFFICE-LAUNCH-SAFETY-003.4, `003.8`, `003.9` and `003.10`
  that these criteria are bound by.
- **The wake-reason registry itself.** Owned by
  [Office Launch Backpressure](launch-backpressure.md). AC-OFFICE-LAUNCH-SAFETY-004.3
  requires an agent-requested wake to name a member of that registry; it does not
  define the registry, its membership, or the priority class each member maps to.
- **Launch volume over time.** Owned by
  [Office Launch Budgets](launch-budgets.md). A self-trigger allowance bounds one
  agent waking itself; a budget bounds total launches whatever caused them. A wake
  can pass this document's allowances and still be refused by a budget.
- **Per-workspace configured overrides.** Both allowances take their numeric values
  from instance-level configuration, as the ceilings do. A stored per-workspace
  override is a later change.
- **Suppressing a two-agent cycle.** A→B→A is not self-caused under
  AC-OFFICE-LAUNCH-SAFETY-004.7 and is bounded by causation depth instead. Detecting
  a cycle of length greater than one is a separate contract; widening "self" to mean
  "anything in my chain" would refuse legitimate delegation.
