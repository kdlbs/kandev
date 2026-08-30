---
id: "01-restore-live-materialization"
title: "Restore live add-branch materialization"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
---

# Task 01: Restore live add-branch materialization

## Acceptance

- An active Worktree task can execute `Service.AddBranchToTask` while both the legacy branch and
  batch workspace-source materializers are configured.
- The operation invokes only the legacy materializer, creates/persists the sibling attachment, and
  returns its worktree and promoted task-root paths without stopping or rebinding the execution.
- Pre-launch success returns no materialized paths, and live materialization failure compensates the
  new attachment.

## Verification

```bash
cd apps/backend && rtk go test ./internal/task/service -run 'TestAddBranchToTask' -count=1
cd apps/backend && rtk go test ./internal/backendapp -run 'TestBranchMaterializer' -count=1
cd apps/backend && rtk go test ./internal/agentctl/server/process -run 'TestRescanRepositories_(TransitionsSingleToMultiRepo|FileTreeReturnsTaskRootContentsAfterTransition)$' -count=1
```

## Files Likely Touched

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_branches.go`
- `apps/backend/internal/task/service/service_branches_test.go`
- `apps/backend/internal/task/service/service_branches_qa_test.go`
- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/backendapp/branch_materializer_test.go`
- `apps/backend/internal/backendapp/workspace_source_materializer.go`
- `apps/backend/internal/backendapp/workspace_source_materializer_test.go`

## Dependencies

None.

## Parallelism

`sequential`. This task establishes the shared service/materializer result contract consumed by
Task 02.

## Inputs

- Spec sections: **What**, **API surface**, **Failure modes**, and the three legacy add-branch
  scenarios.
- ADR-2026-07-27-legacy-add-branch-live-rescan.
- Existing patterns:
  `branchMaterializer.finalizeMaterialize`,
  `Service.commitWorkspaceSourceBatch`,
  `TestBranchMaterializer_PromotesWorkspacePathAndTriggersRescan`, and
  `TestAddBranchToTask_RejectsActiveTaskBeforePersisting`.

## TDD Sequence

1. Replace the active-task rejection expectation with a path-bearing active-turn regression and
   configure spies for both materializers.
2. Run the focused service test and confirm RED because the active-turn gate rejects the call.
3. Remove only the legacy gate/reroute, implement the minimal result propagation, and reach GREEN.
4. Extend the real-Git branch materializer test to assert sibling and task-root paths.
5. Run the exact verification commands above.

## Output Contract

Report the expected RED failure, minimal routing/result changes, files changed, exact command
results, any interface-stub updates, risks, and blockers. Mark this task `done` and update its plan
checkbox in the primary conversation.
