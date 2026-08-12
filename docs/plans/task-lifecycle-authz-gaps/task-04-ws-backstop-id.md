# Task 04 — WS backstop reads `id` on top-level `task.*` actions

**Status:** done
**Depends on:** —

## Why

`dispatch_scope.go` exists so a *newly added* WS action is scoped by default rather
than only when its author remembers. It reads `task_id`, `session_id` and
`task_environment_id`. `task.state` and `task.move` name their task `id`, so the
backstop parsed no refs and allowed them — the file's own comment predicted this
("an action that invents a fourth name … silently opts out").

The service guards in task 01 are the fix. This closes the class.

## Changes

`internal/gateway/websocket/dispatch_scope.go`

- `authorizeAction(ctx, action, payload)` — takes the action name.
- `scopedActionRefs` gains `ID string \`json:"id"\``, used **only** when
  `isTopLevelTaskAction(action)`: the action is `task.` + exactly one more segment.
- Deeper namespaces stay excluded on purpose. `task.plan.revision.get` and
  `task.review.finding.update` also carry `id`, but it names a revision or a finding;
  treating it as a task ID would deny legitimate reads. Rather than a hand-maintained
  action list — which has the same "someone forgot" failure mode the backstop exists
  to remove — the rule keys off the namespace depth, so a future `task.<verb>` is
  covered without an edit.
- `parseScopedActionRefs` returns ok when any of the four names is present.

`internal/gateway/websocket/client.go` — pass `msg.Action` through.

## Tests

`internal/gateway/websocket/dispatch_scope_test.go`:

- `task.state` / `task.move` / `task.archive` with `{"id": "task-b"}` are refused,
  and the task checker is called with `task-b`.
- The owner's identical action is allowed (the checker admits it), so the rule is not
  a blanket deny.
- `task.plan.revision.get` and `task.review.finding.update` with `{"id": …}` are
  **allowed** — the regression this rule could plausibly cause.
- `task.create` / `task.list` (no `id`) are unaffected.
- A synthetic or identity-less client is unaffected.

## Docs

`apps/backend/CLAUDE.md` — the scoping-model section records which payload names the
backstop reads, including the `task.<verb>` + `id` rule, so the next author knows
what is and is not covered.

## Verification

```
cd apps/backend && CGO_ENABLED=1 go test -tags fts5 -race -count=1 ./internal/gateway/...
```

Mutation: drop the `isTopLevelTaskAction` branch; the `task.state` case must fail.
