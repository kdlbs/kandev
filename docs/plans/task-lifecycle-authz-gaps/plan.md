---
spec: docs/specs/auth/requirements/auth.md
created: 2026-08-10
status: building
---

# Implementation Plan: Task Lifecycle Authorization Gaps

## Overview

Four per-user authorization holes let one authenticated user mutate another user's
task by naming its ID. All four share one root cause: the entry point never asks
`authorizeTaskID`, and the structural backstops that would have caught it do not
see the request.

- **Gaps 1 & 2** — `Service.UpdateTaskState` and `Service.MoveTaskWithOptions` start
  at `s.tasks.GetTask` with no authorize call. Their WS actions (`task.state`,
  `task.move`) name the task `id`, and the gateway backstop only parses `task_id`,
  `session_id` and `task_environment_id`, so it finds no refs and allows the action.
- **Gap 3** — `TaskHandlers.httpDeleteTask` builds its context from
  `context.Background()`. The identity the auth middleware put on the request is
  dropped, so `authorizeTaskID` sees an identity-less internal caller and permits
  everything.
- **Gap 4** (found while verifying gap 3's fix) — the production delete/archive path
  does not reach `Service.DeleteTask` / `Service.ArchiveTask` at all. When a
  `HandoffService` is wired — which `backendapp` always does — the handlers call
  `HandoffService.DeleteTaskTree` / `ArchiveTaskTree`, and `HandoffService` has no
  identity awareness whatsoever. Fixing gap 3's context alone would leave the
  shipped delete route wide open; the fixture in which gap 3 was reproduced has
  `handoffSvc == nil`, which is why the sibling archive route looked correct.

Layering matters here. The service guards are the fix; the gateway backstop is a
structural net that makes the *next* `task.*` action safe by default rather than
only when its author remembers. Per `apps/backend/CLAUDE.md` neither substitutes for
the other, so both land.

Four tasks. 01 and 04 are independent; 02 is meaningless without 03 (the production
route bypasses the guarded service), so they land together in that order.

---

## Backend

### Service layer (`internal/task/service/service_workflow.go`)

`authorizeTaskID` first, before any repo, executor or event-bus use — matching
`UpdateTaskMetadata` (`service_tasks.go:1333`) and `deleteTaskWithReason`
(`service_tasks.go:1911`):

- `UpdateTaskState` — `if err := s.authorizeTaskID(ctx, id); err != nil { return nil, err }`
- `MoveTaskWithOptions` — same, ahead of `s.tasks.GetTask`. The destination
  workflow needs no separate check: `validateTaskMove` already refuses a move
  whose target workflow belongs to a different workspace.
- `UpdateRepositoryBaseBranch` — same, on `req.TaskID`. Its WS payload uses
  `task_id`, so the gateway backstop does cover it today; the check lands anyway
  because a backstop is not a substitute for a service-level guard and the method
  is reachable from HTTP and MCP too.

Denials return the existing `repoerrors.ErrTaskNotFound` sentinel. No handler
change: `wsUpdateTaskState` and `wsMoveTask` already collapse every service error
to a generic `ErrorCodeInternalError` message, which is exactly what a nonexistent
task produces — the denial is indistinguishable from "no such task".

### Cascade layer (`internal/task/service/handoff_service.go`, `handoff_cascade.go`)

Mirror the orchestrator's checker pattern (`internal/orchestrator/service.go:982-1021`)
rather than inventing a second one:

```go
func (s *HandoffService) SetTaskAccessChecker(check func(ctx context.Context, taskID string) error)
func (s *HandoffService) authorizeTask(ctx context.Context, taskID string) error // no-op when unwired
```

Called first in `ArchiveTaskTree`, `DeleteTaskTree` and `UnarchiveTaskTree` — before
the `rootID == ""` and `s.tasks == nil` guards do their repo work. Installed by
`TaskHandlers.SetHandoffService`, which is the call that makes those routes prefer
the cascade over the guarded `Service` methods, so the substitution cannot happen
without the guard.

Unwired stays unscoped, so every existing fixture that builds a bare
`NewHandoffService(repo, nil, nil, nil, nil, nil)` is unaffected.

### HTTP layer (`internal/task/handlers/task_http_handlers.go`)

`httpDeleteTask` becomes:

```go
deleteCtx, cancel := context.WithTimeout(
    context.WithoutCancel(c.Request.Context()), constants.TaskDeleteTimeout)
```

`WithoutCancel` keeps the request's values — the identity among them — while
detaching from the request's cancellation, so a client that disconnects mid-delete
cannot abort a half-finished subtree teardown. Swapping in `c.Request.Context()`
directly would fix the identity and reintroduce that abort.

### WS gateway backstop (`internal/gateway/websocket/dispatch_scope.go`)

`authorizeAction` gains the action name and one rule: for an action of the form
`task.<verb>` — exactly two dot-separated segments — a payload `id` is a task ID.
That covers `task.get/update/delete/move/state/archive` and any future sibling,
without a hand-maintained list to fall out of date.

Deeper namespaces are deliberately excluded: `task.plan.revision.get` and
`task.review.finding.update` also carry `id`, but it names a revision or a finding,
and treating it as a task ID would deny legitimate reads. `task.create` and
`task.list` carry no `id` and are unaffected.

---

## Docs

- `docs/specs/auth/requirements/auth.md` — the behavioral guarantee (lifecycle mutations are
  ownership-checked below the transport; the backstop's `id` rule) plus two
  scenarios.
- `apps/backend/CLAUDE.md` — the scoping-model section gains the `id`-vs-`task_id`
  rule so the next author knows which payload names are covered.

---

## Testing

TDD throughout: denial test first, watched fail, then the guard.

Every denial test also issues the identical request **as the owner** and asserts it
succeeds. Without that witness a denial test passes for the wrong reason — a broken
fixture, a typo'd ID, a missing seed row — and several tests in this area were
vacuous for exactly that reason. Every denied mutation additionally asserts the
repository recorded no write; a guard that refuses the response after committing
the write is still data loss.

New symbols in `internal/task/handlers` are `authz`-prefixed so they cannot collide
with the fixtures PR #2500 adds to the same package while it is unmerged.

Mutation check: each guard is reverted one at a time and the new test must fail and
name itself.

## Tasks

| # | Task | Depends on |
|---|------|-----------|
| 01 | [Service-layer guards for state, move and base-branch](task-01-service-guards.md) | — |
| 02 | [HandoffService cascade authorization](task-02-handoff-cascade-authz.md) | — |
| 03 | [HTTP delete keeps the caller identity](task-03-http-delete-identity.md) | 02 |
| 04 | [WS backstop reads `id` on top-level `task.*` actions](task-04-ws-backstop-id.md) | — |
