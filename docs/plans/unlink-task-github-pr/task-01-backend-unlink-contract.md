---
id: "01-backend-unlink-contract"
title: "Backend unlink contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/link-existing-task-github-issue.md"
---

# Task 01: Backend Unlink Contract

## Acceptance

- One workspace-authorized endpoint detaches exactly one GitHub task-PR
  association, leaves sibling associations and the remote PR untouched, and
  fails closed for unknown or cross-workspace IDs.
- Detached associations survive restart/migration replay, disappear from all
  active association reads and automation inputs, are not resurrected by
  automatic watch discovery, and can be restored by explicit URL linking.
- A successful detach publishes a workspace-routed `github.task_pr.deleted`
  notification only after persistence succeeds.

## Verification

```bash
cd apps/backend && go test ./internal/github ./internal/gateway/websocket ./pkg/websocket
```

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_pr_watch.go` or a focused sibling
- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/*_test.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/internal/gateway/websocket/task_notifications_test.go`

## Dependencies

None.

## Parallelism

Parallel-safe with Task 04 only: implementation and public-doc files are
disjoint, and Task 04 does not depend on generated contracts.

## Inputs

- Spec sections: `What`, `Scenarios`, `Data And API`, `Failure Modes`.
- Plan sections: `Durable association detachment`, `Service and HTTP contract`,
  `Live update contract`.
- Patterns: GitLab's workspace-scoped `DeleteTaskMRForWorkspace`, GitHub's
  `ReplaceTaskPR`, and workspace-routed task notification payloads.

## Risks

- Existing databases and both supported SQL dialects must receive the nullable
  column through replay-safe migration order.
- Internal automatic association paths must not clear the tombstone intended
  for explicit relinking only.

## Output contract

Report the behavioral tests added, files changed, exact command result,
remaining risks/blockers, and update this task plus `plan.md` status in the same
conversation.
