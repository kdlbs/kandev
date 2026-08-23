---
status: current
system: tasks
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
created: 2026-07-22
owners:
  - kandev
---
# Attach Workspace Sources System Design

## Purpose and boundaries

This design record preserves the technical source for the capability mapped to REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001 while the task system completes its migration.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001 | Migrated legacy design detail below |

## Migrated design source

## Why

Tasks often grow beyond the repositories selected at creation time. Users need to add another
repository or supporting folder without recreating the task, losing conversation context, or
manually moving files into the task workspace.

## What

- A repository-backed task exposes one **Workspace actions** menu in the Files panel on desktop and
  mobile. The menu contains **Add Repositories to workspace** and **Open workspace folder** rather
  than separate toolbar controls.
- **Add sources** uses the same repository-selection language as task creation without adding a
  second mode switch. One **Add repository** menu offers **Workspace repository**, **Local Git
  repository**, and **Remote repository**; **Add folder** remains a separate action when supported.
- Subject to the task executor's capabilities, one submission can add one or more mixed sources.
  **Workspace repository** combines saved workspace repositories and discovered local Git
  repositories in the shared repository selector, including its refresh and create-repository
  actions. **Local Git repository** accepts an existing checkout, **Remote repository** reuses the
  provider-backed and pasted-URL selector from task creation, and **Add folder** offers an arbitrary
  local folder when supported.
- Adding another repository or folder appends a row without hiding or discarding configured rows,
  so one submission can mix workspace, local, remote, and folder sources.
- Repository rows choose a base branch with the shared task-creation controls. The add-sources UI
  does not expose a second checkout-branch field.
- A successful submission makes every added source visible as a named top-level entry in the Files
  panel. Repository sources also appear in repository-aware Changes, branch, editor, and pull
  request surfaces; folder sources remain file-only.
- Contradictory repository/branch pairs, contradictory canonical folder paths, cross-workspace repository
  IDs, invalid remote URLs, and inaccessible local paths are rejected before the task changes.
- A source file rename may stay within its canonical workspace/source root; a cross-root move or
  rename is rejected before either source is mutated.
- A multi-source submission is atomic from the user's perspective: either every source is attached
  and materialized in the current task environment, or none of the new attachments remain. When an
  attachment repointed a pre-existing Kandev-owned entry, a failed submission restores that entry to
  the target it had before the attachment rather than deleting it.
- Repository attachment works for every executor that can run the task. Arbitrary folders are
  available only to Local and Worktree tasks, where the selected host paths remain live. Container
  and remote pickers do not offer the folder source kind, and the backend rejects a forged folder
  request without changing the task.
- Kandev may re-root or restart an idle task environment when its executor cannot safely change the
  agent working directory in place. The action is unavailable while a turn or tool call is active,
  and the backend independently rejects that race with a conflict response.
- Before submission, the desktop dialog and phone drawer explain the executor-specific effect on
  the agent working directory, provider session context, terminal and workspace processes, existing
  files and Git changes, and atomic rollback. They state that **Cancel** leaves the task unchanged.
- Providers that honor a changed `session/load` working directory keep their native ACP session
  after the re-root. Providers that do not are started in a fresh ACP session at the promoted task
  root, and Kandev rehydrates the recorded conversation context with the next prompt.
- Worktree and Local/Local PC rebinds stop terminal shells, the task editor server, dev servers, and
  other agentctl-managed workspace processes; users must reopen or restart them. Docker, SSH, and
  Sprites attach repository siblings through the live workspace and rescan without restarting the
  agent or those processes.
- When **Add Repositories to workspace** is unavailable, the combined Files action remains
  reachable so **Open workspace folder** still works. The repository action is disabled and shows
  the reason in touch-visible text rather than relying on a tooltip.
- Existing conversations, task state, plan, sessions, and repository attachments remain intact.
- Agents receive a batch `add_workspace_sources_kandev` MCP tool that uses the same validation and
  materialization path.
- The existing worktree-only `add_branch_to_task_kandev` tool remains a live compatibility path
  for one repository/branch source. It may run during the invoking agent turn, creates the new
  worktree as a sibling under the Kandev-owned task root, promotes the persisted task workspace
  path to that root, and refreshes Files and repository trackers without restarting the agent or
  agentctl-managed workspace processes.
- A legacy add-branch call does not nest the new worktree inside the current repository and does
  not change the running agent or terminal working directory. Its MCP result returns the exact new
  worktree path, the promoted task workspace path, and that the agent CWD did not change.

Decisions: [ADR-2026-07-22-runtime-mutable-task-workspace-sources](../../../decisions/2026-07-22-runtime-mutable-task-workspace-sources.md)
[ADR-2026-07-23-workspace-source-root-move-boundary](../../../decisions/2026-07-23-workspace-source-root-move-boundary.md),
and [ADR-2026-07-27-legacy-add-branch-live-rescan](../../../decisions/2026-07-27-legacy-add-branch-live-rescan.md).

## Data model

Repository attachments continue to use `task_repositories`; their current uniqueness contract on
`(task_id, repository_id, base_branch, checkout_branch)` is unchanged.

Arbitrary folder attachments use `task_workspace_folders`:

| Field                      | Contract                                             |
| -------------------------- | ---------------------------------------------------- |
| `id`                       | Stable attachment identity.                          |
| `task_id`                  | Owning task; cascade-deleted with the task.          |
| `local_path`               | Canonical absolute path selected on the Kandev host. |
| `display_name`             | Sanitized, non-empty top-level workspace entry name. |
| `position`                 | Stable order among folder attachments.               |
| `created_at`, `updated_at` | Audit timestamps.                                    |

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
contradictory duplicates or an active turn return `409`, and materialization failures return `422` after rollback. Exact normalized retries succeed as no-ops.

The backend publishes `task.updated` with both `repositories` and `workspace_folders`, then emits a
session-scoped workspace-sources update after agentctl has adopted the new workspace root. Clients
refresh the Files tree and repository trackers from those events rather than assuming the POST
response is the only writer.

Full and summary task-session responses expose `workspace_path` as the effective task workspace
root. In a multi-source task this is the parent that contains every repository and folder entry;
`worktree_path` remains the flattened primary-repository path for backward compatibility. Live
workspace-source and sibling-materialization events update `workspace_path` without replacing the
primary `worktree_path`, and a later session refresh returns the same effective root.

Chat file links resolve against `workspace_path`, so an absolute path under any attached source is
converted to its task-root-relative Files path before it is opened. Clients may fall back to
`worktree_path` for legacy session payloads that do not yet include `workspace_path`. Absolute paths
outside the effective workspace remain non-actionable and are never rewritten into a workspace
file request.

`add_workspace_sources_kandev` accepts the same source union and defaults `task_id` to the current
task. The caller may target only itself or a same-workspace direct child. The task MCP server injects
the bound caller task/session provenance and the backend verifies it before mutation; provenance is
not an agent-supplied tool argument. Exact normalized repository and folder retries are idempotent,
while contradictory source identities remain conflicts. See ADR
[`2026-08-19-parent-authorized-child-workspace-sources`](../../../decisions/2026-08-19-parent-authorized-child-workspace-sources.md).

`add_branch_to_task_kandev` preserves its existing request arguments and returns:

```json
{
  "id": "task-repository-id",
  "task_id": "task-id",
  "repository_id": "repository-id",
  "base_branch": "main",
  "checkout_branch": "feature/example",
  "position": 1,
  "worktree_path": "/absolute/task-root/repository-feature-example",
  "task_workspace_path": "/absolute/task-root",
  "agent_cwd_changed": false
}
```

`worktree_path` and `task_workspace_path` are omitted when a pre-launch attachment succeeds but
materialization is intentionally deferred until the task launches. `agent_cwd_changed` is always
false. A live call returns the materialized sibling path so the invoking agent can address it
without inferring a directory name.

## Permissions

The action follows Kandev's trusted-local-user model and is scoped to the task's workspace. Saved
repository IDs must belong to that workspace. Explicit local repository and folder selections grant
access only to their canonical paths, not to parent directories, sibling paths, or filesystem
volumes. Remote credentials follow the existing provider-neutral repository contract and are never
persisted in source URLs or copied into agent-visible metadata.

## Failure modes

| Condition                                                      | Observable behavior                                                                                                                                 |
| -------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| A turn or tool call is active for batch attachment             | The UI disables the action when known; a racing batch request returns `409` without mutation.                                                       |
| The invoking agent calls legacy add-branch during its turn     | The worktree is created and trackers refresh without stopping the active agent, terminals, or workspace processes.                                  |
| Any source is invalid or contradicts an existing source identity | The full batch is rejected before persistence or materialization. Exact normalized retries are no-ops.                                             |
| A host materializer fails                                      | New filesystem entries and source records are rolled back; existing task contents remain.                                                           |
| A container/remote repository clone fails                      | Newly created remote entries are removed best-effort, durable attachments are rolled back, and the response identifies the failed source.           |
| A container/remote task submits a folder source                | The request returns `422` without persistence or filesystem changes.                                                                                |
| Agentctl cannot rescan the new root                            | The attachment fails rather than reporting success with a stale Files tree.                                                                         |
| A chat message links to an absolute path outside the task root | The link is not sent to the workspace file API; normal external/unsafe-path handling remains in effect.                                              |
| An idle agent must restart to adopt the promoted root          | The intentional stop is not shown as a prior agent failure; the replacement agent uses the promoted task root.                                      |
| A requested file move or rename crosses canonical source roots | The request is rejected before either source is mutated.                                                                                            |
| A persisted local folder later disappears                      | The current live environment keeps its existing materialization; a new/reset environment surfaces the missing source and does not silently omit it. |
| The client disconnects during materialization                  | Rollback runs on a detached bounded context and the eventual task event reflects durable state.                                                     |
| A Kandev-owned task-root entry already points at a different directory than the current durable spec (e.g. a stale entry left by an earlier launch) | Because the task root is Kandev-owned and the entry is a directory link, reconciliation repoints it to the current spec target and the launch/resume proceeds; it does not fail closed forever. |
| A Kandev-owned task-root entry's ownership marker names a different task than the one reconciling | Reconciliation does not repoint the entry; it fails closed with a marker-conflict error and leaves the other task's entry pointing at its existing target. |
| A multi-source attachment that repointed a pre-existing owned entry later fails during materialization | Rollback restores each repointed entry to the target it had before the attachment and deletes only entries this attachment created; no pre-existing entry is left deleted. |
| A safe replacement would overwrite a non-owned or out-of-root entry, or its recreate fails after removal | A non-owned or changed entry, or a traversal path, is rejected before any removal, and a failed recreate restores the entry to its prior target rather than leaving it missing. |

## Persistence guarantees

Repository and folder attachments survive backend restarts. Local/worktree environments continue to
resolve the exact canonical host path. New container or remote environments recreate repository
checkouts from durable repository attachments; they never persist folder attachments. Existing task
conversations and source records survive an environment restart even when runtime materialization
must be retried.

Each task that materializes a Kandev-owned task root under the tasks base directory derives its
task-root directory name from task identity. The name is collision-resistant, not injective: two
distinct tasks — including two tasks whose titles sanitize to the same slug, or a local task and a
worktree task sharing a title — are overwhelmingly unlikely to resolve to the same task root, and any
residual collision is caught by the fail-closed ownership marker, which verifies task identity before
any Kandev-owned entry under a shared root is repointed. The task-root name is computed once and
persisted; every relaunch and resume of that task reuses the persisted name.

## Scenarios

- **GIVEN** a repository-backed task with the Files tree loaded, **WHEN** the user opens the
  workspace-actions control, **THEN** one menu exposes **Add Repositories to workspace** and **Open
  workspace folder**.
- **GIVEN** a running worktree task with one repository and no active turn, **WHEN** the user opens
  **Add sources**, chooses **Workspace repository**, and adds a saved or discovered repository and
  branch with the shared task-create selector, **THEN** the new worktree appears as a top-level
  Files entry and in repository-aware Changes surfaces without recreating the task.
- **GIVEN** an idle task whose runtime resumes its agent or emits a late session-resumed status while
  adopting an updated workspace root, **WHEN** the user attaches another source, **THEN** lifecycle
  boot/status messages do not create a phantom turn and the subsequent attachment succeeds.
- **GIVEN** an idle single-repository task whose agent started in the repository directory, **WHEN**
  another source promotes the workspace to the task root, **THEN** the next agent prompt runs from
  the task root without a previous-agent-error banner; compatible providers retain their native
  session and incompatible providers receive a fresh session with recorded conversation context.
- **GIVEN** an idle Worktree or Local task, **WHEN** the user opens **Add sources**, **THEN** a
  visible consequence summary explains that the CWD moves to the task root, the agent and
  agentctl-managed workspace processes restart or stop, recorded task context remains, and
  provider-private context that Kandev did not record may not carry over.
- **GIVEN** an idle Docker, SSH, or Sprites task, **WHEN** the user opens **Add sources**, **THEN**
  the consequence summary says repositories are attached and rescanned under the current remote
  workspace without restarting the agent or changing its CWD.
- **GIVEN** configured source rows in either add-sources surface, **WHEN** the user chooses
  **Cancel** or closes the surface before submission, **THEN** no attachment request is sent and
  the task workspace remains unchanged.
- **GIVEN** an Add sources batch with a local row already configured, **WHEN** the user chooses
  **Remote repository** from **Add repository**, **THEN** both rows remain visible in the batch and
  submit atomically.
- **GIVEN** a repository-backed local task, **WHEN** the user adds a local Git repository and an
  arbitrary folder in one submission, **THEN** both live sources appear under one task workspace and
  the folder does not appear in Git-only controls.
- **GIVEN** a Docker, SSH, or Sprites task, **WHEN** the user opens **Add sources**, **THEN** the
  workspace, local-Git, and remote repository choices are available from **Add repository**, and
  the local-folder affordance is not offered.
- **GIVEN** a Docker, SSH, or Sprites task, **WHEN** a client submits a forged folder source,
  **THEN** the backend returns `422` and leaves the task and executor filesystem unchanged.
- **GIVEN** a mixed three-source submission whose second source cannot be cloned, **WHEN**
  materialization fails, **THEN** none of the three new attachments remain in the database, Files
  tree, or executor workspace.
- **GIVEN** an active agent turn, **WHEN** the user attempts to add sources, **THEN** no source is
  attached, the **Add Repositories to workspace** menu item explains that the task must be idle
  first, and **Open workspace folder** remains available.
- **GIVEN** an active worktree-executor agent whose CWD is the initial repository, **WHEN** it calls
  `add_branch_to_task_kandev`, **THEN** Kandev creates the new repository/branch worktree as a
  sibling under the task root, promotes the persisted workspace path, refreshes Files and
  repository trackers, and does not restart the agent or change its CWD.
- **GIVEN** a live legacy add-branch materialization, **WHEN** the MCP result returns, **THEN** it
  includes the absolute new `worktree_path`, the promoted `task_workspace_path`, and
  `agent_cwd_changed: false`.
- **GIVEN** the original repository has no pending changes, **WHEN** a legacy add-branch call creates
  a sibling worktree, **THEN** Git status in the original repository does not report the sibling as
  an untracked or changed path.
- **GIVEN** a task whose workspace contains multiple repositories, **WHEN** an agent message links
  to an absolute file path in either the primary or an attached repository, **THEN** clicking the
  link opens the exact file through the task-root-relative Files API path without a file-not-found
  notification.
- **GIVEN** that multi-repository task is reloaded after workspace promotion, **WHEN** the user
  clicks the same chat file link, **THEN** the session-restored `workspace_path` resolves the same
  file and `worktree_path` still identifies the primary repository.
- **GIVEN** a legacy single-repository session payload without `workspace_path`, **WHEN** a chat
  file link is inside `worktree_path`, **THEN** the link continues to open as a repository-relative
  path.
- **GIVEN** a live legacy add-branch materialization fails, **WHEN** the MCP call returns an error,
  **THEN** the new `task_repositories` row and any newly created repository entity are rolled back
  while the agent and existing processes continue running.
- **GIVEN** a source identity conflicts with an already attached repository/branch or canonical folder,
  **WHEN** it is submitted, **THEN** the request returns a conflict and leaves the task unchanged.
- **GIVEN** the same normalized repository/branch or canonical folder is already attached, **WHEN** it is
  submitted again, **THEN** the request succeeds without changing the task.
- **GIVEN** a phone viewport on the Files tab, **WHEN** the user opens the 44px workspace-actions
  control, **THEN** an inset bottom-sheet menu exposes both actions with touch-sized rows.
- **GIVEN** that phone action menu, **WHEN** the user selects **Add Repositories to workspace**,
  chooses repository kinds from the touch-sized **Add repository** menu, adds two sources, and
  submits, **THEN** a touch-usable full-height picker completes the same operation without
  horizontal document overflow and returns focus to the workspace-actions control.
- **GIVEN** an agent calls `add_workspace_sources_kandev` for its current idle task or an idle
  same-workspace direct child, **WHEN** all
  inputs materialize, **THEN** the UI receives the same task and session updates as the human flow.
- **GIVEN** two distinct tasks whose titles sanitize to the same task-root slug, **WHEN** each task
  materializes a Kandev-owned task root, **THEN** their collision-resistant suffixes normally produce
  different task-root directory names; if a residual suffix collision occurs, the ownership marker
  rejects cross-task repointing with a marker-conflict error rather than redirecting the other task's
  entries.
- **GIVEN** a local task whose persisted task root already contains a Kandev-owned directory-link
  entry for a repository that points at a different directory than the current durable spec target,
  **WHEN** the task launches or resumes, **THEN** reconciliation repoints the owned entry to the
  current spec target and the launch/resume proceeds instead of failing with an owned-link target
  mismatch on every attempt.
- **GIVEN** a Kandev-owned task-root entry that is not a directory link (a real file or directory a
  reconcile did not create), **WHEN** the task launches or resumes, **THEN** reconciliation does not
  delete or overwrite it and the launch surfaces an error identifying the conflicting entry.
- **GIVEN** two persisted legacy tasks whose environments share one Kandev-owned task root, **WHEN**
  the second task launches or resumes and its ownership marker does not match the root's marker,
  **THEN** reconciliation fails closed with a marker-conflict error and does not redirect the first
  task's live entry.
- **GIVEN** a multi-source attachment that repointed a pre-existing Kandev-owned entry, **WHEN** a
  later source in the same submission fails to materialize, **THEN** rollback restores the repointed
  entry to its prior target and no pre-existing entry is left deleted.
- **GIVEN** a Kandev-owned entry whose recreate could fail after the old link is removed, **WHEN**
  reconciliation replaces it, **THEN** a traversal or out-of-root name is rejected before any removal
  and a failed recreate leaves the original entry intact rather than missing.

## Out of scope

- Removing or detaching sources after they have been attached.
- Promoting a repository-less task into a repository-backed task.
- Copying, mounting, or synchronizing arbitrary host folders into container or remote executors.
- Running batch workspace-source attachment while an agent turn or tool call is active; the legacy
  worktree-only `add_branch_to_task_kandev` compatibility path is the explicit exception.
- Changing the running agent or terminal CWD during a legacy add-branch call.
- Nesting a new Git repository or worktree inside the current repository.
- Expanding or bypassing a provider-owned filesystem sandbox when it excludes the returned sibling
  path.
- Reordering sources after attachment.
- Sharing task-creation state, its mode switch, or its **None**/scratch semantics with Add sources;
  only the repository picker leaves are shared.
- Making the unimplemented remote Docker executor runnable; its source-materializer capability is
  required when that executor becomes available.

## Implementation plan

See [Attach Workspace Sources plan](../../../plans/attach-workspace-sources/plan.md) and the
[live add-branch compatibility repair plan](../../../plans/restore-live-add-branch/plan.md), plus the
[multi-repository chat file-link repair plan](../../../plans/multi-repo-chat-file-links/plan.md) and the
[owned link target mismatch repair plan](../../../plans/owned-link-target-mismatch-repair/plan.md).