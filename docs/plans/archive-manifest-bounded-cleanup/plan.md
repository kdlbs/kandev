---
created: 2026-10-01
status: done
requirements:
  - REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/archive-source-manifest.md
legacy_specs: []
---

# Implementation Plan: Bounded archive manifest cleanup

## Overview

Correct [issue #4130](https://github.com/kdlbs/kandev/issues/4130) without weakening
the pre-cleanup evidence barrier. First omit ignored directory contents, then
make capture cancellable, then move due-job recovery into the owned worker.
These work orders share files and run sequentially. All three work orders are
complete.

The task system owns durable cleanup evidence and recovery. The worktree manager
supplies Git ownership validation and source capture. There are no rendered UI
changes. Existing desktop and phone readiness flows require no layout changes.

## Confirmed root cause and reproduction

The investigation used commit `d2adc37ffebee6893275c4a47bcd91a2033a8271`.
`capturePresentArchiveSourceManifest` requests `--ignored=matching`.
`archiveSourceManifestEntry` hashes every returned entry.
Directory entries recursively reach `archiveSourceManifestReadDigest` through
the directory collector. Those helpers lack the attempt context.

The cleanup worker uses a two-minute attempt budget. A completed uncancellable
hash reaches snapshot persistence after expiration, so the write fails.
The capture marker remains false and each retry repeats the entire hash.
Cascade triggers intentionally retain capped, recoverable retry after attempt eight.

`StartTaskResourceCleanupWorker` calls resume synchronously before its goroutine
starts. Due jobs therefore delay startup. Stop cancels and joins the worker, but
the digest cannot observe cancellation. The reported production shutdown hang
is consistent with this path. A full desktop shutdown hang was not reproduced.

Three temporary tests failed against the current implementation:

| Temporary test | Observed failure | Permanent regression |
| --- | --- | --- |
| `TestIssue4130IgnoredDirectoryDoesNotHashContents` | A registered worktree with one ignored dependency file produces a directory content hash | `TestArchiveManifestOmitsIgnoredDirectoryContents` |
| `TestIssue4130CancellationDuringDigest` | A reader cancels on its first read, but the digest consumes four reads and returns success | `TestArchiveManifestDigestStopsOnCancellation` |
| `TestIssue4130StartupReturnsBeforeResumeDrains` | Start does not return while the repository resume barrier is blocked | `TestStartTaskResourceCleanupWorkerReturnsBeforeRecovery` |

Command: `(cd apps/backend && go test -count=1 -run '^TestIssue4130' ./internal/worktree ./internal/task/service)`.
Both packages failed with the expected assertions. The temporary files were
removed after the command completed. No production or permanent test code changed.

## Scope

### In scope

- Preserve ignored directory path/status evidence with an additive omission field.
- Preserve content hashes for ignored regular files and existing source types.
- Propagate attempt cancellation through all capture helpers and close handles.
- Start recovery immediately in one owned goroutine and preserve resume retry.
- Serialize worker start/stop and preserve the original prepared-job cutoff.
- Prove archive/delete persistence ordering, retry reuse, and shutdown drainage.

### Out of scope

- Change cascade retry limits, deadline duration, or dirty-worktree consent.
- Retain partial evidence or persist a completed manifest under a fresh deadline.
- Change dirty-submodule content selection or untracked dependency enumeration.
- Backfill manifests, rewrite successful snapshots, or add tables or configuration.
- Change the mandatory archive-cascade readiness gate, public APIs, or UI.

## Technical approach

### Manifest boundary

Update `internal/worktree/manager_archive_source_manifest.go`.
Add `ContentOmission string` with `json:"content_omission,omitempty"` to
`ArchiveSourceManifestEntry`. Its only new value is `ignored_directory`.
Validate the relative path and classify it through existing no-follow handles.
For `!!` plus a directory type, record the omission without opening descendants.
Do not infer directory type from a name or trailing slash alone.

Keep the existing `--ignored=matching` query. A missing ignored path remains a
capture error. Ignored symlinks retain link-target digests without dereference.
Only true deletion status permits a missing digest without the omission marker.
The service embeds this entry type in snapshots, so JSON round trips must retain
the field. Historical entries without the field retain their meaning.

### Context propagation

Thread `ctx` from capture through entry parsing, parent traversal, directory
collection, subdirectory collection, file collection, and digest reads.
Update direct helper callers and tests, including any file-digest helper retained.
Use bounded read chunks, for example a 32 KiB buffer. Check the context before
and after reads and before returning a successful hash. Do not use a detached
context or return a partial digest as success.

Maintain sorted directory records, no-follow handle ownership, symlink behavior,
Git metadata exclusions, and close-error handling. The task service must keep
`ArchiveSourceManifestCaptured` false after cancellation. Snapshot persistence
and claim fencing remain prerequisites for all destructive cleanup.

### Worker ownership

Update `internal/task/service/resource_cleanup_jobs.go` and worker state in
`service.go`. Start validates its context, registers cancellation/wake ownership,
accounts for the goroutine, starts it, and returns.
The worker attempts resume immediately before its first wait.
It preserves the original startup cutoff across failed resume retries.

Add a lifecycle mutex for complete start/stop serialization. Keep it separate
from the wake-state mutex. Stop cancels and joins without blocking wake producers.
All due-job execution stays in the owned worker. Existing explicit synchronous
resume callers retain their behavior.

The first asynchronous resume failure must log a warning and retain resume state.
Existing tests that expect a recovery error from Start must instead assert
registered-worker success, asynchronous failure evidence, and eventual recovery.
`storage.Runtime` retains registration-failure health reporting.

## Tests

| Criteria | Evidence |
| --- | --- |
| Manifest `.3`, `.7` | Mixed ignored `node_modules/`, nested workspace dependencies, `dist/`, ignored regular file, tracked changes, untracked source, and ignored symlink. Directory mutation does not create a content digest. A fake handle rejects descendant opens. |
| Manifest `.1`, `.4`, `.8` | Pre-cancelled context, cancellation between entries, cancellation during a multi-chunk read, deadline expiry, closed handles, and no capture marker or cleanup after cancellation. |
| Manifest `.2`, `.5`, `.6` | Existing ownership, foreign-workspace, missing-path, symlink, rename, index, and source-isolation regressions remain passing. |
| Manifest `.9` | Channel-driven blocked recovery proves Start returns, initial recovery starts immediately, Stop cancels/joins, and concurrent start cannot replace a draining worker. |
| Runtime `.32` | Existing capped retry and cascade-never-terminal tests remain passing. Deadline errors preserve retry and diagnostics. |

## End-to-end evidence

Task 03 includes a Go lifecycle integration using a real registered Git worktree
and the task-service cleanup path. Archive and delete cases must reach persisted
evidence and physical cleanup with ignored dependencies present.
This backend-only change needs no Playwright flow or screenshot.

## Work orders

- [x] [Task 01: Bound ignored directory evidence](task-01-bound-ignored-directory-evidence.md)
- [x] [Task 02: Cancel source capture promptly](task-02-cancel-source-capture.md)
- [x] [Task 03: Recover cleanup in the owned worker](task-03-background-cleanup-recovery.md)

## Verification results

Investigation: three expected RED failures, as recorded above.
Implementation is complete after the explicit user request. Task 01 checks passed:

- `(cd apps/backend && go test -count=1 -run '^TestArchiveManifest' ./internal/worktree)`
- `(cd apps/backend && go test -count=1 -run 'TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity|TestCleanupRetryReusesPersistedSourceManifest|TestCleanupPersistsSourceManifestAfterStopBeforeWorktreeRemoval|TestCleanupCaptureFailureBlocksWorktreeRemoval' ./internal/task/service)`

Task 02 checks passed:

- `(cd apps/backend && go test -race -count=1 -run '^TestArchiveManifest' ./internal/worktree)`
- `(cd apps/backend && go test -count=1 -run 'TestCleanupManifestCancellationBlocksRemoval|TestCleanupCaptureFailureBlocksWorktreeRemoval|TestCleanupRetryReusesPersistedSourceManifest|TestCleanupPersistsSourceManifestAfterStopBeforeWorktreeRemoval|TestRetryTaskResourceCleanupJobPersistsAfterRunContextCancellation|TestRetryTaskResourceCleanupPersistsAfterDeadlineExpiry' ./internal/task/service)`

Task 03 checks passed:

- `(cd apps/backend && go test -race -count=1 -run 'Test.*(TaskResourceCleanup|Cleanup|ArchiveManifest|ArchiveTaskCleanup|TaskLifecycleCleanup|StartupActivation|WorkerResume|WorkerRetries|PeriodicReconciliation)' ./internal/task/service ./internal/worktree)`
- `(cd apps/backend && go test -count=1 ./internal/system/storage)`
- `(cd apps/backend && go test -count=1 -run '^TestHTTPGetArchiveSourceManifestDeniesForeignWorkspace$' ./internal/task/handlers)`
- `(cd apps/backend && go build -tags fts5 -o /tmp/kandev-issue4130-check ./cmd/kandev)`

The design-package checks passed:

- `python3 scripts/list-docs.py validate`: catalog and references valid.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `git diff --check`: no whitespace errors.
- `git status --short -- docs/plans/archive-manifest-bounded-cleanup`: all four
  package files present and untracked. The package remains unstaged and uncommitted.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual documentation-only
  changes are exempt. A preflight with the three intended production paths
  returns `ok: true`, `status: covered`, and all three work orders.

Implementation documentation checks also passed: the spec catalog and spec
lint passed, `git diff --check` is clean, and the coverage validator returned
`ok: true` with all changed paths covered by this plan. Public task and Git
guidance already describes asynchronous cleanup; the manifest omission marker
is internal evidence, so no public-doc wording changed.

The final work order lists the exact implementation and documentation gates.

## Risks

- Omitted directory evidence cannot prove descendant integrity. Consumers must
  preserve the explicit omission marker and avoid stronger audit claims.
- Dirty submodules keep recursive content capture. Their installed artifacts can
  still exhaust an attempt, but cancellation and background recovery bound impact.
- Context checks cannot interrupt a single blocked operating-system syscall.
- Recovery errors become asynchronous warnings rather than Start return values.
- The cutoff and worker lifecycle lock require race coverage to prevent newer
  prepared jobs from cancellation or old/new workers from overlap.

## Related delivery records

- [Original source manifest package](../archive-source-manifest/plan.md) records
  completed historical delivery. This package owns its changed directory semantics.
- [Ignored directory decision](../../decisions/2026-10-01-archive-manifest-ignored-directories.md).
- [Durable cascade decision](../../decisions/2026-09-05-durable-task-cascade-mutations.md).

## Documentation impact

Internal docs change now. Public docs do not describe the internal manifest
format. Implementation must revisit public task/worktree guidance if its existing
cleanup description conflicts with the delivered behavior.
