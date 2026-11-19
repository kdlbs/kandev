---
id: "01-volume-root-walk"
title: "Walk Windows no-follow opens from the volume root"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.10
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.6
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 01: Walk Windows no-follow opens from the volume root

## Summary

No-follow opens of absolute Windows paths succeed for ordinary directories and
still refuse symbolic links and junctions at every component below the volume
root.

## In scope

- `internal/common/winpath` (shared path parsing and Windows native opens):
  `PathParts` and `NTPath` use explicit Windows rules and are tested on every
  platform. Windows builds provide `OpenDirectoryPath`,
  `OpenDirectoryPathWithDisposition`, `OpenVolumeRoot`, relative opens with
  explicit disposition and sharing, `OpenTarget`, reparse
  checks, handle duplication, and delete marking.
- `internal/system/storage/workspaces`: cleanup and directory-handle code call
  `winpath` directly. Creation uses the shared disposition-aware path walk;
  ancestors use traverse access.
- `internal/agent/managedruntime`: the cache root, inventory, and repair-lock
  configuration file open through `winpath`.
- Windows tests for plain directories, nested creation, junctions as a path
  component, as the target, as an ancestor of the root, and as the root, plus
  npx execution-tree removal through a junction.
- `checkout_inspection_test.go`: on Windows the fake Git is written as
  `git.cmd`, because `exec.LookPath` ignores an extensionless `git` script, and
  the "Git inspection denied" case asserts a marker written by the shim. Before
  this fix every no-follow open failed first, so that case passed without
  reaching Git.

## Out of scope

- Non-Windows filesystem operations and the DELETE share-mode contract.
- Git shims in `branch_cleanup_interruption_test.go`.

## Acceptance

1. `OpenDirectoryNoFollow`, `CreateDirectoryNoFollow`, and the managed runtime
   cache root open ordinary directories below a drive letter.
2. `PathParts` returns the complete server and share as the root for ordinary
   and extended UNC paths, so native opens start at the share root.
3. A junction at any component below the volume root, including an ancestor of
   the task root, makes open, create, and remove fail before they touch the
   junction target.
4. Managed npm cache validation refuses drive and share roots before file access
   and accepts an ordinary cache directory.
5. The packages that consume the boundary (`internal/worktree`,
   `internal/task/inventoryrepair`) pass on Windows without new failures.

## Verification

```bash
# from the repository root, on Windows
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  ./internal/common/winpath/ ./internal/system/storage/workspaces/ \
  ./internal/agent/managedruntime/ ./internal/worktree/ ./internal/task/inventoryrepair/)
(cd apps/backend && GOOS=linux CGO_ENABLED=0 go vet ./internal/common/winpath/ \
  ./internal/system/storage/workspaces/ ./internal/agent/managedruntime/)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

## Likely files

- `apps/backend/internal/common/winpath/path.go` (new)
- `apps/backend/internal/common/winpath/path_test.go` (new)
- `apps/backend/internal/common/winpath/winpath_windows.go` (moved)
- `apps/backend/internal/system/storage/workspaces/dependency_cleanup_windows.go`
- `apps/backend/internal/system/storage/workspaces/directory_handle_windows.go`
- `apps/backend/internal/system/storage/workspaces/directory_handle_windows_test.go`
- `apps/backend/internal/agent/managedruntime/cache_windows.go`
- `apps/backend/internal/agent/managedruntime/repair_lock_windows.go`
- `apps/backend/internal/agent/managedruntime/cache_windows_test.go`
- `apps/backend/internal/agent/managedruntime/cache_test.go` (volume-root cases)
- `apps/backend/internal/worktree/checkout_inspection_test.go`
- `.github/workflows/backend-tests.yml` (Windows package allowlist)

## Results

Measured on Windows 10 22H2 (build 19045) with `go test -trimpath -tags fts5
-count=1`, on `main` and on this change:

| Package | `main` | This change |
| --- | --- | --- |
| `internal/system/storage/workspaces` | 9 failures, including `TestDirectoryHandlePinsWorktreeAcrossPathReplacement` and `TestCleanupDependencies*` | pass |
| `internal/agent/managedruntime` | 6 failures | 1 failure (`TestRepairNPMGlobalConfigPath`, also on `main`) |
| `internal/worktree` (main-checkout tests) | `TestMainCheckoutAdmissionAndReuse` refuses every healthy checkout | `TestMainCheckoutAdmissionAndReuse`, `TestMainCheckoutRejectsInvalidMetadata`, and `TestMainCheckoutInspectionPreservesOperationalErrors` pass |

- The whole `internal/worktree` package exceeds the 10-minute default timeout
  on this host before and after the change; up to the timeout, 108 tests fail
  on `main` and 18 with this change, none of them new. The remaining failures
  come from extensionless Git shims and symbolic-link privilege on Windows.
- `internal/task/inventoryrepair`: 42 failures on `main`, 41 with this change,
  none new; they share the same host causes.
- The new junction tests run without symbolic-link privilege and pass.
- `GOOS=linux go vet` passes for the three touched packages.
- The Windows CI job runs `./internal/system/storage/workspaces/...`; this
  host reproduces the open failure that those tests would hit wherever the
  object manager rejects `\??\C:` under `OBJ_DONT_REPARSE`.

### Maintainer follow-up

- From `apps/backend`, `go test ./internal/common/winpath ./internal/agent/managedruntime ./internal/system/storage/workspaces` passes on Linux.
- `GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-winpath.test.exe ./internal/common/winpath` passes.
- `GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-managedruntime.test.exe ./internal/agent/managedruntime` passes.
- `GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-workspaces.test.exe ./internal/system/storage/workspaces` passes.
- This changed-file Go lint command reports `0 issues`:

  ```bash
  bash scripts/lint-go-changed \
    apps/backend/internal/agent/managedruntime/cache.go \
    apps/backend/internal/agent/managedruntime/cache_test.go \
    apps/backend/internal/agent/managedruntime/cache_windows.go \
    apps/backend/internal/agent/managedruntime/repair_lock_windows.go \
    apps/backend/internal/common/winpath/path.go \
    apps/backend/internal/common/winpath/path_test.go \
    apps/backend/internal/common/winpath/winpath_windows.go \
    apps/backend/internal/system/storage/workspaces/dependency_cleanup_windows.go \
    apps/backend/internal/system/storage/workspaces/directory_handle_windows.go \
    apps/backend/internal/system/storage/workspaces/directory_handle_windows_test.go
  ```
- `python3 scripts/list-docs.py validate` passes.
- `python3 scripts/lint-spec-files.py --all` passes.
