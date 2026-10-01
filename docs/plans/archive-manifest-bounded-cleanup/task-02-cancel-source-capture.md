---
id: "02-cancel-source-capture"
title: "Cancel source capture promptly"
status: done
wave: 2
depends_on:
  - "01-bound-ignored-directory-evidence"
plan: "plan.md"
requirements:
  - REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001
acceptance_criteria:
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.1
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.4
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
system_design:
  - ../../specs/tasks/system-design/archive-source-manifest.md
---

# Task 02: Cancel source capture promptly

## Summary

Propagate the cleanup context through filesystem capture and bounded digest reads.
Cancellation preserves the worktree and prevents partial evidence from authorizing cleanup.

## In scope

- Use TDD. Start with `TestArchiveManifestDigestStopsOnCancellation`.
  A custom reader cancels its context after one chunk and counts subsequent reads.
- Thread `ctx` through the parser, path-handle traversal, directory collector,
  entry collector, subdirectory/file helpers, and digest reader. Update all callers.
- Cover cancellation before traversal, between entries, during a regular-file
  read, and during dirty-submodule capture. Include deadline expiry separately.
- Close every file and directory handle on error. Preserve close failures when
  capture succeeds and preserve cancellation identity when errors coexist.
- Add `TestCleanupManifestCancellationBlocksRemoval`: use a controlled capture
  implementation that observes cancellation and the real claimed cleanup path.
  Assert no worktree cleanup, no capture-complete marker, and retained retry diagnostics.
- Preserve hashes for identical inputs, sorted directory records, and no-follow
  symlink/ancestor safety. Do not persist partial results or extend deadlines.

## Out of scope

Worker startup, timeout duration, retry limits, and changes to source selection.

## Acceptance

- `errors.Is` detects cancellation and deadline causes through capture wrappers.
  The digest reads no subsequent chunk after observed cancellation.
- Cancellation closes owned handles and returns no successful partial manifest.
- A cancelled claimed job retains its worktree and persists retry state through
  the existing detached transition context.

## Verification

```bash
(cd apps/backend && go test -race -count=1 -run '^TestArchiveManifest' ./internal/worktree)
(cd apps/backend && go test -count=1 -run 'TestCleanupManifestCancellationBlocksRemoval|TestCleanupCaptureFailureBlocksWorktreeRemoval|TestCleanupRetryReusesPersistedSourceManifest|TestCleanupPersistsSourceManifestAfterStopBeforeWorktreeRemoval|TestRetryTaskResourceCleanupJobPersistsAfterRunContextCancellation|TestRetryTaskResourceCleanupPersistsAfterDeadlineExpiry' ./internal/task/service)
```

## Files likely touched

- `apps/backend/internal/worktree/manager_archive_source_manifest.go`
- `apps/backend/internal/worktree/manager_archive_source_manifest_test.go`
- `apps/backend/internal/task/service/resource_cleanup_source_manifest_boundary_test.go`

## Dependencies

Task 01 establishes the directory omission and shares the capture/test files.

## Risks

A filesystem syscall can outlive cancellation. The implementation bounds work
between syscalls rather than guaranteeing interruption inside a kernel read.
Avoid `io.Copy` paths that bypass context checks through `WriterTo` or `ReaderFrom`.

## Parallelism

`sequential`

## Inputs

- [Manifest design: Capture cancellation](../../specs/tasks/system-design/archive-source-manifest.md#capture-cancellation).
- Existing handle cleanup, symlink, dirty-submodule, and cancellation tests.
- The plan's mid-read cancellation RED reproduction.

## Results

Threaded the cleanup attempt context through source parsing, no-follow path
traversal, recursive directory capture, submodule capture, and bounded digest
reads. The reader stops after the first observed cancellation, returns no partial
digest, and closes owned handles while preserving cancellation and close errors.
The cleanup boundary test proves a canceled claimed job retains retry diagnostics,
persists no partial manifest, and does not remove its worktree.

Passed:

- `(cd apps/backend && go test -race -count=1 -run '^TestArchiveManifest' ./internal/worktree)`
- `(cd apps/backend && go test -count=1 -run 'TestCleanupManifestCancellationBlocksRemoval|TestCleanupCaptureFailureBlocksWorktreeRemoval|TestCleanupRetryReusesPersistedSourceManifest|TestCleanupPersistsSourceManifestAfterStopBeforeWorktreeRemoval|TestRetryTaskResourceCleanupJobPersistsAfterRunContextCancellation|TestRetryTaskResourceCleanupPersistsAfterDeadlineExpiry' ./internal/task/service)`
