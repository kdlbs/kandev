---
id: "06-ws-actions-and-dtos"
title: "Review WS actions, DTOs, and E2E reset cascade"
status: done
wave: 4
depends_on: ["04-run-orchestration"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 06: Review WS actions, DTOs, and E2E reset cascade

The client-facing contract for triggering and reading a review.

## Inputs

- Spec **API surface** → WebSocket actions, including the rejection codes.
- Pattern: the `task.walkthrough.get` / `task.walkthrough.delete` plain (non-MCP) actions in `internal/mcp/handlers/handlers.go`, and the DTO+`ToAPI` convention in `pkg/api/v1/`.
- `apps/backend/AGENTS.md` → "E2E reset invariant": any new task-scoped entity needs a workspace cascade in `cmd/kandev/e2e_reset.go`.

## Work

1. `pkg/api/v1/` — `TaskReviewRunDTO`, `TaskReviewFindingDTO` with `ToAPI` converters. Field names must match the frontend types in task 08.
2. `pkg/websocket/actions.go` — `task.review.run`, `task.review.cancel`, `task.review.get`, `task.review.finding.update`, `task.review.clear`.
3. Handlers (in the same dispatcher that owns the walkthrough plain actions):
   - `task.review.run` — validates `task_id`; returns `review_no_changes` / `review_agent_unavailable` as a WS error with that code in the error payload; otherwise creates the run, launches it detached, and returns the `pending` run.
   - `task.review.cancel` — cancels by run id; idempotent for an already-terminal run.
   - `task.review.get` — `{runs, findings}`.
   - `task.review.finding.update` — validates the target status against the enum.
   - `task.review.clear`.
4. `cmd/kandev/e2e_reset.go` — delete `task_review_findings` then `task_review_runs` for the workspace before task deletion; add the repository cascade method it needs.

## Acceptance

- Each action returns the documented shape; unknown status on `finding.update` is a 400-equivalent WS error.
- `task.review.run` on a task with no changes creates no run row.
- E2E reset leaves both tables empty for the worker's workspace.

## Verification

```
cd apps/backend && go test ./internal/mcp/... ./pkg/api/... ./cmd/kandev/...
cd apps/backend && make lint
```

## Files likely touched

`pkg/api/v1/`, `pkg/websocket/actions.go`, `internal/mcp/handlers/handlers.go`, `internal/task/repository/sqlite/review.go`, `cmd/kandev/e2e_reset.go`.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
