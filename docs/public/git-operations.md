---
title: "Git Operations"
description: "Use Kandev worktrees, commits, remote operations, change requests, and cleanup safely."
---

# Git Operations

Kandev runs Git commands in the selected repository workspace for a task session. The Worktree executor gives a session a dedicated host worktree; Local uses the configured shared checkout, while container and remote executors use runtime-specific workspace behavior described in [Executors](executors.md). The browser sends a WebSocket request to the backend, which forwards it to `agentctl` in that executor. This keeps the command's filesystem, Git configuration, network access, SSH agent, and provider CLI in the same environment as the agent.

Use the task's **Changes** panel to inspect, stage, discard, commit, push, reset, or rename a branch. The task toolbar and command panel also expose **Commit Changes**, **Push**, **Pull**, provider-appropriate pull-request or merge-request creation, **Rebase**, and **Merge** when a Git-capable session is selected.

## Prerequisites and trust boundary

The repository must be a valid Git checkout in the executor workspace and the session's `agentctl` must be reachable. Remote commands use the remote named `origin`; configure its URL and credentials before relying on Pull, Push, Rebase, Merge, or change-request creation.

These UI operations enter through Kandev's `/ws` endpoint, which currently has no backend authentication. Anyone who can reach an unprotected backend can invoke destructive Git actions with the executor's permissions. Keep Kandev on loopback or behind an authenticated, origin-protected reverse proxy; see [WebSocket API](websocket-api.md).

Credentials are resolved where `agentctl` runs. A host SSH agent, credential helper, `gh` login, or `az` login is not automatically available inside every Docker, SSH, or remote executor. Give the executor only the repository access it needs and test with a disposable branch. See [Executors](executors.md) for executor-specific credential handling.

When the Worktree executor is selected, its filesystem separation is not a security sandbox. Host worktrees share the repository's Git object database and local refs, and agents can run arbitrary repository commands allowed by their executor.

## Managed worktree and branch lifecycle

This section describes Kandev's Worktree executor and other executor paths that create managed Git workspaces. Local, Docker, remote Docker, SSH, and Sprites have executor-specific checkout, mount, and cleanup behavior; see [Executors](executors.md) before assuming these host paths or isolation properties apply.

By default, managed worktrees live under the configured task-data directory, commonly:

```text
~/.kandev/tasks/{task-directory}/{repository-name}
~/.kandev/tasks/{task-directory}/{repository-name}-{branch-slug}
```

Additional branches are siblings of the primary repository worktree, not directories nested inside it. Multi-repository tasks have one worktree per repository. Kandev reuses a valid session/repository worktree; if its directory is missing, it attempts to recreate it from the recorded local or remote branch.

For a new task branch, the repository default template is:

```text
feature/{title}-{suffix}
```

`{title}` is an ASCII-safe, lower-case task-title slug and `{suffix}` is a short collision-avoidance value. Repository settings can change the template. When `pull_before_worktree` is omitted it defaults to `true`: Kandev best-effort fetches the base branch before creating the worktree. The public configuration defaults both fetch and fast-forward pull timeouts to 60 seconds. An authentication, network, or timeout failure can fall back to an available local or remote-tracking ref with a visible warning; a base branch that cannot be resolved, including its configured fallback, stops creation.

When a task opens an existing branch or GitHub PR, Kandev fetches that branch; for a numbered GitHub PR it can fetch `refs/pull/NUMBER/head`, including fork PRs. If the intended branch is already checked out in another worktree, the new worktree uses a suffixed local branch and tracks the original `origin` branch when available. If remote fetch fails but the local branch exists, Kandev can continue with that possibly stale branch and reports the fallback.

After creation, Kandev copies any repository-configured files and runs its setup script. Setup-script failure is non-fatal: the worktree remains and the session surfaces a warning. Cleanup scripts run before worktree removal, but their failure also does not prevent removal.

## Everyday operations

All operations below run in the selected repository workspace.

| UI operation | Effective Git behavior | Important consequence |
|--------------|------------------------|-----------------------|
| Pull | `git pull origin BRANCH`, optionally with `--rebase`. | Uses the current branch when any upstream exists. With no upstream it falls back to `origin/main`, then `origin/master`, then the current branch. It does not parse an upstream that points to a differently named remote branch. |
| Push | `git push origin CURRENT_BRANCH`; adds `--set-upstream` when requested or no upstream exists. | Force Push uses `--force-with-lease`, not unconditional `--force`. It still rewrites remote history when the lease is valid. |
| Rebase | Fetches `origin BASE`, then rebases onto `origin/BASE`. | Rewrites local commits. If conflict files are detected from Git output, Kandev attempts `git rebase --abort` automatically and returns the file list. |
| Merge | Fetches `origin BASE`, then merges `origin/BASE`. | Conflicts are deliberately left in the worktree. Resolve and commit them, or use Abort Merge. |
| Abort | Runs `git merge --abort` or `git rebase --abort`. | Fails when that operation is not in progress or the repository cannot be restored. |
| Stage | With paths, `git add -- PATHS`; with an empty path list, `git add -A`. | Empty means all changes, including deletions. |
| Unstage | With paths, `git reset HEAD -- PATHS`; with an empty path list, `git reset HEAD`. | Keeps working-tree content. |
| Commit | Optionally runs `git add -A`, then `git commit -m MESSAGE`; Amend adds `--amend`. | The normal UI defaults to staging all when it invokes this helper. Amend rewrites `HEAD`. |
| Discard | Restores tracked paths from `HEAD`; added and untracked files are unstaged and deleted. | Removes both staged and unstaged work. Explicit paths are required, but deletion is not recoverable through Kandev. |
| Edit branch | `git branch -m NEW_NAME` for the current local branch. | Does not rename/delete the old remote branch or automatically repair every external reference. Push the new branch explicitly. |

Only one Git operation can run at a time for a given repository operator. A second concurrent request is rejected as “another git operation is already in progress.” Different repositories in a multi-repository workspace have separate operators.

Most Git command failures are normal responses with `success:false`, `error`, and sometimes `conflict_files`; they are not WebSocket transport errors. Read the result body even when the request itself completed. The web client waits 60 seconds for an ordinary Git operation.

### Multi-repository tasks

In a multi-repository task, every wire request must identify one repository with its `repo` subpath. The workspace root is not itself a Git repository, so omitting `repo` normally fails. The UI handles this for you:

- Per-file Stage, Unstage, and Discard are routed to the file's repository.
- Stage All and Unstage All fan out to repositories that have files.
- Commit fans out only to repositories with staged changes.
- Push fans out only to repositories that are ahead.
- Pull, Rebase, Merge, and Abort fan out to all listed repositories.
- The multi-repository toolbar lets you select an individual repository for Commit, Push, Create PR, Pull, Rebase, Merge, or Force Push.

A fan-out can partially succeed. The UI continues after a failure and reports per-repository outcomes; inspect each repository before retrying a history-changing action.

## Commit history and reset behavior

The Changes panel's session history is calculated relative to the session's recorded base commit or current merge base, so it focuses on commits created on the task branch. Kandev refreshes status and emits session Git updates after mutations, but the underlying Git repository remains authoritative.

Two similarly named actions have very different semantics:

- **Revert latest commit** (`worktree.revert_commit`) is not `git revert`. It accepts only the exact current `HEAD` SHA and runs `git reset --soft HEAD~1`, moving the branch back one commit while leaving its changes staged. It creates no inverse commit.
- **Reset to commit** moves `HEAD` to an existing 4–40 character hexadecimal commit SHA. `soft` leaves changes staged, `mixed` leaves them unstaged, and `hard` discards tracked working-tree and index changes. The current UI offers Soft and Hard and requires the short SHA before Hard; the wire handler also accepts Mixed and defaults a missing mode to `mixed`.

Do not reset or amend commits already consumed by other users unless you intend to rewrite and force-push the branch. Kandev hides some revert/reset actions for commits it knows are pushed, but that UI guard is not a repository policy or API authorization boundary.

## Create a pull request or merge request

**Create PR** first runs:

```bash
git push --set-upstream origin HEAD
```

It then selects a provider from the `origin` hostname:

| Provider | Required runtime tools | Creation behavior |
|----------|------------------------|-------------------|
| GitHub | Authenticated `gh` CLI | `gh pr create` with title, body, current head, optional base, and optional `--draft`. |
| GitLab | Authenticated `glab` CLI or the active workspace's matching GitLab PAT | Reuses an existing open MR for the same source/target or creates one with title, description, current source, explicit or project-default target, and optional draft state. The configured GitLab origin must match the HTTPS or SSH `origin`. |
| Azure Repos | Authenticated `az` CLI plus the `azure-devops` extension | `az repos pr create` with parsed organization, project, repository, source, optional target, and optional draft. |

Other remote providers are rejected by this action. A normal Git push can still work with another provider. A returned GitHub PR or GitLab MR is asynchronously associated with the originating task repository. Azure creation returns a URL but does not create a provider review association.

Title is required. Body and base branch may be empty. GitHub and Azure delegate an empty base to their provider CLI; GitLab resolves the project default branch before creation. The web UI defaults new change requests to draft and waits up to 120 seconds. Provider credentials and remote push permission must exist in the executor, and Git hooks or branch policies can still reject the push or change request.

For GitLab, Kandev preserves a self-managed connection's scheme and never retries against another host. It prefers `glab` when installed and uses the workspace-injected `GITLAB_TOKEN` REST path when the CLI is absent or its create command fails. The successful WebSocket payload keeps the compatibility field name `pr_url` and adds `provider:"gitlab"`. Association runs after that response; use **Link GitLab merge request** if the MR was created but the link is missing.

## WebSocket operation reference

These are the registered Kandev WebSocket actions. Every payload requires `session_id`; `repo` is optional only for a single-repository workspace.

| Action | Additional payload |
|--------|--------------------|
| `worktree.pull` | `rebase` boolean |
| `worktree.push` | `force` and `set_upstream` booleans |
| `worktree.rebase` | required `base_branch` |
| `worktree.merge` | required `base_branch` |
| `worktree.abort` | `operation`: exactly `merge` or `rebase` |
| `worktree.commit` | required non-empty `message`; `stage_all`; `amend` |
| `worktree.stage` | `paths` list; empty means all |
| `worktree.unstage` | `paths` list; empty means all |
| `worktree.discard` | required non-empty `paths` list |
| `worktree.create_pr` | required `title`; `body`; `base_branch`; `draft`; response can include `pr_url` and `provider` (`github`, `gitlab`, or `azure_repos`) |
| `worktree.revert_commit` | required `commit_sha`, which must be exact `HEAD` |
| `worktree.rename_branch` | required `new_name` |
| `worktree.reset` | required `commit_sha`; `mode` is `soft`, `mixed`, or `hard` |

Example request and normal operation result:

```json
{
  "id": "pull-1",
  "type": "request",
  "action": "worktree.pull",
  "payload": {
    "session_id": "SESSION_ID",
    "rebase": false,
    "repo": "kandev"
  }
}
```

```json
{
  "id": "pull-1",
  "type": "response",
  "action": "worktree.pull",
  "payload": {
    "success": true,
    "operation": "pull",
    "output": "Already up to date."
  },
  "timestamp": "2026-07-16T10:00:00Z"
}
```

Read-only Git actions used by the Changes panel include `session.commit_diff`, `session.git.commits`, `session.cumulative_diff`, and `session.git.snapshots`. See [WebSocket API](websocket-api.md) for transport and subscription behavior.

`agentctl` also implements `/api/v1/git/*` HTTP routes inside the execution runtime. Those routes are an internal backend-to-runtime control surface, not the public Kandev backend API. External clients should not discover or expose executor-local agentctl ports; use the registered Kandev WebSocket actions.

## Cleanup and data loss

Worktree cleanup runs the repository cleanup script, forcibly removes the Git worktree directory, and may remove the local branch:

- Normal task deletion cleans all owned task worktrees and runs `git branch -D` for their local branches. Remote branches are not deleted, but uncommitted and unpushed-only work can be lost.
- **Reset Environment** is allowed only when no task session is `STARTING` or `RUNNING`. It can optionally push first; a failed requested push aborts the reset. Teardown removes the worktree but deliberately preserves the local branch, then the next launch materializes a fresh environment.
- Office handoff cleanup also preserves the branch when it releases a worktree.

Before deleting a task or performing a hard reset, commit and push anything you need. A cleanup-script failure does not save the directory: Kandev logs the failure and proceeds. If `git worktree remove --force` fails, managed cleanup can fall back to deleting the directory and pruning Git's stale worktree record.

## Troubleshooting

- **No agent/client available:** launch or prepare the session and confirm its executor is healthy. Workspace Git actions can reconstruct runtime control after a backend restart, but still need a valid task environment.
- **Remote/authentication error:** test `git fetch origin` inside the same executor workspace. Verify SSH agent forwarding, token/credential helper availability, remote URL, DNS, and firewall access there.
- **Pull fetched the wrong branch:** Kandev always uses `origin` and, once any upstream exists, the current local branch name. Align local and remote branch names or use an explicit terminal command.
- **Rebase failed but no rebase remains:** detected rebase conflicts are auto-aborted. Use the returned `conflict_files`, resolve with a manual workflow, or merge instead.
- **Merge remains conflicted:** this is expected. Resolve and commit, or choose Abort Merge. Do not start another Git operation until the repository is consistent.
- **Change-request creation failed after push:** the branch may already be remote. Fix `gh`, `glab`, GitLab workspace-token, or `az` authentication as applicable, then retry without assuming the push was rolled back. GitLab retries reuse an existing open MR with the same source and target.
- **Operation timed out:** inspect status before retrying. A client timeout or lost WebSocket response does not prove the underlying command did nothing.
- **Multi-repository operation failed at workspace root:** choose the repository in the toolbar or include its exact `repo` subpath in the request.
- **Missing work after cleanup:** inspect the preserved local branch for Reset Environment/handoff, or the remote branch if it was pushed. Task deletion may already have force-deleted the local branch.

Related guides: [Configuration](configuration.md), [Executors](executors.md), [Operations](operations.md), and [WebSocket API](websocket-api.md).
