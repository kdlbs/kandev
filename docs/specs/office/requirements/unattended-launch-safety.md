---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office Unattended Launch Safety Requirements

## Overview

Office can start agent processes with no human present: a cron routine fires, an
agent creates a task, an assignment wakes the assignee, that agent comments, and
the cycle repeats. Every launch costs money and machine capacity.

Two controls are absent: no ceiling on how many Office agent processes may run at
once, and no way to tell that one launch descends from another, so a chain of
agent-triggered launches cannot be detected, bounded, or attributed.

Office owns this contract because Office owns the decision to start an agent: the
queue row, the claim transition, the wake reason, and the runtime capability
envelope are all Office concepts. The task system owns task rows and the agent
system owns agent profiles, but neither decides when a process starts.

This document bounds concurrency **at an instant**: how many Office agent processes
may run at once, and how deep a chain of agent-caused launches may go. Sibling
documents carry the rest, none duplicating another:

- [Office Enqueue Consolidation](enqueue-consolidation.md) defines the single seam
  every gate here attaches to. It is a delivery prerequisite for this document.
- [Office Run Causation Chain](run-causation-chain.md) defines the identity every
  depth rule here consumes: the causation id, the parent run, the depth, the
  actor, the human-rooted flag, the workspace, and the routine.
- [Office Launch Budgets](launch-budgets.md) bounds launch volume **over time** and
  owns the durable launch record those budgets count.
- [Office Launch Backpressure](launch-backpressure.md) defines who wins when
  capacity is scarce: claim order, age promotion, and gate observability.
- [Office Self-Triggered Launch Suppression](self-trigger-suppression.md) bounds how
  often an agent may wake itself. It owns `REQ-OFFICE-LAUNCH-SAFETY-004` and every
  `AC-OFFICE-LAUNCH-SAFETY-004.x`, which this document's criteria still cite by those
  identifiers; the identifiers did not change when the requirement moved.

This capability is a prerequisite for arming any unattended schedule. The
workspace kill switch and the fail-closed cost budget are the other two safety
controls, and neither is in scope here.

## Prior art

Two prior-art legs were attempted; each receipt names what was searched, because a
degraded search reads like a healthy one that found nothing.

**Leg 1, the compiled wiki.** Searched: vault `@henry`, resolved to
`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`, QMD collection `wiki`.
**Result: unavailable, not empty.** `qmd` and `obsidian-wiki` are absent from `PATH`,
no `qmd` MCP transport is registered, and the documented grep fallback returns
`EPERM`. No prior wiki reasoning was retrieved, and none should be assumed absent.

**Leg 2, cross-vendor.** Searched: intended `saas-kb` MCP `search_fsm_docs`,
`category: "ai_sdlc"`. **Result: unavailable.** No server registered, so no vendor
query ran. Nothing is claimed here about how other platforms bound concurrency.

**Prior art that was available: this repository's own.**

- The claim query already holds a run `claimed` for the process's lifetime, so this
  document counts `claimed` rather than sessions or operating-system processes.
- `system-design/scheduler-01.md` specifies the agent ceiling as
  `< a.max_concurrent_sessions`, and only that **predicate** is carried forward. The
  rest is superseded and is not authority here: its claim query reads
  `office_wakeup_queue` joined to `office_agent_instances`, a table removed in ADR
  0005 Wave C, and counts `task_sessions`, which Terminology below rejects as the
  unit of account.
- `shared.IsPeriodicTasklessWake` already refuses to guess a legacy
  `routine_dispatch` row's trigger source.
  [Office Launch Backpressure](launch-backpressure.md) follows that precedent
  rather than inventing a second answer.

**What is being done differently, and why.** Every gate defined here fails closed on
an unreadable input, the opposite of the adjacent pre-execution cost budget check. An
unreadable cost check risks overspend a human can see and reverse; an unreadable
concurrency check risks the unbounded fan-out this capability exists to prevent. The
asymmetry is deliberate, is recorded in AC-OFFICE-LAUNCH-SAFETY-001.8, and is not to
be later "fixed" into consistency.

## Terminology

- **Launch:** the transition of a run row from `queued` to `claimed`. A claimed run
  is held for the lifetime of the agent process it started, so `claimed` is the
  currency in which live concurrency is counted. Session rows and operating-system
  processes are not the unit of account.
- **Causation chain**, **causation depth**, **root cause**, **actor**, and
  **human-rooted** are defined by [Office Run Causation Chain](run-causation-chain.md)
  and are used here with that meaning. This document sets limits on them; it does
  not define them.
- **Deferral:** declining to claim a run while leaving it `queued`, unmodified, and
  eligible for a later claim.
- **Refusal:** declining to create a run row at all, returning an error to the
  caller that asked for it.
- **Authoritative enqueue API:** the single server-side entry point through which
  every wake reason must reach the queue, defined by
  [Office Enqueue Consolidation](enqueue-consolidation.md). Every **refusal** gate here
  attaches to it, because a refusal gate on a narrower seam is bypassable; the ceilings
  are deferral gates and attach to the claim instead.
- **Unreadable gate input:** a value a gate needs that the system queried and did not
  obtain, because the query errored, timed out, or returned a value outside its
  declared type or range. A context cancelled by shutdown is **not** an unreadable
  input and is not a gate failure.

## Requirements

### REQ-OFFICE-LAUNCH-SAFETY-001: Bounded concurrent launches

**Intent:** Cap simultaneous Office agent processes so an unattended loop cannot
saturate the machine or the provider account. Today the only bound is one claimed
run per agent profile, hardcoded in the claim query, so the effective ceiling is
the number of Office agents and grows every time an operator adds one.

**User story:** As an operator running Office unattended, I want a hard ceiling on
simultaneous agent launches, so that a misbehaving routine cannot consume the
machine while I am away.

#### Acceptance criteria

- **AC-OFFICE-LAUNCH-SAFETY-001.1:** When a run is claimed, the system shall
  evaluate three ceilings and claim only if every one of them has spare capacity:
  an instance ceiling on all runs in status `claimed`, a workspace ceiling on runs in
  status `claimed` carrying the same persisted workspace as the claiming run, and an
  agent ceiling on runs in status `claimed` for that agent profile. The workspace is
  the value persisted by AC-OFFICE-RUN-CAUSATION-001.20; no ceiling or budget shall
  resolve a run's workspace by joining to its agent profile at claim time.
- **AC-OFFICE-LAUNCH-SAFETY-001.2:** The agent ceiling shall be the value of
  `agent_profiles.max_concurrent_sessions` for the claiming run's agent profile.
- **AC-OFFICE-LAUNCH-SAFETY-001.3:** When `agent_profiles.max_concurrent_sessions`
  is `NULL`, absent, or less than `1`, the system shall use `1`.
- **AC-OFFICE-LAUNCH-SAFETY-001.4:** The instance ceiling shall default to `8` and
  the workspace ceiling shall default to `4`, each overridable by operator
  configuration.
- **AC-OFFICE-LAUNCH-SAFETY-001.5:** When a configured instance or workspace
  ceiling resolves to a value less than `1`, the system shall use the documented
  default for that ceiling, log the rejected value at warn level, and start
  normally. A configured `0` shall not mean unlimited. Every ceiling and budget
  this document and its siblings define is a boot-time-only startup setting,
  like `office.schedulerTickMs`: resolved once at startup from YAML/environment
  and clamped there, not read from ADR 0018's runtime settings-override tier and
  not re-resolved without a restart. This clamp requirement therefore applies at
  that single resolution point, not "wherever the value is read" — there is no
  other read site to clamp.
- **AC-OFFICE-LAUNCH-SAFETY-001.6:** The ceiling evaluation and the `queued` to
  `claimed` transition shall be serialized against every other concurrent claim
  attempt, on every supported database engine, so that two claim attempts can never
  both observe the same free slot. A single statement is sufficient only where the
  engine already serializes writers; where it does not, the system shall take an
  explicit lock for the duration of the claim.
- **AC-OFFICE-LAUNCH-SAFETY-001.7:** When a run is not claimed because a ceiling is
  saturated, the system shall leave the run in status `queued` with `requested_at`,
  `retry_count`, and `scheduled_retry_at` unchanged.
- **AC-OFFICE-LAUNCH-SAFETY-001.8:** When any ceiling input is unreadable as defined
  in Terminology, or the claiming run names an agent profile that does not exist, the
  system shall defer the run rather than claim it, and shall record the failure as
  described in AC-OFFICE-BACKPRESSURE-003.3. A missing agent profile shall never be
  read as an unbounded agent ceiling. A shutdown-cancelled evaluation shall defer the
  run without recording a gate failure, so a restart is not mistaken for a gate that
  is failing closed.
- **AC-OFFICE-LAUNCH-SAFETY-001.9:** The effective agent ceiling shall be the lesser
  of `agent_profiles.max_concurrent_sessions` and the configured workspace and
  instance ceilings. An agent that raises its own or another agent's
  `max_concurrent_sessions` shall not thereby raise the real bound above what the
  operator configured.

### REQ-OFFICE-LAUNCH-SAFETY-003: Bounded causation depth

**Intent:** Stop a chain of agent-triggered launches from continuing forever. An
agent can create a task, which wakes an agent, which creates a task. Nothing counts
how deep that has gone.

**User story:** As an operator, I want a chain of agent-caused launches to stop at a
known depth and say so, so that a runaway loop ends by itself and leaves evidence.

#### Acceptance criteria

- **AC-OFFICE-LAUNCH-SAFETY-003.1:** The maximum causation depth shall default to
  `8` and be overridable by operator configuration. A configured value less than
  `1` shall be replaced by the default and logged at warn level; `0` shall not be a
  way to express "roots only".
- **AC-OFFICE-LAUNCH-SAFETY-003.2:** When a run would be queued at a causation
  depth less than or equal to the maximum, the system shall queue it. When it would
  be queued at a depth greater than the maximum, the system shall refuse to create
  the run.
- **AC-OFFICE-LAUNCH-SAFETY-003.3:** When an enqueue is refused for exceeding the
  maximum depth and the request came from a runtime action, the system shall return
  a distinguishable error to the calling agent naming the depth limit, rather than
  reporting success.
- **AC-OFFICE-LAUNCH-SAFETY-003.4:** When an enqueue is refused by any gate, for
  exceeding the maximum depth or for any other reason, the system shall not consume
  the request's idempotency key, so that a later legitimate request carrying the same
  key is not silently discarded as a duplicate. This holds for every refusal defined
  by these documents, including the per-reason self-trigger refusal of
  AC-OFFICE-LAUNCH-SAFETY-004.3, the total self-trigger refusal of
  AC-OFFICE-LAUNCH-SAFETY-004.8, the unreadable-causing-run refusal of
  AC-OFFICE-RUN-CAUSATION-001.21, and the missing-workspace refusal of
  AC-OFFICE-RUN-CAUSATION-001.20. Reading the key to find a coalescing target is not
  consuming it.
- **AC-OFFICE-LAUNCH-SAFETY-003.5:** When an enqueue is refused for exceeding the
  maximum depth, the system shall record a durable operator-visible entry naming
  the causation identifier, the refusing depth, and the agent that requested it.
- **AC-OFFICE-LAUNCH-SAFETY-003.6:** A depth refusal shall apply to every enqueue
  path, including runtime-requested runs, wakes caused by an agent-authored
  comment, wakes caused by an agent-set assignment, wakes caused by a task an agent
  created, and runs queued by a workflow step action.
- **AC-OFFICE-LAUNCH-SAFETY-003.7:** The recovery sweep, retry of an existing run,
  and provider-routing re-dispatch shall not increment causation depth, because
  they re-attempt work already admitted rather than causing new work.
- **AC-OFFICE-LAUNCH-SAFETY-003.8:** The depth check, both self-trigger window
  counts, the idempotency check, and the insert shall share one transaction, and
  shall be serialized against every other concurrent enqueue for the same profile, on
  every supported database engine, so that two concurrent enqueues cannot both observe
  the last remaining allowance. As in AC-OFFICE-LAUNCH-SAFETY-001.6, one transaction
  is sufficient only where the engine already serializes writers; where it does not,
  the system shall take an explicit lock for the duration of the enqueue. The
  self-trigger windows are counts over rows the insert does not lock, so transaction
  scope alone does not serialize them.
- **AC-OFFICE-LAUNCH-SAFETY-003.9:** Every **refusal** gate in this document, meaning
  the depth limit of this requirement and the self-trigger suppression of
  REQ-OFFICE-LAUNCH-SAFETY-004, shall attach to the authoritative enqueue API defined
  by [Office Enqueue Consolidation](enqueue-consolidation.md), and to no narrower seam.
  That document is a delivery prerequisite for this one: until it is satisfied, a
  refusal gate here is bypassable by any other insert path. The three ceilings of
  REQ-OFFICE-LAUNCH-SAFETY-001 are deferral gates evaluated at claim, per
  AC-OFFICE-LAUNCH-SAFETY-001.1, and do not attach to the enqueue seam.
- **AC-OFFICE-LAUNCH-SAFETY-003.10:** The authoritative enqueue shall resolve
  idempotency and coalescing **before** every refusal gate. A request that merges into
  an existing run creates no run row, so it shall not be evaluated against any refusal
  gate: not the depth limit, neither self-trigger allowance, not the unreadable causing
  run of AC-OFFICE-RUN-CAUSATION-001.21, and not the missing workspace of
  AC-OFFICE-RUN-CAUSATION-001.20. Those refusals are one class and are ordered
  alike. The registered-reason check of AC-OFFICE-LAUNCH-SAFETY-004.3 is not in that
  class and is not ordered here: it runs before the authoritative enqueue is called.
  Reading the idempotency key to find a merge target does not record one, so
  AC-OFFICE-LAUNCH-SAFETY-003.4 still holds for a request that is then refused.

## Out of scope

- **Causation identity and propagation.** Owned by
  [Office Run Causation Chain](run-causation-chain.md). This document consumes the
  identifier, the depth, the actor, the human-rooted flag, and the routine
  attribution; it does not define how any of them are derived or carried.
- **How often an agent may wake itself.** Owned by
  [Office Self-Triggered Launch Suppression](self-trigger-suppression.md), which
  carries `REQ-OFFICE-LAUNCH-SAFETY-004` under its original identifiers. The refusal
  seam, ordering and idempotency rules of REQ-OFFICE-LAUNCH-SAFETY-003 still bind
  those criteria; this document defines those rules and that one consumes them.
- **Claim order, promotion, and gate telemetry.** Owned by
  [Office Launch Backpressure](launch-backpressure.md). This document decides
  whether a run may be claimed; that one decides which claimable run goes first and
  how a blocked launch is reported.
- **Consolidating the enqueue paths.** Owned by
  [Office Enqueue Consolidation](enqueue-consolidation.md). This document states that
  its gates attach to that seam; it does not define the seam or the migration onto it.
- **Workspace kill switch.** An atomic workspace-level stop blocking new claims of
  every class is separate work. Ceilings reduce blast radius; a ceiling of `1` is
  not a stop button.
- **Launch volume over time.** Owned by
  [Office Launch Budgets](launch-budgets.md), together with the durable launch
  record those budgets count and the retention question that record raises. This
  document bounds parallelism at an instant only.
- **Fail-closed cost budget check.** The existing pre-execution cost budget proceeds
  when its checker errors. Changing that is separate work; the gates here differ from
  it on purpose.
- **Per-workspace configured overrides.** Counting scope is per workspace, but the
  numeric values come from instance-level configuration. A stored per-workspace
  override is a later change.
- **Stale-claim recovery double launch.** The recovery sweep re-queues a run claimed
  longer than its staleness threshold with no liveness check, so an agent whose
  process outlives the threshold can hold one live process and have a second run
  claimed: an agent ceiling of `N` can be exceeded by one per such agent. Closing it
  needs a launch lease and orphan reclamation, tracked separately; this document
  does not weaken its counting rule to compensate.
- **Authority boundary for unattended side effects.** What a scheduled agent may
  merge, deploy, spend, or delete is a governance contract, not a rate control.
- **Done, stuck, and blocked detection.** Whether the loop is making progress is a
  separate correctness contract.
- **Operator notification channels.** Durable operator-visible records and counters
  are required here; who is paged, on what channel, within what time, is separate.
- **Cron correctness.** Day-of-month and day-of-week semantics, daylight-saving
  behavior, and missed-fire policy are tracked separately. This document bounds what
  a fire may cost, not when it fires.
