---
status: current
system: tasks
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
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
executor transition, and agent promotion or resume. It follows
[ADR-2026-08-08-task-owned-worktree-lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001` | [Canonical environment and inventory](#canonical-environment-and-inventory) |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002` | [Recovery and path authority](#recovery-and-path-authority) |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003` | [Executor transition](#executor-transition), [Launch admission](#launch-admission) |

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

No session row, sibling execution, or cached frontend projection is a workspace
authority. In particular, `task_sessions.workspace_path` and lifecycle
`WorkspaceInfo.WorkspacePath` cannot recover an environment that fails the
canonical attach checks.

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

## Executor transition

An executor-type mismatch changes the environment selection outcome; it is not
merely a request-metadata filtering decision. Environment reuse correctly skips
runtime handles when the executor type changes, but a later lifecycle path can
still create a local execution from the old persisted `workspace_path`. A
missing or non-Git directory can therefore be reported as ready and receive an
agent process even though it is not the selected task repository. Before
lifecycle preparation, the orchestrator must either:

1. select a ready environment owned by the requested executor and validate its
   complete repository inventory; or
2. elect one fresh materializer for the requested executor under the existing
   task and repository identities.

The previous environment stays recorded for diagnostics and cleanup policy, but
none of its workspace path, repository rows, worktree identifiers, execution
identifiers, container identifiers, or runtime metadata is forwarded to the new
launch. The durable environment row is rebound to the new executor only after
the target executor has launched successfully, with backend-specific handles
cleared before response values are applied.

The transition does not create a second task and does not silently repair,
delete, move, or rewrite the old workspace.

## Launch admission

For a repo-backed local or worktree execution, readiness requires both durable
inventory validation and filesystem validation:

- the selected environment executor type equals the requested executor type;
- every repository and branch slot has exactly one active canonical inventory
  row;
- the workspace path exists and resolves to a Git working tree whose repository
  identity matches the selected slot; and
- the validated path belongs to the selected environment, not a stale session
  projection.

Validation is read-only and uses the existing bounded Git inspection path. It
does not read file contents or alter the index, HEAD, branch, tracked files, or
untracked files. Remote executors validate through their executor-owned
inventory contract rather than host filesystem inspection.

The lifecycle manager retains a defense-in-depth guard: if a repo-backed
`WorkspaceInfo` reaches execution creation without the validated-environment
marker and exact selected environment identity, it refuses to create the
execution. It never treats a non-empty path as sufficient proof.

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

The task is not projected as environment-ready until launch admission passes.
Files, changes, terminal, and agent-start surfaces use only the selected and
validated environment. Cached changes from a rejected workspace may remain in
historical records but cannot become the current task change projection.

## Failure and recovery

- An empty execution workspace and empty worktree path preserve the existing
  source-repository fallback for legacy or non-materialized local sessions.
- A recovered execution workspace always wins over source checkout input,
  including when optional worktree metadata is absent.
- Missing or ambiguous physical inventory fails closed. This path does not
  inspect the filesystem to guess a replacement owner.
- Executor mismatch or invalid workspace admission returns a typed, recoverable
  `workspace_reuse_unsafe` result before agent process startup. The safe
  response identifies the failure category and retry eligibility but does not
  disclose filesystem paths, branch names, credentials, or executor secrets.
- Failure preserves all existing task, session, environment, repository, and
  filesystem objects. A later launch may retry only after the control plane has
  a valid selected environment; it does not perform automatic cleanup or
  fallback to a repository path outside that environment.
- Persistence failure follows the existing launch/resume rollback and task
  environment failure handling.

## Verification

The orchestrator unit boundary covers all workspace-path precedence cases,
including workspace-only recovery with a source repository present. A session
resume Playwright scenario uses the Worktree executor, restarts the backend,
allows automatic promotion/resume, and verifies that the Files path remains
the original worktree path.

Executor-transition coverage adds:

- Prove an executor-type transition cannot forward the previous environment
  workspace path or runtime handles.
- Prove a missing and a non-Git local workspace fail before execution creation
  and before agent process startup.
- Prove a valid canonical local repository and a valid worktree attachment keep
  current behavior.
- Prove rejection does not mutate Git state, filesystem contents, environment
  inventory, or the canonical repository.
- Prove current task projections do not select cached changes from the rejected
  workspace.

The Files path component is shared by desktop and phone layouts. This repair
changes backend state normalization, not layout, navigation, scrolling, or
touch behavior, so the focused desktop recovery scenario plus existing shared
component coverage provides mobile parity.
