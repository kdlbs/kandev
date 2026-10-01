---
status: current
system: tasks
requirements:
  - REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
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
| `REQ-TASKS-RUNTIME-CLEANUP-001` | [Background recovery](#background-recovery), specifically `AC-TASKS-RUNTIME-CLEANUP-001.32` |

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
ID. It records HEAD and the SHA-256 digest of `git ls-files --stage -z`, which
includes unmerged stages and does not write Git objects. Changed paths contain
porcelain status and a SHA-256 identity, except for ignored directory entries.
Symlink identities hash the link
target. Dirty submodule identities hash sorted relative paths and file/link
identities while omitting Git administrative metadata.

Ignored directory entries retain their Git-reported path and `!!` status.
They omit `content_sha256` and set the additive
`content_omission: "ignored_directory"` field. Capture validates the path and
uses the existing no-follow handle to identify its type. It does not open the
directory or inspect descendants. Ignored regular files retain their content
digests. Tracked directories, dirty submodules, and symlink identities retain
their existing content rules. A name such as `node_modules` or `dist` does not
exclude a tracked or non-ignored entry.

The omission field is absent for historical entries and all other entry types.
A deletion has no digest and no omission field. No consumer can treat an omitted
directory as proof that its contents are unchanged or absent. Retained snapshots
remain readable and successful capture markers remain authoritative for reuse.
There is no evidence backfill or cleanup-row migration.

The boundary and compatibility choice are recorded in
[Ignored directories in cleanup evidence](../../../decisions/2026-10-01-archive-manifest-ignored-directories.md).

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
worktree removal follows that failure. Unmerged indexes are represented by the
same staged-index digest as resolved indexes. A dirty submodule is represented
by a recursive digest of its working tree rather than rejected as a directory.

If the process crashes before the manifest compare-and-set, the worker has not
entered worktree cleanup. If it crashes after the compare-and-set, retries use
the stored evidence and continue cleanup without recapturing a potentially
changed or partially removed checkout.

## Capture cancellation

The attempt context reaches the entry parser, parent-handle traversal, directory
collector, subdirectory collector, file collector, and digest reader.
Capture checks cancellation before filesystem work and between entries.
A bounded read loop checks cancellation before each read and after each read
before accepting content. It checks again before returning a complete digest.
All open files and directory handles close on cancellation or failure.

Cancellation errors wrap `context.Canceled` or `context.DeadlineExceeded` so
`errors.Is` works through cleanup error wrappers. The worker does not persist
partial manifests, extend the attempt deadline, or remove a worktree after a
capture error. The existing detached transition context records retry state,
but it cannot accept an incomplete source manifest.

These checks bound additional userspace work. They cannot interrupt a single
filesystem syscall that the operating system has blocked. Dirty submodule
content capture keeps its existing semantics and receives the same context.

## Background recovery

`Service.StartTaskResourceCleanupWorker` registers one owned worker and returns
without calling `resumeTaskResourceCleanupJobs` synchronously. The goroutine
performs its first resume immediately, before its ticker/select wait.
It captures the startup prepared-job cutoff once and reuses that cutoff across
resume retries. It does not cancel uncommitted preparation created after start.

Repository resume failures produce the existing cleanup warning and retain
`resumePending` until the complete reset/reconciliation path succeeds. Wakes
and the existing retry timer drive recovery. Worker registration success is
distinct from an asynchronous recovery error. The public synchronous
`ResumeTaskResourceCleanupJobs` method retains its explicit recovery contract.

Start and stop serialize worker ownership through a lifecycle mutex distinct
from the wake-state mutex. Stop cancels the worker and joins it without holding
the mutex used by cleanup wake producers. A successor start cannot register a
new worker until the prior worker drains. Repeated starts and stops are
idempotent. Capture cancellation lets stop drain source inspection promptly.

The archive-cascade recovery gate and outbox readiness obligations remain in
[Runtime Startup Registry](runtime-startup-registry.md). This change moves
task-resource filesystem cleanup off `system.Service.StartBackground`; it does
not bypass the mandatory archive-cascade gate.

Cascade cleanup keeps the recovery contract in
`AC-TASKS-RUNTIME-CLEANUP-001.32`. A deadline is insufficient evidence of a
permanent error. The normal backoff, exhausted diagnostics, generation fencing,
and claim compare-and-set remain in force.

## Implementation plans

- [Original source capture](../../../plans/archive-source-manifest/plan.md).
- [Bounded archive manifest cleanup](../../../plans/archive-manifest-bounded-cleanup/plan.md).

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
