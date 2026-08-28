---
created: 2026-08-27
status: done
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
legacy_specs: []
---

# Implementation Plan: Preserve Archive Resume Identity

## Overview

Keep an archived task resumable after cleanup removes its worktree directory.
Direct archive must retain the task environment owner. Every archive cleanup
pass must also retain the local branch ref. Unarchive can then use normal
worktree preparation to reactivate the recorded workspace.

## Confirmed root cause

The affected task was archived through `Service.ArchiveTask`. Its cleanup
snapshot set `delete_environment_row=true`. The successful cleanup job then
deleted the `task_environments` row and its repository rows. The session kept a
dangling environment ID, so resume could not resolve the workspace owner.

The cleanup also had two worktree teardown paths. The environment destroyer
preserved the local branch. The later batch cleanup removed the same worktree
with branch deletion enabled. The hosting provider had already deleted the
merged remote branch, so the recorded checkout branch was unavailable locally
and remotely.

Cascade archive already passes `deleteEnvironmentRow=false`. Existing tests
cover that path. They did not cover direct archive with a production-like
environment destroyer and a successful cleanup job.

This behavior violates `AC-TASKS-RUNTIME-CLEANUP-001.7`. That criterion already
requires an unarchived task to recreate or reactivate its recoverable worktree.
No new product requirement is needed.

## Scope

### In scope

- Preserve `task_environments` and `task_environment_repos` rows during direct
  and cascade archive cleanup.
- Preserve local Git branch refs during every archive worktree cleanup pass.
- Keep physical worktree directory removal and soft deletion unchanged.
- Keep delete cleanup behavior unchanged.
- Add production-like SQLite and Git regression coverage.
- Prove that unarchive resume reactivates the recorded worktree.

### Out of scope

- Automatic repair of other damaged task rows.
- Pushing branches during archive or unarchive.
- Recreating uncommitted files from a Git snapshot.
- Frontend changes, schema changes, or public API changes.
- Changes to task deletion semantics.

## Technical approach

### Retain the archive owner

Change the direct `Service.ArchiveTask` snapshot to set
`DeleteEnvironmentRow=false`. Keep the existing cascade value. Add a service
regression that runs the durable cleanup with an environment destroyer wired.
The cleanup job must succeed while the environment and repository rows remain.

The test must not rely on a missing destroyer. That setup makes cleanup retry
before row deletion and caused the existing branch-metadata test to pass for the
wrong reason.

### Carry the archive branch policy through batch cleanup

Add a branch-preserving batch operation to the worktree manager. Keep the
existing `CleanupWorktrees` method as the delete-compatible entry point. Both
methods delegate to one implementation with an explicit `removeBranch` value.

The task cleanup service derives branch removal from the lifecycle operation.
Archive and cascade archive pass `removeBranch=false`. Delete paths keep their
current branch policy. If an archive cleanup implementation cannot preserve
branches, fail the cleanup job for retry. Do not fall back to a branch-deleting
method.

This policy applies to the environment teardown and the later batch pass. A
duplicate pass can be idempotent, but it cannot strengthen the disposition.

### Resume proof

Use a real SQLite repository and worktree manager. Archive a task whose branch
exists only as a local ref. Complete cleanup, unarchive the task, and run normal
worktree preparation. Assert that the same environment and worktree identity
become active again at the preserved commit.

## Tests

- `AC-TASKS-RUNTIME-CLEANUP-001.6`: the durable direct-archive snapshot retains
  the environment owner through cleanup and retry boundaries.
- `AC-TASKS-RUNTIME-CLEANUP-001.7`: an unarchived task reactivates the recorded
  worktree when no remote branch exists.
- Existing cascade archive tests remain green and keep
  `deleteEnvironmentRow=false`.
- Existing delete cleanup tests remain green and keep their current branch
  removal behavior.

## E2E tests

No Playwright test is planned. The defect is in backend cleanup persistence and
Git materialization. The SQLite and real-Git integration test covers the full
failure boundary without browser timing or UI state.

## Work orders

- [x] [Task 01: Retain the archive environment owner](task-01-retain-archive-environment.md)
- [x] [Task 02: Preserve archive branch refs](task-02-preserve-archive-branch.md)

## Verification results

Completed on 2026-08-28:

```bash
rtk go test ./internal/task/service -run 'Archive.*(Environment|Worktree|Branch)' -count=1
rtk go test ./internal/worktree -run 'CleanupWorktrees|RestoresReleasedWorktreeAfterArchive|Recreate' -count=1
rtk go test -race ./internal/task/service ./internal/worktree ./internal/orchestrator/executor
rtk make -C apps/backend lint
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
rtk git diff --check
```

The focused service regressions passed. The related worktree command passed 25
tests. The race command passed 2,186 tests across three packages. Backend lint
reported zero issues. All 20 specification-linter tests, all 61 public-doc
tests, and both document validators passed.

## Risks

- A test without the production-like destroyer can hide environment deletion
  behind a retryable setup error.
- A second worktree cleanup pass can delete the branch unless every pass gets
  the same archive policy.
- Trigger inference from `deleteEnvironmentRow` is unsafe. Some delete paths
  capture rows before task deletion and rely on the snapshot after cascading
  rows disappear.
- The integration test must use a branch with no remote ref. Otherwise remote
  recovery can hide local branch deletion.
