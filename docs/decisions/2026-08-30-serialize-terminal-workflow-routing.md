# ADR-2026-08-30-serialize-terminal-workflow-routing: Serialize Terminal Workflow Routing

**Status:** accepted
**Date:** 2026-08-30
**Area:** workflow

## Context

Workflow moves, explicit completion signals, turn-end replay, engine transitions,
and PR lifecycle turns previously committed through separate transaction owners.
A running-session move could therefore report success after only arming a
replaceable pending row, while a later stale signal or cancellation left the
task in its old lane.

## Decision

All automated and deferred transitions use the task row's current workflow step
as a compare-and-swap generation. A terminal move requested by the task's active
agent commits synchronously through the task service, including terminal task
state and the task-step ledger. Terminal state is absorbing for stale agent,
engine, deferred, and integration producers; only an explicit current-generation
manual reopen may leave it.

Deferred moves carry a stable row/operation identity and their expected source
step. Queue set, restore, transfer, claim, and deletion operations compare that
identity and never overwrite a newer generation. Turn-end applies the task CAS
before exact row deletion, so a crash leaves an idempotent row rather than a
lost transition. The committed task-step ledger remains the authoritative lane
audit; signal and external-cause attribution is correlated by the routing
operation identity.

`workflow_route_operations` is the durable request/outcome readback and
`workflow_route_effects` allocates the winning destination-entry identity in
the task transition transaction. Effect delivery uses an atomic pending →
claimed → completed CAS with a claimant token; an expired claim may be
reclaimed after a crash, while completed effects are absorbing. The existing `task_step_transitions` row is
the physical lane ledger, while engine-owned on-entry actions continue to use
their transition-derived `workflow_step_entries` markers. This composes with,
rather than duplicates, the exact-row cancellation schema from PR #3155.
An operation ID is globally immutable across its task, producer, source,
target, caller generation, actor, and external cause. A conflicting reuse
fails the owning task transaction instead of rewriting the stored operation.
Deferred application reloads the original operation before committing, so
turn-end and crash recovery retain the same attribution.

## Consequences

Terminal move success now means the task is physically terminal before the MCP
response returns. Retried producers converge without duplicate transition or
on-entry behavior. Route operation and effect readback exposes the exact
transition correlation and destination-entry status. Legacy pending rows
without a source generation remain readable but fail closed during automatic
replay. New and old backend versions
must not process the same pending queue during rollout.

The queue and task repositories share a lock order: workspace/task admission,
task row, sorted queue-session locks, exact pending row, then destination
capacity. Exact pending-move cancellation reuses this identity and ordering but
retains its separate authorization and administrative policy.

## Alternatives Considered

- Prioritize Done in turn-end handler ordering. Rejected because restart,
  overwrite, and audit gaps remain.
- Add only an expected-step column to pending moves. Rejected because duplicate
  producers and exact row replacement would still have separate ownership.
- Make every merged PR globally auto-complete its task. Rejected because existing
  PR lifecycle settings are prompt-driven and workflows may have post-merge work.
