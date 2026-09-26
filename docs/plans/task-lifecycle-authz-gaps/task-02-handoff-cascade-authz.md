# Task 02 — HandoffService cascade authorization

**Status:** done
**Depends on:** —

## Why

`backendapp` always wires a `HandoffService` into `TaskHandlers`, so in a shipped
instance `DELETE /tasks/:id`, `POST /tasks/:id/archive` and the `task.archive` WS
action never reach the guarded `Service.DeleteTask` / `Service.ArchiveTask` — they
go to `HandoffService.DeleteTaskTree` / `ArchiveTaskTree`, which have no identity
awareness at all. Task 03's identity fix is inert without this.

This is why the sibling archive route looked correctly scoped: it was reproduced
against a fixture with `handoffSvc == nil`.

## Changes

`internal/task/service/handoff_service.go`

- `taskAccessCheck func(ctx context.Context, taskID string) error` field.
- `SetTaskAccessChecker(...)` — same contract as
  `orchestrator.Service.SetTaskAccessChecker`: the checker returns nil for
  identity-less contexts, and leaving it unwired keeps the pre-auth behavior.
- `authorizeTask(ctx, taskID)` — no-op when unwired or when taskID is empty.

`internal/task/service/handoff_cascade.go`

- `authorizeTask` as the first statement of `ArchiveTaskTree`, `DeleteTaskTree` and
  `UnarchiveTaskTree`.

`internal/task/handlers/task_handlers.go`

- `TaskHandlers.SetHandoffService` installs the checker
  (`svc.SetTaskAccessChecker(h.service.AuthorizeTaskAccess)`).

The wiring lives there rather than in `backendapp` on purpose: `SetHandoffService`
is the call that makes archive/delete/unarchive use the cascade *instead of* the
guarded `Service` methods, and every user-facing cascade call site is in this
package. Installing the guard as part of that substitution means it cannot be
wired without it, and it is reachable from a package test — a separate
`backendapp` line would have had no pin short of booting the whole route table.
The integration watch-reset callers share the same instance and are identity-free,
so the checker no-ops for them.

## Tests

`internal/task/service/handoff_cascade_authz_test.go` (new):

- `TestCascadeEntryPointsDenyForeignTask` — table over the three entry points with a
  checker that admits `task-mine` and refuses `task-b`. Each asserts the denial, that
  the outcome is nil, and that the recording repo saw **no** archive/delete/unarchive
  call; **then re-runs the same entry point on the owned task** and asserts it
  proceeds past the guard.
- `TestCascadeEntryPointsGuardBeforeDependencies` — the same table with `s.tasks`
  nil; a panic means the guard runs too late.
- `TestCascadeEntryPointsUnscopedWhenUnwired` — no checker installed, the denial
  sentinel must not appear, so every existing fixture that builds a bare
  `NewHandoffService(...)` keeps working.

## Verification

```
cd apps/backend && CGO_ENABLED=1 go test -tags fts5 -race -count=1 ./internal/task/... ./internal/backendapp/...
```

Mutation: remove each `authorizeTask` call in turn.
