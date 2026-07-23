---
status: implemented
created: 2026-07-22
owner: kandev
---

# Attach Workspace Sources

## Why

Tasks often grow beyond the repositories selected at creation time. Users need to add another
repository or supporting folder without recreating the task, losing conversation context, or
manually moving files into the task workspace.

## What

- A repository-backed task exposes an **Add sources** action in the Files panel on desktop and
  mobile.
- Subject to the task executor's capabilities, one submission can add one or more mixed sources:
  - a repository already saved in the Kandev workspace;
  - an explicitly selected local Git repository;
  - a provider-backed or pasted remote Git repository URL;
  - an arbitrary local folder that is not required to be a Git repository.
- Repository rows include a base branch and may select an existing checkout branch, matching the
  repository and branch controls used by task creation.
- A successful submission makes every added source visible as a named top-level entry in the Files
  panel. Repository sources also appear in repository-aware Changes, branch, editor, and pull
  request surfaces; folder sources remain file-only.
- Duplicate repository/branch pairs, duplicate canonical folder paths, cross-workspace repository
  IDs, invalid remote URLs, and inaccessible local paths are rejected before the task changes.
- A multi-source submission is atomic from the user's perspective: either every source is attached
  and materialized in the current task environment, or none of the new attachments remain.
- Repository attachment works for every executor that can run the task. Arbitrary folders are
  available only to Local and Worktree tasks, where the selected host paths remain live. Container
  and remote pickers do not offer the folder source kind, and the backend rejects a forged folder
  request without changing the task.
- Kandev may re-root or restart an idle task environment when its executor cannot safely change the
  agent working directory in place. The action is unavailable while a turn or tool call is active,
  and the backend independently rejects that race with a conflict response.
- Existing conversations, task state, plan, sessions, and repository attachments remain intact.
- Agents receive a batch `add_workspace_sources_kandev` MCP tool that uses the same validation and
  materialization path. The existing `add_branch_to_task_kandev` tool remains as a compatibility
  wrapper for one repository/branch source.

Decision: [ADR-2026-07-22-runtime-mutable-task-workspace-sources](../../decisions/2026-07-22-runtime-mutable-task-workspace-sources.md).

## Data model

Repository attachments continue to use `task_repositories`; their current uniqueness contract on
`(task_id, repository_id, base_branch, checkout_branch)` is unchanged.

Arbitrary folder attachments use `task_workspace_folders`:

| Field | Contract |
| --- | --- |
| `id` | Stable attachment identity. |
| `task_id` | Owning task; cascade-deleted with the task. |
| `local_path` | Canonical absolute path selected on the Kandev host. |
| `display_name` | Sanitized, non-empty top-level workspace entry name. |
| `position` | Stable order among folder attachments. |
| `created_at`, `updated_at` | Audit timestamps. |

`(task_id, local_path)` and `(task_id, display_name)` are unique. The effective source projection
combines ordered `task_repositories` and `task_workspace_folders`; it does not replace repository
identity or make folders participate in Git operations.

## API surface

`POST /api/v1/tasks/:id/workspace-sources`

```json
{
  "sources": [
    {
      "kind": "repository",
      "repository_id": "optional-workspace-repository-id",
      "local_path": "optional-local-git-path",
      "remote_url": "optional-provider-or-pasted-url",
      "provider": "optional-provider",
      "provider_repo_id": "optional-provider-id",
      "provider_owner": "optional-provider-owner",
      "provider_name": "optional-provider-name",
      "base_branch": "main",
      "checkout_branch": "optional-existing-branch"
    },
    {
      "kind": "folder",
      "local_path": "/absolute/path/to/folder",
      "display_name": "optional-name"
    }
  ]
}
```

The response returns the persisted source projection, the effective task workspace path, and the
affected session IDs. Validation errors return `400`, ownership/not-found errors return `404`,
duplicates or an active turn return `409`, and materialization failures return `422` after rollback.

The backend publishes `task.updated` with both `repositories` and `workspace_folders`, then emits a
session-scoped workspace-sources update after agentctl has adopted the new workspace root. Clients
refresh the Files tree and repository trackers from those events rather than assuming the POST
response is the only writer.

`add_workspace_sources_kandev` accepts the same source union and defaults `task_id` to the current
task. `add_branch_to_task_kandev` translates its existing arguments to a one-item repository batch.

## Permissions

The action follows Kandev's trusted-local-user model and is scoped to the task's workspace. Saved
repository IDs must belong to that workspace. Explicit local repository and folder selections grant
access only to their canonical paths, not to parent directories, sibling paths, or filesystem
volumes. Remote credentials follow the existing provider-neutral repository contract and are never
persisted in source URLs or copied into agent-visible metadata.

## Failure modes

| Condition | Observable behavior |
| --- | --- |
| A turn or tool call is active | The UI disables the action when known; a racing request returns `409` without mutation. |
| Any source is invalid or duplicated | The full batch is rejected before persistence or materialization. |
| A host materializer fails | New filesystem entries and source records are rolled back; existing task contents remain. |
| A container/remote repository clone fails | Newly created remote entries are removed best-effort, durable attachments are rolled back, and the response identifies the failed source. |
| A container/remote task submits a folder source | The request returns `422` without persistence or filesystem changes. |
| Agentctl cannot rescan the new root | The attachment fails rather than reporting success with a stale Files tree. |
| A persisted local folder later disappears | The current live environment keeps its existing materialization; a new/reset environment surfaces the missing source and does not silently omit it. |
| The client disconnects during materialization | Rollback runs on a detached bounded context and the eventual task event reflects durable state. |

## Persistence guarantees

Repository and folder attachments survive backend restarts. Local/worktree environments continue to
resolve the exact canonical host path. New container or remote environments recreate repository
checkouts from durable repository attachments; they never persist folder attachments. Existing task
conversations and source records survive an environment restart even when runtime materialization
must be retried.

## Scenarios

- **GIVEN** a running worktree task with one repository and no active turn, **WHEN** the user adds a
  saved workspace repository and branch from the Files panel, **THEN** the new worktree appears as a
  top-level Files entry and in repository-aware Changes surfaces without recreating the task.
- **GIVEN** a repository-backed local task, **WHEN** the user adds a local Git repository and an
  arbitrary folder in one submission, **THEN** both live sources appear under one task workspace and
  the folder does not appear in Git-only controls.
- **GIVEN** a Docker, SSH, or Sprites task, **WHEN** the user opens Add sources, **THEN** local and
  remote repository choices are available and the arbitrary-folder choice is not offered.
- **GIVEN** a Docker, SSH, or Sprites task, **WHEN** a client submits a forged folder source,
  **THEN** the backend returns `422` and leaves the task and executor filesystem unchanged.
- **GIVEN** a mixed three-source submission whose second source cannot be cloned, **WHEN**
  materialization fails, **THEN** none of the three new attachments remain in the database, Files
  tree, or executor workspace.
- **GIVEN** an active agent turn, **WHEN** the user attempts to add sources, **THEN** no source is
  attached and the UI explains that the task must be idle first.
- **GIVEN** the same repository/branch or canonical folder is already attached, **WHEN** it is
  submitted again, **THEN** the request returns a conflict naming the duplicate and leaves the task
  unchanged.
- **GIVEN** a phone viewport on the Files tab, **WHEN** the user opens Add sources, adds two sources,
  and submits, **THEN** a touch-usable full-height picker completes the same operation without
  horizontal document overflow.
- **GIVEN** an agent calls `add_workspace_sources_kandev` for its current idle task, **WHEN** all
  inputs materialize, **THEN** the UI receives the same task and session updates as the human flow.

## Out of scope

- Removing or detaching sources after they have been attached.
- Promoting a repository-less task into a repository-backed task.
- Copying, mounting, or synchronizing arbitrary host folders into container or remote executors.
- Attaching sources while an agent turn or tool call is running.
- Reordering sources after attachment.
- Making the unimplemented remote Docker executor runnable; its source-materializer capability is
  required when that executor becomes available.

## Implementation plan

See [Attach Workspace Sources plan](../../plans/attach-workspace-sources/plan.md).
