---
id: "01-bound-ignored-directory-evidence"
title: "Bound ignored directory evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001
acceptance_criteria:
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.1
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.2
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.7
system_design:
  - ../../specs/tasks/system-design/archive-source-manifest.md
---

# Task 01: Bound ignored directory evidence

## Summary

Preserve ignored directory path evidence without reading descendant contents.
Keep ignored regular-file hashes and all existing source ownership checks.

## In scope

- Use TDD. First reproduce the current directory hash with
  `TestArchiveManifestOmitsIgnoredDirectoryContents` in the existing manifest test file.
- Add the `content_omission` field with the exact value `ignored_directory`.
  Set it only for a no-follow directory entry whose Git status is `!!`.
- Replace `TestArchiveManifestCapturesIgnoredDirectoryContents` with omission
  assertions. Preserve `TestArchiveManifestCapturesIgnoredUntrackedFile`.
- Cover root/nested `node_modules`, `dist`, arbitrary ignored directory names,
  tracked changes, untracked source, ignored regular files, and ignored symlinks.
- Prove no descendant open through a fake directory handle that rejects it.
  Include a tracked file below a dependency-named directory to prevent name filtering.
- Verify snapshot JSON round trips retain the marker and old snapshots decode
  without it. Missing paths and foreign ownership still fail closed.

## Out of scope

Context propagation, worker startup, dirty-submodule selection, and retry policy.

## Acceptance

- Every ignored directory entry has its path/status and omission marker, with no
  digest and no descendant content read.
- Ignored regular files and symlinks preserve existing identities. Deletion
  records remain distinguishable from omissions.
- Snapshot persistence remains mandatory before cleanup and old evidence remains readable.

## Verification

```bash
(cd apps/backend && go test -count=1 -run '^TestArchiveManifest' ./internal/worktree)
(cd apps/backend && go test -count=1 -run 'TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity|TestCleanupRetryReusesPersistedSourceManifest|TestCleanupPersistsSourceManifestAfterStopBeforeWorktreeRemoval|TestCleanupCaptureFailureBlocksWorktreeRemoval' ./internal/task/service)
```

## Files likely touched

- `apps/backend/internal/worktree/manager_archive_source_manifest.go`
- `apps/backend/internal/worktree/manager_archive_source_manifest_test.go`
- `apps/backend/internal/task/service/resource_cleanup_source_manifest_boundary_test.go`

## Dependencies

None.

## Risks

Missing digest must never imply deletion when the omission field is present.
Do not follow symlinks or broaden the omission to non-ignored source.

## Parallelism

`sequential`

## Inputs

- [Manifest design: Data and contracts](../../specs/tasks/system-design/archive-source-manifest.md#data-and-contracts).
- [Ignored directory decision](../../decisions/2026-10-01-archive-manifest-ignored-directories.md).
- Existing ignored-file, directory, symlink, dirty-submodule, and snapshot tests.
- The plan's registered-worktree RED reproduction.

## Results

Implemented `content_omission: "ignored_directory"` for Git-ignored directory
entries after no-follow type inspection. Added real Git coverage for root and
nested dependency directories, `dist`, an arbitrary ignored directory, ignored
files and symlinks, tracked paths below `node_modules`, snapshot JSON round-trip,
old snapshot decoding, and mutation-independent omitted evidence. A fake handle
proves the ignored directory itself is never opened.

Passed:

- `(cd apps/backend && go test -count=1 -run '^TestArchiveManifest' ./internal/worktree)`
- `(cd apps/backend && go test -count=1 -run 'TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity|TestCleanupRetryReusesPersistedSourceManifest|TestCleanupPersistsSourceManifestAfterStopBeforeWorktreeRemoval|TestCleanupCaptureFailureBlocksWorktreeRemoval' ./internal/task/service)`
