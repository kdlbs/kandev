---
status: current
system: tasks
requirements:
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-001
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-002
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-003
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-004
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-005
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-006
---

# Agent Worktree Adoption System Design

## Purpose and boundaries

The tasks system owns task branch attachments (`task_repositories` plus the canonical inventory in
`task_environment_repos`). Adoption is one more way to create such an attachment, so the task
service owns the decision. agentctl only observes the filesystem and reports what it sees; it never
decides that a worktree belongs to a task.

This design reuses, and does not redefine:

- the add-branch commit path in [attach workspace sources](attach-workspace-sources.md) and
  [repository placement](workspace-repository-placement.md);
- the agentctl tracker graph and rescan contract in
  [git repository scope lifecycle](../../platform/system-design/git-repository-scope-lifecycle.md);
- per-repository Changes groups and commit identity in
  [multi-branch live commits](multi-branch-live-commits.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-AGENT-WORKTREE-ADOPTION-001` | [Adoption service](#adoption-service), [MCP contract](#mcp-contract) |
| `REQ-TASKS-AGENT-WORKTREE-ADOPTION-002` | [Worktree observer in agentctl](#worktree-observer-in-agentctl), [Detection flow](#detection-flow) |
| `REQ-TASKS-AGENT-WORKTREE-ADOPTION-003` | [Adoption service](#adoption-service), [Removal from disk](#removal-from-disk) |
| `REQ-TASKS-AGENT-WORKTREE-ADOPTION-004` | [Frontend](#frontend) |
| `REQ-TASKS-AGENT-WORKTREE-ADOPTION-005` | [Detach](#detach), [Persistence](#persistence), [Frontend](#frontend) |
| `REQ-TASKS-AGENT-WORKTREE-ADOPTION-006` | [Agent guidance](#agent-guidance) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| agentctl worktree observer (`internal/agentctl/server/process`) | Lists linked worktrees of each tracked repository and reports a filtered snapshot on the workspace stream. |
| Lifecycle manager (`internal/agent/runtime/lifecycle`) | Converts the stream message into a bus event that carries the session's task, environment and executor identity. |
| Task service adoption (`internal/task/service`) | Validates a candidate, writes or reuses the attachment, records detachments, and asks the runtime to rescan. |
| MCP tool (`internal/mcp/server`, `internal/mcp/handlers`) | Adds `worktree_path` to `add_branch_to_task_kandev` and forwards it to the adoption service. |
| Detach API (task WS action and HTTP route) | User-initiated removal of a non-primary root. |
| System prompt (`internal/sysprompt`, `config/prompts/kandev-context.md`) | Executor-gated guidance section. |
| Web Changes and Files panels (`apps/web/components/task`) | Render roots, the Files repository-root marker, and the Remove from task action. |

## Data and contracts

### Candidate identity

A candidate is identified by its canonical absolute path after `filepath.EvalSymlinks`, together
with the Git common directory it shares with an attached repository. Path containment is checked
against the canonical task workspace root, using the same containment helper as nested placement.
A candidate is eligible only when all of these hold:

- it is a linked worktree (`git worktree list --porcelain` entry, not `bare`, not `prunable`);
- its common directory equals the common directory of a tracked task repository;
- `HEAD` is a named branch (`branch refs/heads/...`);
- it is not already a tracker path, a submodule path, or the agent working directory checkout.

### Workspace stream message

`types.WorkspaceMessageTypeWorktreeInventory = "worktree_inventory"`, payload:

```text
WorktreeInventory {
  repository_name  string   // tracker that produced the list
  common_dir       string
  entries []{ path string, branch string, head_sha string }
  observed_at      timestamp
}
```

The message carries the full filtered list for one repository, so a consumer can diff it without
history. It is sent only when the list changes, and once after tracker start.

### Bus event

`events.WorkspaceWorktreesObserved` carries the stream payload plus `task_id`, `session_id`,
`task_environment_id` and `executor_type`, taken from the `AgentExecution`. It never uses
identifiers from the agentctl payload for authorization.

### MCP contract

`add_branch_to_task_kandev` gains optional `worktree_path`. With it:

- `checkout_branch` is optional and must equal the worktree's branch when supplied;
- `base_branch` sets the comparison base, defaulting to the repository default branch;
- the repository locator is inferred from the worktree's common directory when omitted;
- the response returns `worktree_path`, `task_workspace_path`, `adopted: true` and
  `already_attached` (true when the call reused a detected root).

Without `worktree_path`, `Service.AddBranchToTask` runs unchanged.

### Detach API

WS action `task.repository.detach` and `DELETE /api/v1/tasks/:id/repositories/:task_repository_id`
both call `Service.DetachTaskRepository(ctx, taskID, taskRepositoryID)`. The payload carries
`task_id`, so the WS dispatch backstop scopes it; the HTTP route authorizes through
`authorizeTaskID`.

## Adoption service

`Service.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID, Path, BaseBranch, CheckoutBranch, Source})`
with `Source` in `reported` or `detected`. Both entry points share it.

1. Take `lockWorkspaceSources(taskID)`, the lock used by `AddBranchToTask`.
2. Resolve the task environment and executor. Reject executors other than host Worktree and Local
   (`detected` returns silently; `reported` returns an actionable error).
3. Re-validate the candidate on disk in the backend host process with Git plumbing
   (`git rev-parse --absolute-git-dir`, `--git-common-dir`, `symbolic-ref`, `rev-parse HEAD`),
   never trusting the report's branch or path. Adoption is host-only, so the backend observes the
   same filesystem as the runtime; other executors are rejected before validation.
4. Look up an existing attachment by canonical worktree path in `task_environment_repos`:
   - found: return it (`already_attached`); for `reported`, also delete any detachment row;
   - not found and a detachment row exists: `detected` stops; `reported` deletes the row and
     continues.
5. Build a `models.TaskRepository` for the repository with the worktree branch as
   `CheckoutBranch` and a `BaseBranchSelection` (explicit or repository default). Reject a
   duplicate `(repository, base, checkout)` key with the existing typed error, unless the existing
   row points at this same path.
6. When the worktree is nested inside another attached checkout, install the managed nested
   workspace exclusion so the outer checkout's status and staging do not expose it. A failed
   exclusion removes any provisional change and aborts adoption.
7. Commit through `commitWorkspaceSourceBatch` with an adoption materializer that writes the
   inventory row in place: `worktree_path` = canonical path, `workspace_relative_path` relative to
   the task root, `worktree_branch` = branch, `worktree_branch_owner = worktree.BranchOwnerExternal`,
   `worktree_origin = "adopted"`. The materializer never runs `git worktree add`.
8. After commit, call `RescanWorkspaceForSession` for every live session of the environment with
   the current source roots plus the adopted path, so agentctl creates one tracker per adopted
   path. The tracker name is the canonical task-relative path, which the web client also uses as
   the root identity. Publish `task.updated` through the normal task path.

A rescan failure does not undo the attachment. The next observer snapshot triggers another rescan,
because step 4 finds the row but the tracker is still missing.

## Worktree observer in agentctl

The observer runs inside the existing git poll tick of each non-submodule tracker
(`gitPollTick`), so it shares the 30-second slow and the faster active intervals of
`workspace_poll_mode.go`, and it stops with the tracker.

- Cheap gate: stat `<common_dir>/worktrees` and compare its modification time and entry names with
  the last tick. Run `git worktree list --porcelain -z` only when they changed, or every tenth tick
  to catch branch switches inside a worktree that was still in detached `HEAD`.
- Use `subproc.RunGit*` with the `background` class.
- Filter by containment against the manager's workspace root and by the identity rules above.
- Publish `worktree_inventory` only when the filtered list differs from the last one sent.

The observer never writes Git state and never changes the tracker graph itself.

## Detection flow

```text
agent: git worktree add .claude/worktrees/fix-b -b fix-b
  -> agentctl observer (next git poll)  --worktree_inventory-->  lifecycle manager
  -> bus WorkspaceWorktreesObserved  -> task service subscriber
  -> for each new entry: AdoptWorktree(source=detected)
  -> commit attachment -> task.updated -> RescanWorkspaceForSession
  -> agentctl adds tracker -> git_status / file_change for the new repository_name -> web panels
```

The subscriber ignores events for sessions that are not the latest live session of the environment.
It only enqueues into a task-service worker, coalescing to the latest snapshot per environment;
Git inspection, database writes, and agentctl rescans run off the workspace-stream goroutine. The
worker has explicit start and stop lifecycle and never starts, resumes, or prompts an agent.

## Removal from disk

When a later snapshot no longer lists an adopted path and the path no longer exists, the
subscriber removes the attachment rows the same way `DetachTaskRepository` does, without writing a
detachment row. `task_prs` rows are untouched. A worktree that is temporarily `prunable` but still
on disk is kept.

## Detach

`DetachTaskRepository` takes the workspace-sources lock and rejects the primary row (lowest
position, or the row whose checkout contains the agent working directory). In one transaction it
inserts the `task_detached_worktrees` suppression row first, marks the `task_environment_repos` row
`status = "detached"` so task cleanup can still remove the physical checkout, and deletes the
`task_repositories` row. It removes the managed nested workspace exclusion, publishes
`task.updated`, then calls `RescanWorkspaceForSession` with the reduced source roots so agentctl
retires the tracker. It performs no filesystem or Git checkout operation, and does not touch
`task_prs`. A running agent turn does not block it.

Kandev-created secondary roots follow the same path. Their directory stays inside the task folder;
the detachment row stops the observer from adopting it again.

## Persistence

- New column `task_environment_repos.worktree_origin TEXT NOT NULL DEFAULT 'kandev'`, added only
  by an idempotent `ADD COLUMN` migration in `runMigrations()`.
- New table:

  ```sql
  CREATE TABLE IF NOT EXISTS task_detached_worktrees (
    task_id       TEXT NOT NULL,
    worktree_path TEXT NOT NULL,
    detached_at   TIMESTAMP NOT NULL,
    PRIMARY KEY (task_id, worktree_path),
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
  );
  ```

- Both schema changes need fresh-database and replay tests for SQLite, and environment-gated
  PostgreSQL behavior tests for the new repository methods.
- Task cleanup treats an adopted directory like other content of the task folder under the existing
  dirty-worktree preservation rules. Because its branch owner is `external`, branch cleanup never
  deletes the branch.

## Agent guidance

`sysprompt.KandevContextOptions` gains `IncludeWorktreeAdoption`. The launch path sets it for host
Worktree and Local executors. `kandev-context.md` gains a `{worktree_adoption_section}` placeholder,
rendered empty otherwise. The section text:

> If you create a Git worktree, create it inside the task workspace, then call
> add_branch_to_task_kandev with worktree_path so Kandev tracks it. Or call
> add_branch_to_task_kandev without worktree_path and Kandev creates the worktree.

## Frontend

- **Changes panel:** adopted roots arrive as ordinary per-repository groups. Their tracker name is
  the canonical task-relative path, exposed on the session worktree projection and used as the
  single root identity by both panels. `RepoGroupItem`
  (`changes-panel-repo-groups.tsx`) already renders stage, commit, push and PR actions per group;
  the label shows `<repository> · <branch>` for every non-primary root. The group header becomes a
  `ContextMenuTrigger` for the shared root menu; no overflow button is added.
- **Files panel:** `task-files-panel.tsx` receives the task's root list (path relative to the task
  workspace, branch, removable flag) from the task store. The file tree marks those folders with a
  repository-root icon and branch label and auto-reveals hidden ancestors of each root. For a root
  row, `file-context-menu.tsx` renders the shared root menu instead of `FileContextMenuItems`, so
  the generic Delete and Rename never apply to a root; the tree's delete and rename shortcuts check
  the same root list. Files are already read by path through agentctl, so a file under the adopted
  folder reads the worktree's copy.
- **Shared root menu:** new `root-context-menu-items.tsx`, driven by one root descriptor (task
  repository ID, absolute path, task-relative path, branch, linked PR, removable flag) and rendered
  by both surfaces. Items: Open in editor (reuses `file-tree-editor-menu.tsx`), Open terminal here
  (a user terminal with the root as working directory; agentctl `shell` already accepts `WorkDir`),
  Show in Files / Show in Changes (focus the other panel and scroll to the root), Copy full path,
  Copy relative path, Copy branch name, Push, Create PR or Open PR, and Remove from task. Disabled
  items carry a reason tooltip. Touch layouts open the same items through the existing Files touch
  trigger pattern (long-press), as a bottom sheet on phone; the context-menu key and Shift+F10 open
  it on the focused row and focus returns to the row on close.
- **Confirmation:** one shared dialog for both surfaces. On phone it is a bottom drawer with full
  width actions, following `/mobile-parity`.
- **i18n:** new `task:` keys for the action, dialog title, body and errors, in all seven catalogs.

## Failure and recovery

| Failure | Behavior |
| --- | --- |
| Report with invalid path | Typed error from step 3; no rows written. |
| Concurrent detect and report of the same path | Serialized by the workspace-sources lock; the second caller finds the row (step 4). |
| Rescan fails after commit | Attachment stays; the next snapshot retries the rescan. |
| Backend restart | Rows persist; launch-time inventory recreates trackers. Observer's first snapshot after start finds existing rows, so nothing is re-adopted. |
| agentctl down | No detection; a reported path still validates and attaches on the host, while tracker creation waits for agentctl to return. |

## Security

- Containment uses canonical paths after symlink resolution, so a symlink inside the task folder
  that points elsewhere cannot be adopted.
- Task and session identity come from the `AgentExecution`, never from the stream payload or the
  MCP arguments, matching the in-session MCP scoping rule in `apps/backend/AGENTS.md`.
- Detach authorizes the task before any read of its repositories.

## Observability

- Counter `task_worktree_adoption_total` labelled by `source` (`reported`, `detected`) and `result`
  (`adopted`, `already_attached`, `rejected`, `suppressed`). Both label sets are closed; no task,
  path or branch value is a label.
- Counter `task_worktree_detach_total` labelled by `reason` (`user`, `removed_from_disk`).
- Structured logs with task, environment and canonical path for every adoption, rejection reason
  and detach.
