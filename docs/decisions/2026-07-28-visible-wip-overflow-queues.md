# ADR-2026-07-28-visible-wip-overflow-queues: Separate Visible Queueing From WIP Admission

**Status:** accepted
**Date:** 2026-07-28
**Area:** backend, frontend, protocol, workflow

## Context

Kandev originally treated a workflow step's WIP limit as a hard cap on resident
task cards. Atomic creation admission fixed overfilling, but it also caused
integration fan-out beyond the cap to remain invisible until a later retry.
That behavior conflates two separate concerns: accepting and displaying work,
and admitting work for active processing.

A feeder step already models an explicit backlog for a limited destination.
Not every workflow has or wants a separate feeder, however. A two-column
`Review -> Done` workflow should still be able to display seven discovered
reviews while limiting active review work to two.

The design must remain generic across UI, HTTP, WebSocket, MCP, and integration
creation. It must also survive restarts without preparing runtime resources for
every queued task.

## Decision

WIP limits govern admitted tasks, not total visible cards.

When creation targets a full limited step:

1. If the destination configures `pull_from_step_id`, Kandev places the new task
   in that feeder and records the intended destination.
2. If it has no feeder, Kandev places the task in the destination itself but
   marks it queued and non-admitted.
3. If the configured feeder is itself full, Kandev stops after that one hop and
   returns a capacity conflict. It does not recursively traverse feeder chains.

Queue and admission state are durable task-domain data. A queued create-and-start
request also stores a durable launch intent; it does not create a session,
workspace, checkout, container, or agent process until promotion. Promotion is
transactional, deterministic, idempotent, and reconciled after capacity-changing
events and backend startup.

Destination-tagged overflow tasks cannot be consumed by another destination
that shares the same feeder. Untagged feeder tasks retain the existing generic
pull behavior.

Manual moves and workflow-engine transitions into a full step continue to
return capacity conflicts. This decision changes creation overflow only.

## Consequences

- All accepted integration work is immediately visible and durable.
- A limited column may contain more cards than its limit; the UI must distinguish
  admitted WIP from queued cards and show `admitted/limit`.
- WIP is no longer derivable from raw resident task count. Repository admission,
  moves, promotion, migration, and recovery must use explicit admission state.
- Deferred agent starts require a durable, idempotent launch-intent boundary.
- Workflows with feeders retain an explicit backlog presentation. Compact
  workflows without feeders gain an in-column queue.
- A full configured feeder can still reject creation. The one-hop rule is
  predictable but remains the draft assumption to revisit if real workflows
  require recursive routing.

## Alternatives considered

- **Keep rejecting task creation at capacity.** Rejected because work discovered
  by integrations remains invisible and depends on polling for eventual
  representation.
- **Always create overflow in a hidden system queue.** Rejected because it
  removes work from the user's workflow and adds a second, implicit task
  location model.
- **Allow every resident task to count as WIP but throttle only sessions.**
  Rejected because workflow WIP and agent-profile execution concurrency are
  distinct controls, and non-agent tasks also require queue semantics.
- **Always overfill the destination without explicit queue state.** Rejected
  because the system could not determine which tasks may auto-start or promote
  safely after restart.
- **Recursively walk feeder chains.** Deferred because it makes task placement
  depend on graph traversal and introduces less predictable authoring,
  ordering, and cycle behavior.
- **Redirect manual moves into the queue.** Rejected for this change because it
  would silently change an explicit drag/drop action from a conflict into a
  successful but non-active move.
