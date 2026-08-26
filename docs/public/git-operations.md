---
title: "Git Operations"
description: "Use Kandev worktrees, commits, remote operations, change requests, and cleanup safely."
---

# Git Operations

Kandev runs Git commands in the selected repository workspace for a task session. The Worktree executor gives a session a dedicated host worktree; Local uses the configured shared checkout, while container and remote executors use runtime-specific workspace behavior described in [Executors](executors.md). The browser sends a WebSocket request to the backend, which forwards it to `agentctl` in that executor. This keeps the command's filesystem, Git configuration, network access, SSH agent, and provider CLI in the same environment as the agent.

Use the task's **Changes** panel to inspect, stage, discard, commit, push, reset, or rename a branch. The task toolbar and command panel also expose **Commit Changes**, **Push**, **Pull**, provider-appropriate pull-request or merge-request creation, **Rebase**, and **Merge** when a Git-capable session is selected.

## Quick path

1. Inspect the selected repository in **Changes**.
2. Stage only the files you intend to commit.
3. Run required checks before pushing or opening a change request.
4. Treat discard, reset, amend, force-push, and cleanup as irreversible or history-changing operations.

## Prerequisites and trust boundary

The repository must be a valid Git checkout in the executor workspace and the session's `agentctl` must be reachable. Remote commands use the remote named `origin`; configure its URL and credentials in the executor where the command runs before relying on Pull, Push, or change-request creation. Rebase and Merge use `origin` when it exists, or a local base branch when it does not. The workspace's provider automation identity does not replace the task's Git credential policy or executor-local SSH setup; see [Executors](executors.md#workspace-automation-identity-and-task-git-transport).

Git commands from the agent shell and Git actions from the **Changes** panel use different permission paths. The Changes panel sends its Git action through Kandev to `agentctl`. It does not run the action inside the agent shell. The agent shell remains subject to the selected agent's permission mode.

In a restricted agent mode, `git status` can work while `git add` or `git commit` fails. These write Git metadata, including `.git/index.lock`.

If the error says that `.git/index.lock` already exists or is held, another Git process can own the lock. Stop other Git operations and inspect the lock before retrying. Remove a stale lock only after you confirm that no Git process owns it. The Changes panel uses the same worktree, so it does not bypass an active lock.

If the agent cannot create `.git/index.lock` because its permission mode blocks the metadata write, the repository and Kandev Git integration can still work.

To commit agent edits, use the **Changes** panel. No mode change is needed. If the agent shell must commit, select an agent mode that allows Git metadata writes. Mode names come from the installed agent. For example, Codex can expose `Agent (default)` and `Agent (full access)`.

Managed Improve Kandev tasks are the exception to the ordinary single-remote push description. The
canonical `origin` still identifies `kdlbs/kandev` for pulls, base comparisons, issue lookup, and
the pull-request target. Before launch, Kandev prepares one exact fork remote for the task branch
and configures Git to use it for pushes. Run ordinary **Push** or `git push`; do not rename `origin`
or replace it with a fork. The PR workflow uses the canonical repository and an explicit
`<fork-owner>:<branch>` head. Other tasks keep the normal `origin` push behavior.

### Compare a fork pull request

When a linked pull request uses a fork, Kandev stores the provider-qualified target repository and
target branch on the exact task-repository attachment. It then fetches that target into a
comparison-only remote-tracking ref. This ref is read-only and is authoritative for Changes,
commits, cumulative diff, and ahead/behind counts.

The comparison target does not replace `origin`, the checked-out branch, or the push route. Kandev
shows the target as `<owner>/<repository>:<branch>` without credentials. If the target cannot be
validated, fetched, or resolved, Kandev marks comparison as unavailable and does not substitute a
same-named branch from `origin`. Numeric comparison totals are hidden until the exact target is
available.

When a provider retargets the pull request, Kandev refreshes the stored target and the live session
comparison. Selecting a task base branch or removing the owning pull-request association clears the
explicit target. A PR with incomplete fork identity is not guessed or applied to another repository.

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

`{title}` is an ASCII-safe, lower-case task-title slug and `{suffix}` is a short collision-avoidance value. Repository settings can change the template. When `pull_before_worktree` is omitted it defaults to `true`: Kandev must refresh and verify the base branch before creating or recreating the worktree. The public configuration defaults both fetch and fast-forward pull timeouts to 60 seconds. An authentication, network, timeout, missing-ref, divergent-ref, or uncertain-ancestry failure stops task preparation and records a repository-specific launch error. Kandev does not create the worktree from a stale local or remote-tracking fallback.

If the repository is intentionally offline, open its workspace repository settings and disable
**Always pull before creating a new worktree**. This preserves the local workflow, but it also
opts out of the freshness guarantee and allows the task to use the local base state. Re-enable the
setting before relying on remote changes for later task launches.

### Named branch policies

Open **Settings → Workspaces → _workspace_ → Repositories**, edit a repository, and expand
**Branch policies**. A policy names the base branch, branch-name template, and pull-request target.
Policies belong to that repository. Create, edit, and delete actions take effect immediately.
The branch controls list local and remote branches. You can search the list or refresh it from Git.

The base branch is the starting point for the new task branch. The pull-request target is its merge
destination. These values are usually the same. A Gitflow Release policy can start from `develop`
and target `main`.

In **New Task** or **New subtask**, the repository branch picker shows saved policies before raw
branches. Select a policy to create a fresh task branch from its base branch. The policy template
controls the branch name. Raw branches keep their existing checkout behavior. Each policy uses one
line in the picker. Point to or focus its information icon to see the saved values. On touch devices,
tap the icon.

The **Gitflow starter** can create Feature, Bugfix, Hotfix, and Release policies in one operation.
It requires two different existing branches and does not change Git branches. A task stores the
selected policy values when it is created. Later policy edits or deletion do not change that task's
branch or pull-request target. Kandev's pull-request dialog uses the saved target by default. You can
select a different target before creation.

Kandev adds the saved target to the agent's task context. The instruction tells the agent to pass the
target to the provider CLI. For example, a GitHub agent uses `gh pr create --base <target>`. Kandev
does not add a separate shell environment value, and it does not prevent the user from changing the
target.

Policies are not available in **Quick Chat**, **Remote**, **Add Sources**, or **Add Branch** flows.

When a task opens an existing branch or GitHub PR, Kandev fetches that branch; for a numbered GitHub PR it can fetch `refs/pull/NUMBER/head`, including fork PRs. If the intended branch is already checked out in another worktree, the new worktree uses a suffixed local branch and tracks the original `origin` branch when available. The required-refresh rule still applies before that new worktree is created.

Tasks created without an initial title can expose the one-shot `set_task_title_kandev` handoff when
**Settings → General → Task Actions → Agent-generated task titles** is enabled. After the owning
session accepts its final title, Kandev regenerates Kandev-managed branch names from that title and
updates the stored branch snapshots. It never renames a repository row with an explicit checkout
branch (for example, a GitHub PR branch) or a Local/Local PC checkout. A branch manually selected
before the title call is preserved as well. Multi-repository tasks apply these rules independently to
each repository, and a Git or snapshot persistence failure does not undo the accepted title.

After creation, Kandev copies any repository-configured files and runs its setup script. Setup-script failure is non-fatal: the worktree remains and the session surfaces a warning. Cleanup scripts run before worktree removal, but their failure also does not prevent removal.

## Everyday operations

All operations below run in the selected repository workspace.

| UI operation | Effective Git behavior | Important consequence |
|--------------|------------------------|-----------------------|
| Pull | `git pull origin BRANCH`, optionally with `--rebase`. | Uses the current branch when any upstream exists. With no upstream it falls back to `origin/main`, then `origin/master`, then the current branch. It does not parse an upstream that points to a differently named remote branch. |
| Push | `git push origin CURRENT_BRANCH`; adds `--set-upstream` when requested or no upstream exists. | Force Push uses `--force-with-lease`, not unconditional `--force`. It still rewrites remote history when the lease is valid. Managed Improve Kandev tasks use the branch's prepared fork push remote instead of `origin`. |
| Rebase | If `origin` exists, fetches `origin BASE` and rebases onto `origin/BASE`. Without `origin`, rebases onto the local `refs/heads/BASE`. | Rewrites local commits. If conflict files are detected from Git output, Kandev attempts `git rebase --abort` automatically and returns the file list. |
| Merge | If `origin` exists, fetches `origin BASE` and merges `origin/BASE`. Without `origin`, merges the local `refs/heads/BASE`. | Conflicts are deliberately left in the worktree. Resolve and commit them, or use Abort Merge. |
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

For an ordinary task, **Create PR** first runs:

```bash
git push --set-upstream origin HEAD
```

For a managed Improve Kandev task, the prepared branch push remote replaces
`origin` for this push. Keep `origin` canonical and create the GitHub pull
request with the explicit fork head shown below.

It then selects a provider from the `origin` hostname:

| Provider | Required runtime tools | Creation behavior |
|----------|------------------------|-------------------|
| GitHub | Authenticated `gh` CLI | `gh pr create` with title, body, current head, optional base, and optional `--draft`. Managed Improve Kandev uses `--repo kdlbs/kandev` and an explicit `<fork-owner>:<branch>` head. |
| GitLab | Authenticated `glab` CLI or the active workspace's matching GitLab PAT | Reuses an existing open MR for the same source/target or creates one with title, description, current source, explicit or project-default target, and optional draft state. The configured GitLab origin must match the HTTPS or SSH `origin`. |
| Azure Repos | Authenticated `az` CLI plus the `azure-devops` extension | `az repos pr create` with parsed organization, project, repository, source, optional target, and optional draft. |

Other remote providers are rejected by this action. A normal Git push can still work with another provider. A returned GitHub PR or GitLab MR is asynchronously associated with the originating task repository. Azure creation returns a URL but does not create a provider review association.

Title is required. Body and base branch may be empty. GitHub and Azure delegate an empty base to their provider CLI; GitLab resolves the project default branch before creation. The web UI defaults new change requests to draft and waits up to 120 seconds. Provider credentials and remote push permission must exist in the executor, and Git hooks or branch policies can still reject the push or change request.

For GitLab, Kandev preserves a self-managed connection's scheme and never retries against another host. It prefers `glab` when installed and uses the workspace-injected `GITLAB_TOKEN` REST path when the CLI is absent or its create command fails. The successful WebSocket payload keeps the compatibility field name `pr_url` and adds `provider:"gitlab"`. Association runs after that response; use **Link GitLab merge request** if the MR was created but the link is missing.

<details>
<summary>WebSocket operation reference</summary>

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

</details>

## Cleanup and data loss

Worktree cleanup runs the repository cleanup script, forcibly removes the Git worktree directory, and may remove the local branch:

- Normal task deletion cleans all owned task worktrees and runs `git branch -D` for their local branches. Remote branches are not deleted, but uncommitted and unpushed-only work can be lost.
- **Reset Environment** is allowed only when no task session is `STARTING` or `RUNNING`. It can optionally push first; a failed requested push aborts the reset. Teardown removes the worktree but deliberately preserves the local branch, then the next launch materializes a fresh environment.
- Office handoff cleanup also preserves the branch when it releases a worktree.

Before deleting a task or performing a hard reset, commit and push anything you need. A cleanup-script failure does not save the directory: Kandev logs the failure and proceeds. If `git worktree remove --force` fails, managed cleanup can fall back to deleting the directory and pruning Git's stale worktree record.

## Troubleshooting

- **No agent/client available:** launch or prepare the session and confirm its executor is healthy. Workspace Git actions can reconstruct runtime control after a backend restart, but still need a valid task environment.
- **Remote/authentication error during worktree preparation:** test `git fetch origin` inside the same executor workspace. Verify the remote URL, SSH agent or key, known-hosts entry, token or credential-helper availability, DNS, and firewall access there. Do not paste command output containing tokens or authenticated URLs into a task or issue.
- **Host GitHub SSH setup:** for host-based Git credentials, run `gh config set git_protocol ssh --host github.com`, restart Kandev, and retry the task. For Docker, SSH, or Sprites, configure the same Git and SSH access in that executor instead of relying on the host.
- **Merge or Rebase without `origin`:** the selected base branch must exist locally. If it does not, Kandev returns `base branch "BASE" does not exist locally` before changing history. If `origin` exists, Kandev does not fall back to a local branch after a fetch or authentication error.
- **Pull fetched the wrong branch:** Kandev always uses `origin` and, once any upstream exists, the current local branch name. Align local and remote branch names or use an explicit terminal command.
- **Rebase failed but no rebase remains:** detected rebase conflicts are auto-aborted. Use the returned `conflict_files`, resolve with a manual workflow, or merge instead.
- **Merge remains conflicted:** this is expected. Resolve and commit, or choose Abort Merge. Do not start another Git operation until the repository is consistent.
- **Change-request creation failed after push:** the branch may already be remote. Fix `gh`, `glab`, GitLab workspace-token, or `az` authentication as applicable, then retry without assuming the push was rolled back. GitLab retries reuse an existing open MR with the same source and target.
- **Operation timed out:** inspect status before retrying. A client timeout or lost WebSocket response does not prove the underlying command did nothing.
- **Multi-repository operation failed at workspace root:** choose the repository in the toolbar or include its exact `repo` subpath in the request.
- **Missing work after cleanup:** inspect the preserved local branch for Reset Environment/handoff, or the remote branch if it was pushed. Task deletion may already have force-deleted the local branch.

Related guides: [Configuration](configuration.md), [Executors](executors.md), [Operations](operations.md), and [WebSocket API](websocket-api.md).
