---
spec: docs/specs/tasks/system-design/attach-workspace-sources.md
decision: docs/decisions/2026-07-27-legacy-add-branch-live-rescan.md
created: 2026-07-27
status: completed
---

# Implementation Plan: Restore Live Add-Branch Compatibility

## Overview

Restore `add_branch_to_task_kandev` as a worktree-only live compatibility path while retaining the
transactional persistence introduced by workspace-source batches. The repair first pins active-turn
routing and materialized-path propagation in the backend, then exposes those paths through the MCP
result and updates public documentation. The Files-panel and `add_workspace_sources_kandev` batch
flow remain idle-only and may continue to rebind or restart host executions.

## Confirmed Root Cause

PR #1900 added two incompatible behaviors to `Service.AddBranchToTask`:

1. `workspaceSourcesIdle` rejects the MCP mutation while the invoking agent has the active turn
   required to make the tool call.
2. Production wiring sets `workspaceSourceMaterializer`, causing `AddBranchToTask` to replace
   `materializeLegacyBranch` with `materializeWorkspaceSources`. On a live Worktree environment that
   reaches `RebindWorkspaceForSession`, which stops and restarts the agent instead of using the
   existing tracker-only rescan.

The preserved `branchMaterializer` already creates the sibling worktree, promotes the persisted
workspace path, calls `RescanWorkspaceForSession`, and publishes the frontend materialization event.
The missing contract is propagation of its materialized worktree and task-root paths back through
the service and MCP response.

---

## Backend

### Live compatibility result contract

Files:

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_branches.go`

Changes:

- Add `BranchMaterializationResult` with `WorktreePath` and `TaskWorkspacePath`.
- Change `BranchMaterializer.MaterializeBranch` to return
  `(*BranchMaterializationResult, error)`; `nil` means pre-launch/deferred materialization.
- Add `AddBranchToTaskResult` containing the persisted `TaskRepository`, optional materialized
  paths, and `AgentCWDChanged` fixed to `false`.
- Carry the legacy materialization result through the existing transactional
  `commitWorkspaceSourceBatch` callback without changing the batch materializer's public behavior.
- Keep the per-task workspace-source lock, repository validation, atomic row persistence,
  compensation, and post-materialization `task.updated` publication.

### Active-turn routing and sibling materialization

Files:

- `apps/backend/internal/task/service/service_branches.go`
- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/backendapp/workspace_source_materializer.go`

Changes:

- Remove the `workspaceSourcesIdle` gate only from `AddBranchToTask`.
- Always pass the legacy branch materializer to `commitWorkspaceSourceBatch`, even when the
  production `WorkspaceSourceMaterializer` is configured.
- Return the created worktree path and the effective promoted task root from
  `branchMaterializer.MaterializeBranch`.
- Preserve `promoteWorkspacePathIfNeeded`, `RescanWorkspaceForSession`, and
  `NotifyWorktreeMaterialized` as the live adoption sequence.
- Do not call `RebindWorkspaceForSession`, mutate the live execution CWD, stop the agent, or stop
  agentctl-managed processes.
- Leave `AttachWorkspaceSources` idle gating and workspace-source materialization unchanged.

### MCP response

Files:

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/server/server.go`

Changes:

- Map `AddBranchToTaskResult` to the existing response fields plus optional `worktree_path`,
  optional `task_workspace_path`, and `agent_cwd_changed: false`.
- Omit the two path fields when materialization is deferred until launch.
- Update the tool description to state sibling placement, unchanged CWD/processes, and that the
  returned path is the location the agent should use.
- Keep task scoping, locator mutual exclusion, and request arguments unchanged.

---

## Frontend

No frontend production change. The existing materialized-worktree
`session.agentctl_ready` handling already repoints the Files tree to `task_workspace_path`, records
the new worktree, and refreshes repository-aware surfaces. The compatibility repair must continue
to publish that event rather than introducing another browser contract.

---

## Public Documentation

Files:

- `docs/public/automation-and-mcp.md`
- `docs/public/coordination.md`
- `docs/public/feature-status.md`

Changes:

- Distinguish idle-only batch source attachment from live
  `add_branch_to_task_kandev`.
- Document sibling placement, unchanged agent/terminal CWD, no process restart, exact returned path,
  and clean outer-repository Git status.
- Keep the recommendation to use `add_workspace_sources_kandev` for new multi-source automation
  when its idle/rebind semantics are acceptable.

---

## Tests

- **What:** an active turn may call `AddBranchToTask` even when both production materializers are
  configured; only `BranchMaterializer` is invoked.
  **File:** `apps/backend/internal/task/service/service_branches_test.go`
  **How:** replace the active-task rejection regression with spies for both materializers and assert
  legacy call count `1`, workspace-source call count `0`, persisted row count `2`, and returned
  paths.
- **What:** a live branch materialization returns the sibling worktree path and promoted task root
  while retaining the live rescan/event sequence.
  **File:** `apps/backend/internal/backendapp/branch_materializer_test.go`
  **How:** extend the existing real-Git happy path to assert the returned result and the same single
  rescan/materialization notification.
- **What:** pre-launch add-branch persists successfully with omitted materialized paths.
  **File:** `apps/backend/internal/task/service/service_branches_test.go`
  **How:** use the existing pre-launch fixture with a nil materialization result and assert the new
  service result contract.
- **What:** a live materialization error compensates the repository attachment without routing to a
  restart-capable materializer.
  **File:** `apps/backend/internal/task/service/service_branches_test.go`
  **How:** inject a failing legacy materializer plus a workspace-source spy and assert rollback and
  call counts.
- **What:** the MCP WS handler exercises handler → service → SQLite persistence and returns
  `worktree_path`, `task_workspace_path`, and `agent_cwd_changed: false`.
  **File:** `apps/backend/internal/mcp/handlers/handlers_test.go`
  **How:** build the real task service/repository fixture, seed an active turn and Worktree
  environment, inject a branch-materializer stub returning paths, invoke
  `handleAddBranchToTask`, and inspect the response plus durable row.
- **What:** the task-mode MCP tool preserves the backend response and advertises the no-restart
  sibling contract.
  **File:** `apps/backend/internal/mcp/server/handlers_test.go`,
  `apps/backend/internal/mcp/server/server_test.go`
  **How:** return a path-bearing fake backend response, parse the tool result JSON, and inspect the
  registered tool description.
- **What:** tracker-only rescan still transitions the Files root from the primary repository to the
  task root containing both sibling repositories.
  **File:** existing `apps/backend/internal/agentctl/server/process/manager_rescan_test.go`
  **How:** rerun the focused single-to-multi and file-tree-root regression tests unchanged.

## E2E Tests

No browser E2E change. The repair changes a task-mode MCP response and restores an already-covered
backend event/file-tree path; focused backend integration and existing process-manager tests are
the faithful validation surface.

---

## Implementation Waves

Wave 1:

- [x] [task-01-restore-live-materialization](task-01-restore-live-materialization.md)

Wave 2:

- [x] [task-02-return-materialized-paths](task-02-return-materialized-paths.md)

Wave 3:

- [x] [task-03-update-public-docs](task-03-update-public-docs.md)

Execution is sequential in the primary conversation. No task is marked parallel-safe, and these
waves do not authorize subagents.

## Risks

- `AddBranchToTask` runs inside an active turn, so its transaction and rescan must remain bounded
  and must not wait for the turn to become idle.
- Interface result changes affect service and backendapp test stubs even though production behavior
  changes only for the legacy MCP path.
- A provider-owned filesystem sandbox may still exclude a sibling path. Returning the path is
  informational and must not be described as bypassing that sandbox.
- Publishing both legacy materialization and workspace-source adoption events would duplicate
  refreshes. The legacy path should retain its existing materialized-worktree event and avoid
  inventing a second session-adoption event.

## Out of Scope

- Making `add_workspace_sources_kandev` callable during an active turn.
- Changing the live agent or terminal CWD.
- Nesting repositories inside the current Git worktree.
- Widening provider-owned filesystem sandboxes.
- Changing non-Worktree executor support for `add_branch_to_task_kandev`.

## Open Questions

None.
