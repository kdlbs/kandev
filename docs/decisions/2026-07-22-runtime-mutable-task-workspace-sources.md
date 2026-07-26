# ADR-2026-07-22-runtime-mutable-task-workspace-sources: Runtime-Mutable Task Workspace Sources

**Status:** accepted
**Date:** 2026-07-22
**Area:** backend, frontend, protocol

## Context

Task creation can select multiple repositories, but the UI restricts that layout to the worktree
executor. After launch, only the task-mode `add_branch_to_task_kandev` MCP path can append a source,
and it is both Git-only and worktree-only. Repository-less tasks may start in one arbitrary host
folder, but that single `workspace_path` is not a durable mixed-source model. Exposing the existing
MCP call in the Files panel would therefore report success only for a subset of tasks and could not
represent arbitrary folders.

## Decision

Task workspace sources are durable task-scoped attachments, while executor filesystems remain
materialized views of those attachments.

Existing repository identity and Git behavior stay in `task_repositories`. Arbitrary folders use a
separate `task_workspace_folders` relation rather than migrating every repository consumer to a new
polymorphic table. Services expose a combined source projection when callers need the complete
workspace.

A batch `AttachWorkspaceSources` service becomes the single mutation boundary for HTTP, UI, and
MCP callers. It validates the entire batch, persists attachments transactionally, materializes them
through an executor-capability interface, asks agentctl to adopt and rescan the effective workspace
root, and publishes events only after success. Failure rolls back newly created durable records and
owned filesystem entries without touching pre-existing sources. The legacy
`add_branch_to_task_kandev` tool becomes a compatibility wrapper over a one-item batch.

Executor adapters own placement and transport:

- local and worktree environments use live references to explicitly trusted host folders and may
  re-root an idle execution into a Kandev-owned task root;
- Docker, SSH, Sprites, and future remote runtimes materialize repositories in their own workspace
  but do not accept arbitrary host-folder attachments;
- agentctl owns workspace-root adoption, repository tracker reconciliation, and the file-tree
  refresh boundary common to every runtime.

Source attachment is rejected while a turn or tool call is active. An idle environment may be
restarted when its process working directory cannot be changed safely in place. Provider
credentials stay behind the existing backend/executor clone boundary and never enter persisted
URLs or agent-visible source metadata.

## Consequences

- The Files panel, MCP tools, and future automation use one source-attachment contract instead of
  reimplementing worktree-specific behavior.
- Existing repository, diff, PR, and branch consumers keep their current relational model.
- Arbitrary folders can coexist with Git sources without pretending to be repositories.
- Runtime backends must implement explicit source materialization and rollback capabilities;
  previously implicit single-repository assumptions become testable interfaces.
- Folder attachment remains a host-executor capability. Remote/container callers receive an
  explicit unsupported-source error instead of a partial copy with different persistence semantics.
- Idle-only mutation avoids changing a process working directory during an active command, at the
  cost of making the UI wait for the current turn to finish.

## Alternatives Considered

1. **Expose `add_branch_to_task_kandev` directly in the Files panel.** Rejected because it is
   worktree-only, Git-only, single-item, and cannot support the requested executor or folder scope.
2. **Replace `task_repositories` with one polymorphic source table.** Rejected because it forces a
   high-risk migration across mature Git, PR, diff, and session-worktree code for no user-visible
   benefit.
3. **Treat arbitrary folders as repositories.** Rejected because non-Git folders have no branch,
   diff, PR, or repository identity and would leak invalid assumptions into existing consumers.
4. **Copy host folders into remote runtimes as snapshots.** Rejected because a detached copy is not
   equivalent to adding the selected live folder and introduces size, retention, and stale-data
   expectations.
5. **Continuously synchronize host folders with remote runtimes.** Rejected because conflict
   resolution, deletion propagation, watching, and credential boundaries form a separate product
   and reliability problem.
