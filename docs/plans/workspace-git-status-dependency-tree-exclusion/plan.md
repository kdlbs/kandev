---
created: 2026-08-30
status: complete
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
legacy_specs: []
---

# Implementation Plan: Workspace Git Status Dependency-Tree Exclusion

## Overview

Prevent an untracked `node_modules` tree from entering workspace status before a repository ignore rule exists. First add failing process and browser regressions. Then split tracked and untracked collection so Git excludes dependency trees during enumeration. Use the same untracked query for the monitor fingerprint and the full status snapshot.

The confirmed incident came from task `cc6ea1f5-723d-4354-b46d-529aae52c013`. `pnpm install` completed at 18:55:12 UTC, and `.gitignore` was created at 18:58:01 UTC. At 18:56:54 UTC, the status snapshot grew from 20 to 4,917 files. Of the affected workspace's 4,893 ignored entries, 4,889 were below `node_modules`. The oversized snapshot stayed cached until a 26-file refresh at 19:02:31 UTC.

The root cause is server-side enumeration, not frontend merging. Agentctl runs `git status --porcelain --untracked-files=all`, which emits every untracked dependency file before an ignore rule exists. The monitor independently runs `git ls-files --others --exclude-standard` and stats every returned path. The frontend replaces each status snapshot as designed.

## Scope

### In scope

- Exclude untracked `node_modules` directories at every repository depth before Git returns individual paths.
- Preserve all tracked changes below `node_modules`.
- Preserve ordinary untracked, nonignored files and their existing synthetic diffs.
- Apply one shared untracked-query policy to full status and monitor fingerprints.
- Add process regressions and one desktop Changes-panel regression.

### Out of scope

- Excluding `.next`, `dist`, `build`, or other generated directory names.
- Adding repository files or changing a user's Git ignore configuration.
- Filtering status only in the frontend or after Git has enumerated the dependency tree.
- Changing Git-status response fields, WebSocket events, or Changes-panel layout.
- Adding mobile-specific coverage. Desktop and mobile consume the same backend snapshot, and no responsive UI behavior changes.

## Technical approach

### Shared untracked query

- Add `apps/backend/internal/agentctl/server/process/workspace_git_untracked.go` with the shared Git argument definition for untracked paths.
- Use `git ls-files --others --exclude-standard --exclude=node_modules/ -z` so Git applies repository ignores and the Kandev dependency exclusion during traversal.
- Parse NUL-separated paths without Git quoting or newline ambiguity. Check the observation context while adding paths.
- Keep the exclusion list limited to `node_modules/`. Document any later addition through the same requirement, design, and ADR boundary.

### Full status collection

- Update `parseGitStatusOutput` in `apps/backend/internal/agentctl/server/process/workspace_git_status.go` to run the tracked query with `--untracked-files=no`.
- Run the shared untracked query with the same observation class and context.
- Add returned paths to `GitStatusUpdate.Untracked` and `GitStatusUpdate.Files` with the existing untracked `FileInfo` shape before `enrichUntrackedFileDiffs` runs.
- Preserve the current porcelain parser for tracked, staged, deleted, renamed, and mixed-facet paths.
- Do not remove paths after status parsing. The exclusion must reduce Git output and agentctl work.

### Monitor fingerprint

- Update `getUntrackedFilesID` in `apps/backend/internal/agentctl/server/process/workspace_monitor.go` to use the shared query and NUL parser.
- Keep the existing path sanitization and mtime fingerprint for eligible untracked files.
- Ensure dependency-only changes do not alter the fingerprint, while ordinary untracked changes still do.

### Changes-panel evidence

- Extend `apps/web/e2e/tests/git/git-changes-panel.spec.ts` with `omits untracked node_modules before repository ignore exists`.
- Create an ordinary untracked source file and nested pnpm-style dependency content without adding `.gitignore`.
- Wait for the ordinary source file in Changes, then assert that no `node_modules` path or package file appears.
- Clean up the test files through the existing Git fixture boundary.

## Tests

- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.13:** Add `TestGetGitStatus_ExcludesUntrackedNodeModules` in `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`. Create top-level and nested `node_modules` files without ignore rules, plus an ordinary untracked source file. Assert that only the source file enters status and receives a synthetic diff.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.14:** Add `TestGetGitStatus_PreservesTrackedNodeModules` in `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`. Force-add and commit a file below `node_modules`, change it, and assert that its tracked status and diff remain visible. Keep `TestGetGitStatus_UntrackedFileWithSpaces` green to protect the new NUL parser.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.15:** Add `TestGetUntrackedFilesID_ExcludesNodeModules` in `apps/backend/internal/agentctl/server/process/workspace_monitor_test.go`. Assert that dependency-only creation and modification keep the same fingerprint, while an ordinary untracked file changes it.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.6 and .8:** Keep the existing diff-budget and large-set tests green. The change reduces the eligible input set but does not change enrichment limits or eligible-path retention.
- Add `TestGetGitStatus_UsesConsistentIndexSnapshotAcrossTransitions` to cover both untracked-to-staged and tracked-to-untracked index changes between the two status queries.
- Add focused parser and snapshot lifecycle coverage for embedded-newline paths, cancellation, error cleanup, and linked-worktree Git directories.

## E2E tests

- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.13 and .14:** Run the Chromium scenario `omits untracked node_modules before repository ignore exists` in `apps/web/e2e/tests/git/git-changes-panel.spec.ts`. The Changes panel must show the ordinary untracked source file and omit the dependency path.
- No mobile-specific case is required. The correction occurs before the shared status payload reaches either Changes layout.

## Work orders

- [x] [Task 01: Bound dependency-tree status enumeration](task-01-bound-dependency-tree-status-enumeration.md)

Execution is sequential in the primary conversation. The browser regression, process tests, shared query, and both callers form one TDD slice and share the same status contract.

## Verification results

- RED process run: `TestGetGitStatus_ExcludesUntrackedNodeModules` and `TestGetUntrackedFilesID_ExcludesNodeModules` failed against the prior implementation because dependency paths entered status and changed the monitor fingerprint. `TestGetGitStatus_PreservesTrackedNodeModules` passed, protecting the tracked-path contract.
- RED Chromium run: `omits untracked node_modules before repository ignore exists` failed at the intended assertion because one dependency row was visible.
- GREEN focused process run: all three new tests passed.
- GREEN tracking-transition process run: `TestGetGitStatus_UsesConsistentIndexSnapshotAcrossTransitions` passed for both untracked-to-staged and tracked-to-untracked interleavings.
- GREEN parser and index-lifecycle process run: `TestParseGitUntrackedOutput` covered NUL-separated paths with embedded newlines and cancellation; `TestSnapshotGitIndex*` covered cleanup, cancellation/error cleanup, and linked-worktree Git directories.
- GREEN existing regressions: the untracked-space, enrichment, and diff-budget tests passed (9 tests).
- GREEN race run: `go test -race ./internal/agentctl/server/process` passed, covering 737 tests.
- GREEN Chromium run: `omits untracked node_modules before repository ignore exists` passed (1 test).
- `make -C apps/backend fmt`, backend lint (0 issues), web lint, and `git diff --check` passed.
- The first ambient `make -C apps/backend test` run selected the task runtime's `/root/.kandev/config.yaml` through inherited `KANDEV_INTERNAL_CONFIG_FILE` and `KANDEV_INTERNAL_CONFIG_HOME_FILE` values. The isolated rerun with both variables unset passed the complete backend suite.
- PR fixup remediation: full status now runs both queries against a temporary stable Git-index snapshot, and E2E cleanup derives dependency directories from `dependencyPaths`.

## Risks

- A path filter on the tracked query can hide tracked files below `node_modules`. Only the untracked query receives the exclusion.
- A filter after parsing leaves Git enumeration and process-output cost unchanged.
- Line-based parsing can regress valid filenames that contain newlines. The new untracked query must remain NUL-separated in both callers.
- Splitting collection adds one Git subprocess to each full observation. Both commands must retain the caller's admission class, context, and timeout behavior.
- Separate argument builders can make the monitor and full observer drift. They must share one definition.
- Separate status queries can observe different index membership. The full observer uses one temporary index snapshot for both queries.
