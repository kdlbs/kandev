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

## Executor contract

Every executor either attests a narrow native policy, applies a layered container mount plus inner agent policy, mediates the allowed Git operations, or fails before starting the agent with `git_metadata_projection_unsupported`. A failed validation returns `git_metadata_projection_invalid` without source paths in user-facing details.

Standalone and ACP agents receive the server-authored additional directories and a compatible filesystem-policy overlay. Docker mounts common metadata read-only, masks sibling worktree administration, and overlays only the owned entry and required writable dependencies. Remote executors resolve their own remote checkout paths and may not import host paths into their policy. Repository-less and read-only environments receive no Git write projection.

## Attachment and cleanup

An attached repository is usable only after the complete replacement projection is installed and attested. At the existing idle rebind boundary, lifecycle may restart an agent child or recreate a container; failure restores the previous workspace and projection together.

On stop and terminal cleanup, executor policy artifacts and mounts are revoked before task-owned worktrees are removed. Cleanup never changes ownership, permissions, or contents of source checkouts, common Git metadata, object stores, refs, or sibling worktree administration.

## Failure modes

| Condition | Result |
|---|---|
| Forged, symlinked, escaped, or stale Git metadata | `git_metadata_projection_invalid`; agent does not start. |
| Executor or agent cannot enforce the projection | `git_metadata_projection_unsupported` with recovery guidance. |
| Projection changes before policy installation | Re-resolve and fail closed if it no longer validates. |
| Another authorized process holds a Git lock | Native Git lock error; no copied metadata or alternate lock domain. |

Decision: [ADR-2026-08-19-task-scoped-git-metadata-projection](../../decisions/2026-08-19-task-scoped-git-metadata-projection.md).
