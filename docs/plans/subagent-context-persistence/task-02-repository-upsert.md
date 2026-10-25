---
id: "02-repository-upsert"
title: "Model and repository upsert"
status: pending
wave: 2
depends_on: ["01-schema-and-backfill"]
plan: "plan.md"
spec: "../../specs/agents/requirements/subagent-context-persistence.md"
---

> **Amendment 1 update:** the shipped conflict key is
> `(task_session_id, agent_execution_id, tool_call_id)`, and `SubagentContext`
> carries `AgentExecutionID` — see the "Amendment 1 update" note near the top
> of `plan.md`.

# Task 02: Model and repository upsert

Add the `SubagentContext` model and the single atomic upsert that encodes every
merge, terminality, and ordering rule in SQL.

## Acceptance

1. `UpsertSubagentContext` is one `INSERT ... ON CONFLICT DO UPDATE` statement
   with no read-then-write anywhere. Repeated calls for one
   `(task_session_id, tool_call_id)` produce exactly one row; concurrent calls
   for the same key and for different keys in one turn both hold. (AC-3, AC-14,
   AC-15)
2. The conflict clause satisfies fill-forward (AC-4, AC-5), write-once
   `settled_at` (AC-11), post-settle immutability of `tool_status` with
   continued NULL-filling (AC-12), original-`turn_id` preservation (AC-2a),
   sticky `is_async`, and write-once `source` / `observed_at` (AC-1a, AC-22).
3. Reads are ordered `observed_at ASC, tool_call_id ASC`, and deleting the
   parent session cascades the rows away. (AC-13, AC-16)

## Files likely touched

- `apps/backend/internal/task/models/subagent_context.go` — new. Every nullable
  column is a pointer; `ToolUseCount *int64` specifically so a reported `0` is
  distinguishable from "not reported".
- `apps/backend/internal/task/repository/sqlite/subagent_context.go` — new:
  `UpsertSubagentContext`, `ListSubagentContextsBySession`,
  `ListSubagentContextsByTurn`.
- `apps/backend/internal/task/repository/sqlite/subagent_context_test.go` — new.
- `apps/backend/internal/task/repository/sqlite/workspace_test.go` — add
  `task_session_subagents` to `assertNoWorkspaceCascadeDependents` (line ~317).
- The task repository interface in `apps/backend/internal/task/repository/`, if
  the service consumes the repo through an interface rather than the concrete
  type — check before assuming either way.

## Dependencies

Task 01 — the table must exist.

## Parallelism

`sequential` — same package as task 01.

## Inputs

- Plan § *Model and repository upsert (task 02)* — carries the full
  column-by-column conflict-expression table with its AC mapping. Implement from
  that table; do not re-derive it.
- Spec § *Data model*, AC-2a, AC-3 through AC-6, AC-10 through AC-16.
- Upsert patterns already in the package: `review.go:27`, `document.go:291`,
  `executor.go:171`. Build the statement as a package-level `const` and pass it
  through `r.db.Rebind`.
- `git_snapshots.go` — the sibling read/scan shape for a session-scoped child
  table.
- Concurrency test shape: `messagequeue/repository_reorder_test.go:212`.

## Verification

```
cd apps/backend && gofmt -l internal/task/models internal/task/repository/sqlite && make lint && go test -run 'TestSubagentContext' ./internal/task/repository/sqlite/... && go test -race -run 'TestSubagentContext' ./internal/task/repository/sqlite/... && go test ./internal/task/repository/sqlite/...
```

The `-race` run is not optional: AC-14 and AC-15 are concurrency claims and a
non-race pass is not evidence for them.

## Results

Pending. Before marking the task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries
when applicable, or explicitly state `None`.
