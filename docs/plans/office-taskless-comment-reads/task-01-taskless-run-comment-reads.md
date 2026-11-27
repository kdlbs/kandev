---
id: "01-taskless-run-comment-reads"
title: "Taskless run comment reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENT-COMMENT-READS-001
  - REQ-OFFICE-AGENT-COMMENT-READS-009
acceptance_criteria:
  - AC-OFFICE-AGENT-COMMENT-READS-001.13
  - AC-OFFICE-AGENT-COMMENT-READS-009.1
  - AC-OFFICE-AGENT-COMMENT-READS-009.2
  - AC-OFFICE-AGENT-COMMENT-READS-009.3
  - AC-OFFICE-AGENT-COMMENT-READS-009.4
  - AC-OFFICE-AGENT-COMMENT-READS-009.8
system_design:
  - ../../specs/office/system-design/agent-comment-reads-01.md
---

# Task 01: Taskless run comment reads

## Summary

Let a taskless run caller (empty task claim, non-empty run claim, non-empty
workspace claim) read comments on any task in its own workspace. Every other
agent caller keeps today's behavior.

## Regression test (write first, must fail on main)

`apps/backend/internal/office/dashboard/handler_comments_agent_read_test.go`:

- `TestListComments_TasklessRunReadsTaskInWorkspace`: mint
  `MintRuntimeJWT(agent.ID, "", agent.WorkspaceID, "run-1", "sess-1", "")`,
  seed an unrelated task in the same workspace with two comments, then expect
  `200` and a window of two comments. On main this returns
  `403 {"error":"document access denied"}`.
- `TestListComments_TasklessRunDeniedCrossWorkspaceAndMissing`: foreign-workspace
  task and nonexistent id both return `403` with identical bodies.
- `TestListComments_TasklessTokenWithoutRunStillDenied`: empty task and empty
  run claims return `403` for a same-workspace task (AC-001.13).
- `TestListComments_TaskBoundRunKeepsRelationGuard`: a task-bound token reading
  an unrelated same-workspace task still gets `403` (AC-009.4).

`apps/backend/internal/task/service/handoff_comments_test.go`:

- `TestListCommentsForTasklessRun`: table test covering same workspace (window
  returned), foreign workspace, missing target, empty workspace argument (all
  `ErrAccessDenied`), and a lookup error (returned as-is, not
  `ErrAccessDenied`).
- Keep `TestListCommentsForCallerDeniesEveryTargetWhenCallerTaskEmpty`
  unchanged: `ListCommentsForCaller` still denies an empty caller task.

## Implementation

- `apps/backend/internal/task/service/handoff_comments.go`: extract the body of
  `ListCommentsForCaller` after the guard into a private
  `commentWindow(ctx, targetTaskID, limit)`. Add
  `ListCommentsForTasklessRun(ctx, workspaceID, targetTaskID, limit)`: trim
  inputs; deny empty ones; `s.tasks.GetTask`; map
  `repository.ErrTaskNotFound` and nil to `ErrAccessDenied`; return other errors
  unchanged; deny `task.WorkspaceID != workspaceID`; then `commentWindow`.
- `apps/backend/internal/office/dashboard/handler_comments.go`: in
  `listCommentsForAgent`, select the entry point from the claims per the
  design's table (`strings.TrimSpace(claims.TaskID) == ""`, non-empty
  `claims.RunID`, non-empty trimmed `claims.WorkspaceID`). Keep the existing
  error mapping (`ErrAccessDenied` → `403`, otherwise `500`).
- `apps/backend/internal/office/configloader/instructions/ceo/HEARTBEAT.md`,
  step 6: read a task's comments with
  `$KANDEV_CLI kandev comment list --task <task-id>` before judging it stalled.
- No change to `office_scope.go`: `isAgentCommentRead` already defers this
  route to the handler.

## Verification

```bash
cd apps/backend
go test ./internal/task/service/ -run 'ListComments' -count=1
go test ./internal/office/dashboard/ -run 'Comment' -count=1
go test ./internal/office/configloader/... -count=1
golangci-lint run ./internal/task/service/... ./internal/office/dashboard/... --timeout=5m
```

## Results

Implemented via TDD (tests written first, watched RED for the missing method
and for the un-dispatched handler branch, then made GREEN):

- `handoff_comments.go`: extracted `commentWindow(ctx, targetTaskID, limit)`
  from `ListCommentsForCaller`; added `ListCommentsForTasklessRun(ctx,
  workspaceID, targetTaskID, limit)` (trims inputs, denies empty; `GetTask`;
  `repository.ErrTaskNotFound`/nil → `ErrAccessDenied`; other errors returned
  unchanged; workspace mismatch → `ErrAccessDenied`; otherwise shared window).
- `handler_comments.go` `listCommentsForAgent`: dispatches to
  `ListCommentsForTasklessRun` when `strings.TrimSpace(claims.TaskID)==""` and
  `claims.RunID!=""` and `strings.TrimSpace(claims.WorkspaceID)!=""`; else the
  existing `ListCommentsForCaller` path. Same error mapping.
- `configloader/instructions/ceo/HEARTBEAT.md` step 6: added
  `$KANDEV_CLI kandev comment list --task <task-id>` before judging a task
  stalled (AC-009.8), matching the existing `AGENTS.md` convention for the
  same CLI invocation.
- Tests added: `handoff_comments_test.go`
  (`TestListCommentsForTasklessRun` table test covering same-workspace read,
  foreign-workspace deny, missing-target deny, empty-workspace/-target deny,
  and lookup-error passthrough via a new `erroringTaskLookup` stub;
  `TestListCommentsForCallerStillDeniesUnrelatedSameWorkspaceTask` for
  AC-009.4); `handler_comments_agent_read_test.go` (added
  `TestListComments_TasklessRunReadsTaskInWorkspace`,
  `TestListComments_TasklessRunDeniedCrossWorkspaceAndMissing`,
  `TestListComments_TasklessTokenWithoutRunStillDenied`,
  `TestListComments_TaskBoundRunKeepsRelationGuard`).

Verification:
```
$ go test ./internal/task/service/ -run 'ListComments' -count=1
ok  	github.com/kandev/kandev/internal/task/service	2.217s
$ go test ./internal/office/dashboard/ -run 'Comment' -count=1
ok  	github.com/kandev/kandev/internal/office/dashboard	1.460s
$ go test ./internal/office/configloader/... -count=1
ok  	github.com/kandev/kandev/internal/office/configloader	2.607s
```
