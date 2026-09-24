---
id: "01-capture-task-cleanup-source-evidence"
title: "Capture task cleanup source evidence"
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
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.4
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.5
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.6
system_design:
  - ../../specs/tasks/system-design/archive-source-manifest.md
---

# Task 01: Capture and expose task cleanup source evidence

## Outcome

Archive and delete cleanup persists a task-bound source manifest after runtime
stop and before worktree removal. Authorized workspace callers can retrieve
the retained evidence after task deletion.

## Scope

- Verify exact repository and registered worktree identity before capture.
- Record Git and changed-path identities without storing source bytes, including
  read-only staged-index records, unmerged indexes, dirty submodules, symlinks,
  and rename status.
- Persist the completed manifest under the claimed cleanup job before
  destructive work and reuse it on retries.
- Keep the task-scoped route authorized and test its foreign-workspace denial.

## Exclusions

- Backfilling old cleanup records or asserting historical task cleanliness.
- Public documentation or user-facing UI for the internal Coordinator audit
  route.

## Verification

```bash
cd apps/backend && go test -count=1 -run '^TestArchiveManifest' ./internal/worktree
cd apps/backend && go test -count=1 -run 'TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity|TestDeleteTaskWithDiscardConsentPersistsAndCleansDirtyWorktree|TestTaskLifecycleCleanup_MissingWorktree' ./internal/task/service
cd apps/backend && go test -count=1 -run '^TestHTTPGetArchiveSourceManifestDeniesForeignWorkspace$' ./internal/task/handlers
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Results

Completed in the PR fixup.

- Passed: `cd apps/backend && go test -count=1 -run '^TestArchiveManifest' ./internal/worktree`.
  This includes a staged-file regression that verifies inspection does not
  increase the repository's loose Git object count.
- Passed: `cd apps/backend && go test -count=1 -run 'TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity|TestDeleteTaskWithDiscardConsentPersistsAndCleansDirtyWorktree|TestTaskLifecycleCleanup_MissingWorktree|TestCleanupPersistsSourceManifestAfterStopBeforeWorktreeRemoval|TestCleanupCaptureFailureBlocksWorktreeRemoval|TestCleanupRetryReusesPersistedSourceManifest' ./internal/task/service`.
- Passed: `cd apps/backend && go test -count=1 -run 'TestHTTPGetArchiveSourceManifestDeniesForeignWorkspace' ./internal/task/handlers`.
- Passed under the race detector: the `TestArchiveManifest` worktree cases and the three cleanup boundary/retry cases above.
- Passed: `cd apps/backend && golangci-lint run ./internal/worktree ./internal/task/service ./internal/task/handlers --new-from-rev=25271b6e7fc04344e3e8e851bc73257dd783e538 --timeout=5m`.
- Passed: `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
- Passed: `.github/scripts/pr-docs.cjs` `validateCoverage` accepted this changed
  work order for the backend cleanup and route paths.
- The historical archived task rows are not represented as reconstructed or
  confirmed-clean evidence.
