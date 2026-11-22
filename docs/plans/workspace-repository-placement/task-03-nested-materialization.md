---
id: "03-nested-materialization"
title: "Materialize nested repositories safely"
status: completed
wave: 3
depends_on: ["01-durable-layout"]
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.8
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.9
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.10
system_design:
  - ../../specs/tasks/system-design/workspace-repository-placement.md
---

# Task 03: Materialize nested repositories safely

## Summary

Add backend placement preview and real nested worktrees without changing the running agent root.
Preserve repository-aware tracking, Git isolation, and cleanup ownership.

## In scope

- Read-only source preview route and typed placement/revision fields on repository-only Worktree batches.
- Owned internal relative destinations through the worktree manager and canonical attachment inventory.
- Gitdir-conditional excludes, ownership receipts, compensation, and same-common-directory serialization.
- Rescan-only adoption with complete source roots. Retain outer and inner repository trackers.
- Durable reuse and child-before-parent cleanup with preservation checks.

## Out of scope

UI placement cards, root-changing recovery, folder placement, and changing legacy add-branch semantics.

## Acceptance

1. Both nested placements create real worktrees inside the established root. Agent and terminal identities remain unchanged, including exact retries.
2. Ordinary outer status/staging excludes only owned nested entries. Inner changes remain visible, and a sibling worktree's same-named ordinary folder remains visible.
3. Collisions, stale preview, root mismatch, path escape, invalid batches, and partial failure leave prior work intact. Cleanup never removes a protected nested child through its parent.

## Tests and TDD

Create proposed `workspace_placement_test.go` files in backendapp, worktree, task service, and task handlers.
Use `TestWorkspacePlacementPreview`, `TestWorkspacePlacementNestedBatch`, `TestWorkspacePlacementGitExclusion`, and `TestWorkspacePlacementCleanup`.
Add `TestWorkspacePlacementTrackers` in agentctl process tests with real nested Git worktrees and an ignored child.
Cover repeated same-repository branch slots, spaces, case collisions, symlink/junction parents, tracked/untracked destinations, and an existing `kandev/` directory.
Cover existing user excludes, negations that defeat exclusion, concurrent shared-config edits, and rollback without overwriting later user edits.
Run native `git add .` in the outer repository and assert no embedded gitlink. Force-add is outside the promised protection.
Inject failure on the second source and during rescan. Assert batch rollback and unchanged prior roots.
A restart/reuse test must prove stored nested paths survive even after the repository count becomes two or more.

## Verification

```bash
(cd apps/backend && go test ./internal/worktree ./internal/backendapp ./internal/task/service ./internal/task/handlers ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process -run 'WorkspacePlacement|AddBranchToTask_ActiveTurnUsesLegacyMaterializer|BranchMaterializer|WorkspaceSources' -count=1)
(cd apps/backend && go test -race ./internal/worktree ./internal/backendapp ./internal/task/service -run 'WorkspacePlacement' -count=1)
```

Run path/config tests on Linux, macOS, and Windows in available CI/native environments. A Linux pass does not prove Windows path safety.

## Files likely touched

- `apps/backend/internal/task/service/service_workspace_sources.go` and source batch persistence helpers
- `apps/backend/internal/task/handlers/task_handlers.go` and the existing `httpAttachWorkspaceSources` handler owner
- `apps/backend/internal/backendapp/workspace_source_materializer.go`, `branch_materializer.go`
- `apps/backend/internal/worktree/worktree.go`, `manager_lifecycle.go`, `manager_cleanup.go`, `config.go`
- Proposed focused `workspace_placement.go` and `workspace_placement_git_excludes.go` in `internal/worktree/`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rescan.go` and source-root reconciliation
- `apps/backend/internal/agentctl/server/process/` workspace tracking, rescan, file traversal, and staging guards
- Task/environment DTO projections for the new source preview and placement result

## Dependencies

Task 01. Execute after Task 02 in this session, but the technical dependency is durable identity only.

## Inputs

Design: Placement; Preview contract; Nested worktrees; Tracking and cleanup.
Existing batch compensation and `manager_rescan_test.go` provide the source and tracker patterns.

## Risks

A tracker rescan cannot widen provider permissions. A symlink to a sibling is not a valid nested materialization.
Git configuration writes affect a shared administrative file even when their condition is worktree-specific. Keep exact ownership and concurrency tests.


## Parallelism

`sequential`

## Results

Implemented placement preview and revision checks, nested worktree materialization, Git exclusion ownership, exact-path persistence, lifecycle reconciliation, multi-branch identity, and rollback cleanup after partial materialization or rescan failure. The changed backend package suite, worktree tests, store-conformance race check, backend lint, SQL guard, and built backend passed. Desktop and phone Add sources regression flows passed. Native provider sandbox behavior and non-Linux path evidence remain unverified.
