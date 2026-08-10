# Task 03 — HTTP delete keeps the caller identity

**Status:** done
**Depends on:** 02

## Goal

`DELETE /api/v1/tasks/:id` authorizes the caller, without reintroducing the
client-disconnect abort that `context.Background()` was there to prevent.

## Changes

`internal/task/handlers/task_http_handlers.go` — `httpDeleteTask`:

```go
deleteCtx, cancel := context.WithTimeout(
    context.WithoutCancel(c.Request.Context()), constants.TaskDeleteTimeout)
```

`context.WithoutCancel` keeps the request context's *values* — the identity the auth
middleware attached is one of them — while dropping its cancellation and deadline.
A browser that navigates away mid-delete no longer aborts a half-finished subtree
teardown, which is what plain `c.Request.Context()` would have reintroduced.

## Tests

`internal/task/handlers/task_delete_authz_test.go` (new). Symbols are `authz`-prefixed
so they cannot collide with the fixtures PR #2500 adds to this package while it is
unmerged.

- `TestHTTPDeleteTaskDeniesForeignTask` — user-a gets 404 with the sanitized body,
  the recording repo saw no delete; **then user-b issues the identical request and
  gets 200**, with the delete recorded.
- `TestHTTPDeleteTaskDeniesForeignTaskThroughCascade` — the same pair with a
  `HandoffService` wired (the production shape), proving task 02's guard is what
  carries this route.
- `TestWSArchiveTaskDeniesForeignTaskThroughCascade` — the third route the cascade
  substitution unscoped.
- `TestHTTPDeleteTaskSurvivesClientDisconnect` — the request context is cancelled
  before the handler runs; the delete must still reach the repository **on a live
  context**. Asserting the status code alone was vacuous (the fake repository
  ignores its context, so a cancelled one still "succeeds") — the mutation run
  caught that, and the test now records `ctx.Err()` at the repository boundary.

## Verification

```
cd apps/backend && CGO_ENABLED=1 go test -tags fts5 -race -count=1 ./internal/task/handlers/...
```

Mutation: restore `context.Background()`; the denial tests must fail. Replace with a
bare `c.Request.Context()`; the disconnect test must fail.
