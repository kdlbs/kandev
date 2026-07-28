# ADR-2026-07-27-legacy-add-branch-live-rescan: Preserve Live Rescan for Legacy Add Branch

**Status:** accepted
**Date:** 2026-07-27
**Area:** backend, protocol

## Context

`add_branch_to_task_kandev` predates runtime-mutable workspace-source batches. Its worktree-only
contract lets an agent add a repository/branch during its current turn by creating a sibling
worktree and updating Kandev's file and Git trackers without restarting the agent. The workspace
sources implementation routed this compatibility tool through the idle-only host rebind path,
making an in-turn MCP invocation fail and making an idle invocation stop and restart the agent and
agentctl-managed processes.

The compatibility tool also returned repository metadata without the materialized filesystem path.
An agent whose current working directory remains the original worktree could therefore know that
the operation succeeded without knowing the exact sibling path to use.

## Decision

`add_branch_to_task_kandev` remains a worktree-executor compatibility lane separate from
`AttachWorkspaceSources` runtime adoption:

- The tool may run during the invoking task's active turn. Its per-task workspace-source lock,
  validation, transactional repository-row persistence, and rollback behavior remain in force.
- A live materialization creates the new repository/branch worktree as a sibling beneath the
  Kandev-owned task root. It never nests another Git repository inside the current worktree.
- Kandev promotes the persisted task environment `workspace_path` to that task root, asks agentctl
  to rescan its file and repository trackers at the promoted root, and publishes the existing
  materialized-worktree event so the Files and Changes surfaces refresh.
- The live rescan changes tracker scope only. It does not call `RebindWorkspaceForSession`, stop or
  restart the agent, change the running agent's working directory, or stop existing terminals,
  editor servers, dev servers, or other agentctl-managed workspace processes.
- The MCP result includes the materialized `worktree_path`, promoted `task_workspace_path`, and
  `agent_cwd_changed: false`. Materialized paths may be absent for a pre-launch task where durable
  attachment succeeds and worktree creation is intentionally deferred until launch.

`add_workspace_sources_kandev`, the Files-panel action, and
`POST /api/v1/tasks/:id/workspace-sources` retain the batch workspace-source contract from
ADR-2026-07-22-runtime-mutable-task-workspace-sources: they require an idle task and may rebind or
restart a host execution when adopting a new root.

## Consequences

- Agents retain their current ACP conversation, process, working directory, and terminals while a
  sibling worktree becomes visible through Kandev's promoted Files and per-repository Git trackers.
- Agents receive the exact absolute sibling path instead of having to infer or discover it. The
  returned path does not bypass a provider's own filesystem sandbox.
- `add_branch_to_task_kandev` and batch workspace-source attachment deliberately have different
  activity and runtime-adoption rules. Shared persistence helpers must not silently select the
  batch materializer for the compatibility tool.
- Git status in the original worktree remains clean because the new worktree is outside that Git
  working tree.

## Alternatives Considered

1. **Keep routing the compatibility tool through batch workspace adoption.** Rejected because an
   in-session MCP tool call necessarily occurs during a turn, while the batch path requires an idle
   execution and may restart the process that is making the call.
2. **Create the new repository inside the current worktree.** Rejected because the outer
   repository reports the nested repository as untracked, and a broad `git add` stages it as an
   embedded Git link without a valid submodule contract.
3. **Rebind the agent to the task root after every legacy call.** Rejected because it discards the
   compatibility tool's process-continuity guarantee and stops agentctl-managed workspace
   processes.
4. **Return only repository and branch identifiers.** Rejected because the unchanged agent working
   directory gives the caller no reliable path to the new sibling.
