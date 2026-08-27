# Task 01 — Service-layer guards for state, move and base-branch

**Status:** done
**Depends on:** —

## Goal

`Service.UpdateTaskState`, `Service.MoveTaskWithOptions` and
`Service.UpdateRepositoryBaseBranch` refuse a task the caller cannot see, before
they read or write anything.

## Changes

`internal/task/service/service_workflow.go`

- `UpdateTaskState` — `authorizeTaskID(ctx, id)` as the first statement, ahead of
  `s.tasks.GetTask`.
- `MoveTaskWithOptions` — same. `MoveTask` delegates here, so both entry points are
  covered by one guard.
- `UpdateRepositoryBaseBranch` — `authorizeTaskID(ctx, req.TaskID)` first. Defense in
  depth: the WS action names `task_id` so the gateway backstop already covers that
  transport, but the method is also reachable from HTTP and MCP, and
  `apps/backend/CLAUDE.md` requires the service-level check regardless.

No handler changes. `wsUpdateTaskState` and `wsMoveTask` map every service error to
a generic `ErrorCodeInternalError` message, which is byte-identical to what a
nonexistent task already produces — so the denial leaks no existence.

## Tests

`internal/task/service/service_workflow_authz_test.go` (new), using the
`ctxAs` / `seedScopedWorkspaces` / `createTestService` fixtures already in
`service_access_test.go`:

- `TestUpdateTaskStateDeniesForeignTask` — user-a is refused with
  `repoerrors.ErrTaskNotFound`, the returned task is nil, and the repository state
  is unchanged; **then user-b runs the identical call and it succeeds** and the
  state actually changes.
- `TestMoveTaskDeniesForeignTask` — same shape; asserts the task's
  workflow/step/position are untouched after the denial, and that the owner's move
  lands.
- `TestUpdateRepositoryBaseBranchDeniesForeignTask` — same shape.
- `TestTaskWorkflowGuardsPrecedeRepositoryUse` — table over the three entry points
  with a `Service` whose repos are nil except the two the guard itself needs; a
  panic means the guard is placed after the first repo use.

## Verification

```
cd apps/backend && CGO_ENABLED=1 go test -tags fts5 -race -count=1 ./internal/task/service/...
```

Mutation: remove each `authorizeTaskID` line in turn; the matching test must fail
and name itself.
