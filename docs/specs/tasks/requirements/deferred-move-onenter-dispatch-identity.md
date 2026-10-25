---
status: draft
system: tasks
created: 2026-09-21
owners:
  - nova28
---

# Deferred Move on_enter Dispatch Identity Requirements

## Overview

A deferred `move_task_kandev` (a backward or otherwise non-immediate board
move, applied at turn end) commits a workflow-step transition and then
dispatches the destination step's `on_enter` actions itself. That dispatch
must be able to identify itself as the current entry using the transition it
just committed, so the destination step's declared `on_enter` actions
(including `auto_start_agent`) actually run instead of being rejected by the
staleness guard as if they belonged to an earlier, superseded entry.

## Requirements

### REQ-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001: Deferred move on_enter dispatch identity

**Intent:** A deferred move's own `on_enter` dispatch carries the identity of
the transition it just committed, so the staleness guard recognizes it as
current rather than rejecting it as stale.

#### Acceptance criteria

- **AC-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001.1:** When a deferred
  `move_task_kandev` commits a workflow-step transition, the system shall pass
  that transition's ledger identity to the destination step's `on_enter`
  dispatch.
- **AC-TASKS-DEFERRED-MOVE-ONENTER-DISPATCH-IDENTITY-001.2:** When the
  `on_enter` staleness guard evaluates a deferred-move dispatch that carries
  its own just-committed transition identity, the system shall recognize it
  as current and execute the destination step's declared `on_enter` actions,
  including `auto_start_agent`, in declared order. A dispatch that does not
  carry the current transition identity shall remain rejected.

## Out of scope

- The staleness guard's rejection logic itself
  (`workflowEntryDispatchMayReplaceRoute`, `workflowEntryDispatchIsCurrentForSession`),
  which correctly rejects a genuinely stale callback and is unchanged by this
  requirement.
- The forward-move (`E1`, agent-turn auto-advance) `on_enter` dispatch path,
  which already threads its own transition identity through
  `stepentry.AllocationResult`.
- The broader multi-path `on_enter` dispatch unification (covering
  `switch_workflow`, WIP-queue promotion, and the full acceptance-criteria set
  `AC-A1`-`AC-A13`) tracked informally in
  [`workflow-on-enter-action-dispatch/spec.md`](../../workflow-on-enter-action-dispatch/spec.md).
  This requirement formalizes only the narrow deferred-move identity gap that
  subset (`AC-A1`, `AC-A2` as applied to the `E2` deferred-move entry path)
  describes.
