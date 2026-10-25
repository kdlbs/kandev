---
id: "05-caller-attribution-wiring"
title: "Caller attribution wiring"
status: done
wave: 5
depends_on: ["04-ledger-writer-chokepoints"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-task-step-transition-ledger.md"
---

# Task 05: Caller attribution wiring

Give each production path that changes a task's workflow step its declared
trigger, actor kind, actor identifier, and initiating session, by wrapping the
context once near the entry point. After this task the ledger's `trigger` and
`actor_kind` columns carry real values instead of `unknown`.

## Acceptance

1. Each path in the spec's *Path → Trigger* table records its mapped trigger,
   and a move reaching `MoveTask` through more than one path is attributed to
   the **outermost** caller (`mcp_move` beats `manual_move`).
2. `session_id` is the initiating session when one genuinely initiated the
   transition, and `NULL` when there is none or when several sessions are
   candidates with no single initiator.
3. `actor_id` holds an identifier and nothing else: no display name, email,
   title, or prompt text appears anywhere in any row.

## Verification

```
cd apps/backend && go test -race ./internal/task/service/... ./internal/task/repository/sqlite/... ./internal/orchestrator/... ./internal/mcp/... && make lint
```

Because this task edits `internal/orchestrator/`, also run the CI-style
changed-file linter with the PR base SHA before pushing:

```
cd apps/backend && golangci-lint run ./... --new-from-rev="<base-sha>" --timeout=5m
```

## Files likely touched

- `apps/backend/internal/task/service/service_workflow.go` —
  `MoveTaskWithOptions` (428), `BulkMoveSelectedTasks` (947), `BulkMoveTasks`
  (1049), `pullNextTaskOnVacate` (558), `promoteNextQueuedTask` (612)
- `apps/backend/internal/task/service/service_tasks.go` — `UpdateTask` (1362),
  where the request carries a new `workflow_step_id`
- `apps/backend/internal/mcp/handlers/config_task_handlers.go` —
  `applyMoveTaskImmediate` (176)
- `apps/backend/internal/orchestrator/event_handlers_workflow.go` —
  `executeStepTransition` (296), `applyPendingMove` (1243)
- `apps/backend/internal/orchestrator/workflow_store.go` — `ApplyTransition`
  (154)
- `apps/backend/internal/orchestrator/watcher_dispatch.go`,
  `source_jira.go`, `source_linear.go` — integration actor
- `apps/backend/internal/task/repository/sqlite/step_transitions_actor_test.go`
  (new)
- `apps/backend/internal/orchestrator/workflow_step_ledger_test.go` (new) —
  engine trigger, deferred move, and `OperationID` retry cases

## Dependencies

Task 04 — the writer and the `Attribution` type must exist.

**Also confirm before starting:** the spec records that wiring
`session_step_history`'s dormant writer is separate work that "lands first" and
edits `event_handlers_workflow.go` and `workflow_store.go` — the same two files
this task edits. Check whether it has landed; if not, expect to be the second
writer into those files.

## Parallelism

`sequential`.

## Inputs

- Spec, *Trigger* and *Actor kind* tables, the *Path → Trigger* mapping (which
  is fixed precisely so no implementer has to choose), and *Scenarios → Slice 2
  / Actor and privacy*
- Plan, *Area 6*
- **Outermost caller wins.** `MoveTaskWithOptions` must set `manual_move` only
  when the context carries no trigger already, so the `mcp_move` set by the MCP
  handler survives the inner call. An unconditional overwrite there silently
  relabels every agent move as a board click.
- **`actor_id` and `session_id` deliberately duplicate for agents.** For actor
  kind `agent` both hold the same session ID, so `actor_id` has one uniform
  meaning across every actor kind and consumers never branch on kind to find the
  actor. This is intentional, not redundancy to clean up.
- **`engine_transition` splits by origin.** It is `agent` when the trigger came
  from a session's turn (`on_turn_start`, `on_turn_complete`, or an
  `on_exit`/`on_enter` reached through one) and `system` when it came from a
  non-session trigger such as a children-completed rollup or a scheduled
  evaluation. `ApplyTransition` already receives `sessionID`; use it to decide.
- **WIP pulls record `system` with `session_id` NULL** even when the task has
  live sessions. No one session initiated the pull, and the spec forbids picking
  one to manufacture ownership that does not exist.
- `authn.IdentityFromContext` (`internal/auth/authn/identity.go:57`) is the
  human actor source. `Identity.Synthetic` marks the implicit single-user
  identity injected when auth is disabled — that is still actor kind `human`
  with that identity's `UserID`, per the spec's scenario, not `system`.
- The privacy test should scan every text column of every row produced by the
  full trigger matrix against a fixture whose title, description, and prompt are
  unique sentinel strings. Asserting only on `actor_id` misses a name leaking
  into another column.
- Anything left unwrapped records `unknown`/`unknown` and still commits. Do not
  add an error path for undeclared attribution.

## Output contract

Summary, files changed, tests run with counts, blockers, risks, and a status
update to this file and `plan.md` in the same conversation.

## Results

See `plan.md` § Verification Results for the consolidated commands, outcomes, and decision record covering all six tasks (implemented and committed together).
