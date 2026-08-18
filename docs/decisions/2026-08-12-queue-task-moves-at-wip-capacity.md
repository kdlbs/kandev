# ADR-2026-08-12-queue-task-moves-at-wip-capacity: Queue Task Moves at WIP Capacity

**Status:** accepted
**Date:** 2026-08-12
**Area:** backend, frontend, protocol, workflow

## Context

WIP limits separate visible work from admitted work during task creation.
Explicit task moves still return a conflict when the destination has no
capacity. Users cannot signal that a task is ready for a limited step while
another task occupies its WIP slot.

The existing feeder mechanism does not provide this manual gate. Untagged
feeder tasks are eligible for automatic intake without an explicit move.

## Decision

All task moves use destination admission. A move to a full limited step commits
the task in that destination with `wip_admitted = false`,
`queued_for_step_id` set to the destination, and `queued_at` set.

The move does not route through `pull_from_step_id`. Feeders remain optional
automatic intake sources. Destination-resident queued tasks have admission
priority over feeder-resident candidates.

The source step exits when the queued move commits. Destination entry behavior
runs only after admission. The Kanban column shows active and queued areas. The
task sidebar shows the queue position for destination-resident queued tasks.

This decision replaces the capacity-conflict rule for explicit, bulk, approval,
MCP, and workflow-engine moves in ADR-2026-07-28-visible-wip-overflow-queues.
The remaining creation, persistence, and feeder rules stay accepted.

## Consequences

- Users can approve work for a limited step before capacity opens.
- Bulk and automated task moves no longer fail only because the destination is
  full.
- Queue metadata and existing promotion transactions remain the persistence
  boundary. No new task column is necessary.
- The orchestrator must separate source exit from destination entry for a
  queued move.
- Queue position is derived from the same deterministic order that controls
  destination admission.
- Workflows that need automatic feeder intake keep `pull_from_step_id`.
  Workflows that need a manual gate omit it and use destination queue moves.

## Alternatives Considered

- **Keep move conflicts and add a manual feeder mode.** Rejected because it
  adds step configuration and preserves two locations for one destination
  queue.
- **Change every feeder to manual-only intake.** Rejected because it silently
  changes existing automatic workflows.
- **Keep queued tasks in the source step.** Rejected because the board would
  not show that the move succeeded or group queued work with its destination.
- **Add a hidden global queue.** Rejected because users could not inspect the
  queue in the workflow that owns it.
