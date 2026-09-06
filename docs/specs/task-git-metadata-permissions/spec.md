---
status: building
created: 2026-08-19
owner: kandev
---

# Task-Scoped Git Metadata Permissions

## Why

Task worktrees keep their checkout under a task-owned directory but may keep Git metadata in a linked-worktree entry beneath the source repository's common Git directory. A task sandbox must support ordinary `git add` and `git commit` without granting the source checkout or the common Git control plane.

## What

For every mutable, task-owned repository checkout, the lifecycle resolves a fresh `GitMetadataProjection`. The projection validates the checkout's `.git` shape, linked-worktree reciprocal pointers, common-directory ownership, and current branch ref. It describes only the task checkout and owned linked-worktree Git directory, the common object store, and the current branch ref and reflog paths required by an ordinary commit.

The common `.git` root and its `worktrees` parent are never writable grants. A linked-worktree entry is reopened only after the shared parent is denied or masked. Source checkouts and sibling worktree administration remain unavailable.

The projection is ephemeral. It is recomputed from task-environment repository records at launch, resume, recovery, and after an attachment. Its version and hash diagnose a stale running executor; they are never persisted as authorization authority.

Clone-based executors do not receive a host projection: their canonical checkout does not exist until their prepare step has run. A launch with one or more mutable repository specs instead carries an intent-only policy requirement. The executor attests the canonical checkout it actually created, derives a regular-checkout projection there, and installs that policy before it starts the agent process. Host checkout, Git-dir, and source-root paths are never copied into a container, SSH host, Sprite, agent environment, or user-facing error.

## Executor contract

Every executor either attests a narrow native policy, applies a layered container mount plus inner agent policy, mediates the allowed Git operations, or fails before starting the agent with `git_metadata_projection_unsupported`. A failed validation returns `git_metadata_projection_invalid` without source paths in user-facing details.

The projection separates agent authorization from executor backing mounts. `AgentWritablePaths` contains the owned worktree metadata, object store, exact active ref and reflog, and their exact `.lock` siblings. `MountSupportPaths` may contain the ref and reflog parent directories that a POSIX bind mount needs for Git's native create-and-rename protocol, but those paths are executor plumbing and never become agent policy. An executor that consumes mount support paths must also attest and install the exact-path inner policy; otherwise it fails closed. Sibling refs and reflogs therefore inherit read-only access from the common directory even when the container layer needs writable backing directories.

Standalone and ACP agents receive the server-authored additional directories and a compatible filesystem-policy overlay. Docker mounts common metadata read-only, masks sibling worktree administration, and overlays only the owned entry and required writable dependencies. Remote executors resolve their own remote checkout paths and may not import host paths into their policy. Repository-less and read-only environments receive no Git write projection.

| Executor | Git metadata enforcement |
|---|---|
| Standalone Codex ACP | A server-authored `CODEX_CONFIG` profile reads common metadata, denies the shared worktrees parent, and reopens only validated writable dependencies. |
| Local Docker | The container prepare path attests its canonical clone checkout before agentctl starts an agent. It uses the contained checkout's regular `.git` directory only; it never compiles a host mount plan for a clone. Later attachments materialize independently in the container, stop the child, batch-attest the complete ordered checkout set, and restart only with the returned policy. |
| SSH / Sprites Codex ACP | After the remote prepare step, a regular, non-symlinked task checkout is revalidated on that host. The server sends a `CODEX_CONFIG` profile that writes only that checkout's `.git` directory. Later attachments follow the same per-executor materialize, stop, batch-attest, configure, and restart transaction. A linked remote checkout, missing capability, incompatible legacy sandbox setting, or stale child fails closed with recovery guidance. |
| Remote Docker | Unsupported until the executor can resolve and attest a remote policy; it fails closed before `CreateInstance` and directs mutable work to local Docker, SSH, or Sprites with Codex. |
| Other agents or repository-less workspaces | Agents without an enforceable profile fail closed for mutable repositories. Repository-less and read-only workspaces need no projection. |

## Attachment and cleanup

An attached repository is usable only after the complete replacement projection is installed and attested. Clone runtimes treat this as a compensating transaction across every distinct executor filesystem: after materialization they stop the child, batch-attest the exact ordered root set, configure from the returned pairs, and restart. A failed transaction removes new clones and restores a prior execution only after its former roots and policy are re-attested; otherwise the child remains stopped and the execution is failed. No runtime may retain a stale mount or profile.

On stop and terminal cleanup, executor policy artifacts and mounts are revoked before task-owned worktrees are removed. Cleanup never changes ownership, permissions, or contents of source checkouts, common Git metadata, object stores, refs, or sibling worktree administration.

## Failure modes

| Condition | Result |
|---|---|
| Forged, symlinked, escaped, or stale Git metadata | `git_metadata_projection_invalid`; agent does not start. |
| Executor or agent cannot enforce the projection | `git_metadata_projection_unsupported` with recovery guidance. |
| Projection changes before policy installation | Re-resolve and fail closed if it no longer validates. |
| Another authorized process holds a Git lock | Native Git lock error; no copied metadata or alternate lock domain. |
| Clone executor cannot attest its canonical checkout before agent start | `git_metadata_projection_unsupported` with executor-specific recovery guidance; no host path is disclosed. |

Decisions: [ADR-2026-08-19-task-scoped-git-metadata-projection](../../decisions/2026-08-19-task-scoped-git-metadata-projection.md) and [ADR-2026-08-20-clone-policy-and-acp-workspace-roots](../../decisions/2026-08-20-clone-policy-and-acp-workspace-roots.md).
