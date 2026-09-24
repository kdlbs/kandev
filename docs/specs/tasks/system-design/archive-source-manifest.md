---
status: current
system: tasks
requirements:
  - REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001
---

# Task Cleanup Source Manifest System Design

## Purpose and boundaries

The task cleanup worker owns when manifest evidence becomes durable and whether
filesystem cleanup may proceed. The worktree manager owns Git registration
validation and source-state inspection. Workspace authorization owns read
access to retained cleanup evidence.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001` | [Control flow](#control-flow), [Persistence](#persistence), and [Security](#security) |

## Components and responsibilities

- `Service.persistTaskResourceCleanup` and cascade preparation persist the
  cleanup inventory and lifecycle barrier before task-row mutation. They do not
  capture source contents while a runtime can still write.
- `Service.executeTaskResourceCleanupJob` refreshes and stops runtime targets,
  then captures and compare-and-set persists the manifest before invoking any
  destructive task cleanup.
- `worktree.Manager.CaptureArchiveSourceManifests` verifies the checkout path
  is registered with the recorded repository, reads Git state, and hashes
  changed content without retaining source bytes.
- `Service.GetTaskSourceManifest` validates job and worktree identities and
  authorizes through the cleanup snapshot's persisted workspace identity.
- `GET /api/v1/tasks/:id/archive-source-manifest` returns the service result
  with the task handler's existing not-found behavior for inaccessible tasks.

## Data and contracts

The cleanup snapshot contains one manifest per owned task worktree. A manifest
binds task ID, cleanup-job ID, task-environment ID, worktree ID, and repository
ID. It records HEAD, the staged index tree when available, or an index-file
SHA-256 for an unmerged index. Changed paths contain porcelain status and a
SHA-256 identity. Symlink identities hash the link target. Dirty submodule
identities hash sorted relative paths and file/link identities while omitting
Git administrative metadata.

The existing `archive_source_manifest` field remains additive and
backward-compatible in cleanup snapshots. A capture-complete marker
distinguishes a successful empty inventory from a not-yet-captured retry.

## Control flow

1. A direct or cascade lifecycle operation persists a cleanup job and blocks
   new task resource admission.
2. The worker reloads the exact snapshot and stops every recorded runtime.
   Failed stop operations defer source capture and filesystem cleanup for a
   retry.
3. The worker captures all source manifests from the snapshot's worktree
   inventory. The worktree manager compares Git common directories and checks
   that each exact path appears in `git worktree list` for the recorded
   repository.
4. The worker writes the manifest and capture-complete marker through the
   claimed-job snapshot compare-and-set. A lost claim or failed write aborts
   before destructive cleanup.
5. Only then does the worker remove worktrees and other task resources. A later
   retry reuses the already persisted manifest.
6. Retrieval loads cleanup generations for the requested task, checks manifest
   IDs against the persisted worktree inventory, and authorizes the persisted
   workspace before returning evidence.

## Failure and recovery

Missing or malformed Git state, unreadable indexes, unsafe paths, disappearing
untracked files, unreadable file content, unregistered or foreign worktrees,
and snapshot persistence failures produce a retryable cleanup error. No
worktree removal follows that failure. Unmerged indexes are represented using
the index-file digest when `git write-tree` cannot create a tree because
unmerged entries exist. A dirty submodule is represented by a recursive digest
of its working tree rather than rejected as a directory.

If the process crashes before the manifest compare-and-set, the worker has not
entered worktree cleanup. If it crashes after the compare-and-set, retries use
the stored evidence and continue cleanup without recapturing a potentially
changed or partially removed checkout.

## Persistence

Evidence is stored inside the existing durable task cleanup job snapshot; no
separate table or retention policy is introduced. Existing cleanup-job
retention governs manifest retention. The cleanup-job claim attempt protects
the manifest update from stale workers.

## Security

Manifests store identifiers, Git object IDs, status values, relative paths, and
SHA-256 digests only. Source bytes are streamed into hashes. Symlinks are
identified by their link target and never followed. Retrieval is authorized
against the persisted workspace so deleted task rows do not erase the audit
access boundary. Worktree registration and repository identity checks prevent
cross-task source attribution.

## Observability

Capture and persistence failures are returned to the cleanup job, which records
its retryable error using the existing cleanup lifecycle. No source bytes or
task-controlled content is added to logs or metric labels.
