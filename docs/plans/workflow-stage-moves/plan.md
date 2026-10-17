---
created: 2026-10-12
status: implemented
requirements:
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-001
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-002
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-003
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-004
system_design:
  - ../../specs/tasks/system-design/workflow-stage-moves.md
legacy_specs:
  - ../../specs/workflow-step-move-overrides/spec.md
  - ../../specs/workflow-on-enter-action-dispatch/spec.md
---

# Implementation Plan: Workflow stage move integrity

## Outcome

Four independent fixes on the workflow step-change paths:

1. A deferred `move_task_kandev` no longer overwrites a later move, including a
   later move that leaves and returns to the same step.
2. A same-step WIP queue admission runs the admitted step's entry once, for the
   entry it admitted.
3. A signal-gated step does not advance while its own entry prompt is still
   waiting for session capacity.
4. `move_task_kandev` rejects a hand-off to a manual step and tells the agent
   the reason for known move failures.

The requirements are in
[`workflow-stage-moves.md`](../../specs/tasks/requirements/workflow-stage-moves.md).
The technical contract is in
[`workflow-stage-moves.md`](../../specs/tasks/system-design/workflow-stage-moves.md).

## Root causes

- **Deferred move:** `applyPendingMove` uses the task's current step as the
  transition source. A pending move records only its target, so a move
  requested from step A still applies after a later move placed the task on B,
  or moved it A to B to A.
- **Same-step admission:** a same-step admission records no ledger row, so
  `task.queue_promoted` carries `step_transition_id` zero. The `processOnEnter`
  entry guard cannot identify the admitted entry and drops it as a route
  change. Resolving the entry at handling time from the latest ledger row
  admits a stale delivery after a re-entry.
- **Signal-gated advance:** `validateCeilingDestination` discards a bound,
  capacity-deferred entry prompt as superseded when its destination session is
  running. `allowEngineSignalCompletion` then consumes a matching completion
  signal without checking that the step's own prompt was delivered.
- **Manual step hand-off:** the MCP move path defers a step change with entry
  options before any check that the target step starts an agent. Move errors
  collapse to fixed generic text that does not tell the agent what to change.

## Work orders

| Order | Work order | Requirement | Depends on |
| --- | --- | --- | --- |
| 1 | [task-01-drop-superseded-deferred-moves.md](task-01-drop-superseded-deferred-moves.md) | `REQ-TASKS-WORKFLOW-STAGE-MOVES-001` | none |
| 2 | [task-02-same-step-admission-entry.md](task-02-same-step-admission-entry.md) | `REQ-TASKS-WORKFLOW-STAGE-MOVES-002` | none |
| 3 | [task-03-hold-signal-advance-for-entry-prompt.md](task-03-hold-signal-advance-for-entry-prompt.md) | `REQ-TASKS-WORKFLOW-STAGE-MOVES-003` | none |
| 4 | [task-04-reject-manual-step-hand-offs.md](task-04-reject-manual-step-hand-offs.md) | `REQ-TASKS-WORKFLOW-STAGE-MOVES-004` | none |

The work orders have no logical dependency. Orders 1 to 3 edit
`orchestrator/event_handlers_workflow.go`, so they land as separate commits in
this order on one branch.

## Constraints

- Keep each change to the named path. Do not move or rename shared helpers in
  `event_handlers_workflow.go`, `ceiling_entry.go`, or
  `task/repository/sqlite/task.go`.
- Schema changes are additive columns on `pending_moves` only, created in the
  table DDL and with idempotent `ADD COLUMN`.
- Do not weaken the `processOnEnter` entry guard, the ceiling running-provider
  check, or `TestMoveTaskErrorMessage_SanitizesClassifiedErrors`'s intent that
  internal identifiers never reach the agent.
- HTTP and WebSocket move validation is unchanged.

## Verification strategy

Each work order adds focused unit tests next to the changed code. The tests
fail before the fix and pass after it. Run only the named tests with `-run`,
then build and vet the changed packages once:

```bash
cd apps/backend
go build ./...
go vet ./internal/orchestrator/... ./internal/task/... ./internal/mcp/... ./internal/workflow/...
```

Documentation checks run once after all orders:

```bash
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
node scripts/validate-public-docs.mjs
```

## Risks

- **Pending-move schema:** an open proposal for exact pending-move
  cancellation also adds a `PendingMove` field and changes the same
  `pending_moves` column lists. The second change to land must merge both
  column sets. The two predicates are independent.
- **Signal-gated timing:** order 3 delays an advance until the entry prompt is
  delivered. Ceiling records have no age limit, so a hold ends only when the
  destination turn ends, the task leaves the step, or the destination session
  ends. A destination that stays running keeps the hold for that long. The
  design records this limitation, and the work order tests the release
  events.
- **Agent surface narrowing:** order 4 rejects options to a manual step even
  when the task has an active session that the service path could use for a
  hand-off. This rejection is deliberate for agent callers only. The public
  agent documentation states it.
- **Concurrent churn:** these files change often. Re-check the branch against
  the current `main` before review, and re-run the focused tests.
