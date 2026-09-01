---
status: current
system: tasks
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
created: 2026-08-30
owners:
  - kandev
---

# Additional Session Workspace Reuse System Design

## Purpose and boundaries

The task system owns the canonical task environment, its physical worktree
inventory, and the execution lifecycle that reuses those resources. The
workspace system supplies repository and worktree resources, while the agent
runtime creates the process that consumes the resolved task workspace.

This design preserves one effective workspace identity across initial launch,
additional-session attachment, backend restart, workspace-only reconstruction,
and agent promotion or resume. It follows
[ADR-2026-08-08-task-owned-worktree-lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001` | [Canonical environment and inventory](#canonical-environment-and-inventory) |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002` | [Recovery and path authority](#recovery-and-path-authority) |

## Canonical environment and inventory

`task_environments` owns the task-level execution environment.
`task_environment_repos` owns the ordered physical repository and worktree
inventory. `task_sessions.task_environment_id` attaches a session to that
environment; an execution ID or repository source path is not workspace
identity.

The initial materializer publishes a ready environment and complete repository
inventory. Additional sessions validate and attach to that environment without
running repository preparation or creating another physical owner. Missing,
creating, inconsistent, or unsupported environments fail through the existing
typed reuse errors.

## Recovery and path authority

`task/service.GetWorkspaceInfoForSession` reconstructs the runtime workspace
from the canonical environment-repository inventory. For a single repository,
the effective workspace is its worktree. For multiple repositories, it is their
task root. The lifecycle manager uses that value as the recovered
`AgentExecution.WorkspacePath` and as the agentctl working directory.

The lifecycle adapter returns both the execution workspace and any optional
worktree metadata. Workspace-only reconstruction can legitimately return the
correct `WorkspacePath` without a `worktree_path` metadata key when the
execution is promoted to an agent execution.

The orchestrator persists an effective workspace using this authority order:

1. The lifecycle response's non-empty `WorkspacePath`, which is the actual
   execution working directory.
2. The response's non-empty `WorktreePath` for legacy adapters that do not
   expose an execution workspace.
3. The request's `RepositoryPath` only when no materialized runtime workspace
   is available.

The source repository path is preparation input. It must not overwrite a
materialized task workspace reported by the lifecycle response.

## Persistence and projection

After successful launch or resume, the orchestrator refreshes
`task_environments.workspace_path` with the authoritative effective workspace.
No schema migration is required. A successful resume under this contract also
self-heals an environment path written incorrectly by the former fallback
order, provided the canonical worktree inventory remains valid.

Task-session reads project the linked environment workspace before the legacy
session-local path. The Files panel and other workspace consumers use that
effective `workspace_path`, with `worktree_path` retained as the primary
repository and compatibility fallback.

## Failure and recovery

- An empty execution workspace and empty worktree path preserve the existing
  source-repository fallback for legacy or non-materialized local sessions.
- A recovered execution workspace always wins over source checkout input,
  including when optional worktree metadata is absent.
- Missing or ambiguous physical inventory fails closed. This path does not
  inspect the filesystem to guess a replacement owner.
- Persistence failure follows the existing launch/resume rollback and task
  environment failure handling.

## Verification

The orchestrator unit boundary covers all workspace-path precedence cases,
including workspace-only recovery with a source repository present. A session
resume Playwright scenario uses the Worktree executor, restarts the backend,
allows automatic promotion/resume, and verifies that the Files path remains
the original worktree path.

The Files path component is shared by desktop and phone layouts. This repair
changes backend state normalization, not layout, navigation, scrolling, or
touch behavior, so the focused desktop recovery scenario plus existing shared
component coverage provides mobile parity.
