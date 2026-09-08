---
id: "01-bound-dependency-tree-status-enumeration"
title: "Bound dependency-tree status enumeration"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.6
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.8
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.13
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.14
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.15
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 01: Bound Dependency-Tree Status Enumeration

## Summary

Exclude untracked `node_modules` trees in Git before agentctl receives individual dependency paths. Preserve tracked changes and ordinary untracked files, and use the same policy for full snapshots and monitor fingerprints.

## In scope

- Add the browser and process regressions before production changes.
- Add one shared NUL-separated untracked Git query with the `node_modules/` exclusion.
- Split full status into tracked porcelain collection and eligible untracked collection.
- Apply the shared untracked query to monitor fingerprinting.
- Preserve all existing status, diff, cancellation, admission, and payload contracts outside this exclusion.

## Out of scope

- Other generated directory names.
- Frontend filtering or presentation changes.
- Git ignore-file creation or modification.
- Status schema, route, or WebSocket changes.

## Acceptance

- Untracked top-level and nested `node_modules` files do not enter a status snapshot, synthetic diff work, or the monitor fingerprint when no ignore rule exists.
- Tracked changes below `node_modules` and ordinary untracked files remain visible with their established metadata and diffs.
- The Chromium Changes regression shows the ordinary untracked file and no dependency-tree path from the same completed snapshot.

## Verification

If `apps/node_modules` is absent, run the workspace install once before the E2E or frontend lint command.

```bash
cd apps
rtk pnpm install --frozen-lockfile

cd backend
rtk go test ./internal/agentctl/server/process -run 'Test(GetGitStatus_ExcludesUntrackedNodeModules|GetGitStatus_PreservesTrackedNodeModules|GetUntrackedFilesID_ExcludesNodeModules|GetGitStatus_UsesConsistentIndexSnapshotAcrossTransitions|ParseGitUntrackedOutput|SnapshotGitIndex.*)$'
rtk go test ./internal/agentctl/server/process -run 'Test(GetGitStatus_UntrackedFileWithSpaces|EnrichUntrackedFileDiffs|DiffBudget)'
rtk go test -race ./internal/agentctl/server/process

cd ../web
rtk pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "omits untracked node_modules before repository ignore exists"

cd ../..
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
rtk pnpm --dir apps --filter @kandev/web lint
rtk git diff --check
```

During RED, run the focused Go command and the focused Chromium command after adding their tests. Confirm both fail against the current implementation before changing production code. The final report must contain the later GREEN results.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_untracked.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_index.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_monitor.go`
- `apps/backend/internal/agentctl/server/process/workspace_monitor_test.go`
- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`

## Dependencies

None.

## Risks

- Passing the exclusion to tracked status collection can hide valid tracked changes.
- Reusing line parsing for untracked output can mishandle filenames with embedded newlines.
- Different argument lists in the observer and monitor can reintroduce repeated dependency scans.
- The extra full-status Git command must not lose the current observation class or cancellation behavior.

## Parallelism

`sequential`

## Inputs

- `REQ-PLATFORM-WORKSPACE-GIT-STATUS-001`, especially acceptance criteria .6, .8, and .13 through .15.
- `docs/specs/platform/system-design/workspace-git-status.md`, section `Generated untracked dependency trees`.
- `docs/decisions/2026-08-30-bound-untracked-dependency-enumeration.md`.
- Existing path, cancellation, diff-budget, and interactive-admission tests in the process package.
- Existing untracked-file Changes coverage in `apps/web/e2e/tests/git/git-changes-panel.spec.ts`.

## Results

- RED process run: `TestGetGitStatus_ExcludesUntrackedNodeModules` and `TestGetUntrackedFilesID_ExcludesNodeModules` failed against the prior implementation because dependency paths entered status and changed the monitor fingerprint. `TestGetGitStatus_PreservesTrackedNodeModules` passed, protecting the tracked-path contract.
- RED Chromium run: `omits untracked node_modules before repository ignore exists` failed at the intended assertion because one dependency row was visible.
- GREEN focused process run: all three new tests passed.
- GREEN tracking-transition process run: `TestGetGitStatus_UsesConsistentIndexSnapshotAcrossTransitions` passed for both untracked-to-staged and tracked-to-untracked interleavings.
- GREEN parser and index-lifecycle process run: `TestParseGitUntrackedOutput` covered NUL-separated paths with embedded newlines and cancellation; `TestSnapshotGitIndex*` covered cleanup, cancellation/error cleanup, and linked-worktree Git directories.
- GREEN existing regressions: the untracked-space, enrichment, and diff-budget tests passed (9 tests).
- GREEN race run: `go test -race ./internal/agentctl/server/process` passed, covering 737 tests.
- GREEN Chromium run: `omits untracked node_modules before repository ignore exists` passed (1 test).
- `make -C apps/backend fmt`, backend lint (0 issues), web lint, and `git diff --check` passed.
- The first ambient backend suite run selected `/root/.kandev/config.yaml` through inherited `KANDEV_INTERNAL_CONFIG_FILE` and `KANDEV_INTERNAL_CONFIG_HOME_FILE` values. The isolated rerun with both variables unset passed the complete backend suite.
- PR fixup remediation: full status now runs both queries against a temporary stable Git-index snapshot, and E2E cleanup derives dependency directories from `dependencyPaths`.
