# 0012: Multi-branch task support — extend multi-repo to allow N branches per repo

**Status:** accepted
**Date:** 2026-06-01
**Area:** backend, frontend

## Context

The `task_repositories` table previously enforced `UNIQUE(task_id, repository_id)`, which meant a task could only attach a given repository once. In practice users wanted a single task to open multiple PRs against the same repo — feature flag rollouts, stacked PRs, A/B-of-the-same-change experiments, or simply two branches that should be reviewed together. The only workaround was creating sibling tasks, which fragmented the conversation, the agent's context, and the kanban view.

The data model already carried `TaskRepository.CheckoutBranch` per row — the column existed for "checkout this specific branch in the worktree" semantics — but the unique constraint on `(task_id, repository_id)` prevented two rows from coexisting even when their `CheckoutBranch` differed.

## Decision

A task can hold N `(repository_id, checkout_branch)` rows, including multiple branches on the same repo. PRs, worktrees, sessions, and review surfaces all key on the pair instead of just `repository_id`.

- **Schema.** `task_repositories` now uses `UNIQUE(task_id, repository_id, base_branch, checkout_branch)`. Both branch columns participate because the two executor shapes carry the branch differently: the worktree executor anchors the branch in `base_branch` with `checkout_branch` empty, while the local executor splits them (`base_branch` = repo default, `checkout_branch` = chosen branch). Either column on its own is insufficient to disambiguate. An idempotent migration in `apps/backend/internal/task/repository/sqlite/base_migrations.go` (`migrateTaskRepositoriesAllowMultiBranch`) rebuilds existing tables; it runs twice with two trigger phrases so DBs already on the interim three-column constraint upgrade cleanly. New databases get the four-column constraint directly from `initTaskSchema`.

- **Worktree paths.** When a task has more than one row sharing `RepositoryID`, the orchestrator (`buildRepoSpecs` in `internal/orchestrator/executor/executor_execute.go`) derives a deterministic `BranchSlug` per row from `CheckoutBranch` (falling back to `BaseBranch`). The first occurrence of each repo keeps the legacy flat layout `~/.kandev/tasks/<task>/<repo>/`; additional occurrences sit as **siblings** at `~/.kandev/tasks/<task>/<repo>-<branch-slug>/`. Nesting under the primary (`<task>/<repo>/<slug>/`) was rejected because it places the second worktree INSIDE the primary's working tree — agent file scans, git status, and IDE indexers would all see the second worktree as an unexpected subdirectory of the first.

- **Service.** `Service.AddBranchToTask` (in `internal/task/service/service_branches.go`) appends a new `task_repositories` row to a live task. It enforces the `(repo, branch)` uniqueness up front so callers get a clear "already attached" error rather than an opaque DB constraint failure.

- **MCP.** A new `add_branch_to_task_kandev` tool — backed by `ws.ActionMCPAddBranchToTask` — exposes the service method to agents. `create_task_kandev` accepts the same multi-row shape it did for multi-repo, just with duplicate `repository_id` entries when desired.

- **PR plumbing.** No schema change needed. `task_prs` already used `UNIQUE(task_id, repository_id, pr_number)` and carried `head_branch`, so the existing multi-repo loops in `internal/github/` and `internal/orchestrator/` transparently handle a second PR opened from a same-repo-different-branch row.

- **Frontend.** `TaskRepository.checkout_branch` was already on the http type. The Zustand `worktrees.items` map keys on `worktree.id` (not `repository_id`), so multi-branch worktrees already coexist in state. The chat-message repo chips render with `(repository_id, checkout_branch)` keys to keep distinct chips for same-repo-different-branch tasks. A full "+ Branch" UI affordance is deferred — agents drive the flow via the MCP tool today.

## Consequences

- **Worktree path collision.** Two branches that sanitize to the same slug (e.g. `feat/a` vs `feat-a`) would collide on disk. The service-layer dedup catches this in the common case (matching `CheckoutBranch` exactly), but distinct-but-equivalent slugs slip through. Acceptable because branch names are usually distinct enough in practice and the failure mode is a clean `git worktree add` error rather than data loss.

- **Path layout split.** Single-branch tasks keep the flat layout; multi-branch tasks nest one level deeper. We considered always nesting (uniform layout), but the back-compat surface for in-flight tasks made the conditional layout the safer call.

- **PR uniqueness.** `task_prs` did not need a `head_branch`-aware unique key because `pr_number` is already unique per `(task, repo)`. If we later allow two rows for the *same* PR number under different branches (unlikely), we'd extend the constraint then.

- **Concurrent agents.** Worktrees for different branches of the same repo share the underlying git directory. The worktree manager's per-repo lock already keys on `RepositoryPath`, so concurrent agent runs serialize on the same physical repo regardless of branch — a feature for safety, a small bottleneck for throughput. Revisit if this becomes a real contention point.

- **Frontend follow-up.** No grouped tabs (repo > branch) yet; today's flat per-(repo, branch) tab is the agreed first step. Add explicit "+ Branch" buttons and an `update_task_kandev` extension only if usage suggests the MCP tool is too coarse.

## Files touched

Backend:
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_branches.go` *(new)*
- `apps/backend/internal/worktree/worktree.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/errors.go`
- `apps/backend/internal/worktree/config.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/cmd/kandev/adapters.go`

Frontend:
- `apps/web/components/task/chat/messages/kandev/task-renderers.tsx`

Spec: `docs/specs/tasks/multi-branch/spec.md`
