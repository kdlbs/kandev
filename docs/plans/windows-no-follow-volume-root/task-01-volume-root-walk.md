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

- `internal/system/winpath` (new, Windows implementation plus a non-Windows
  stub): `OpenDirectoryPath`, `OpenVolumeRoot`, relative opens with explicit
  disposition and sharing, `OpenTarget`, `PathParts`, reparse checks, handle
  duplication, and delete marking.
- `internal/system/storage/workspaces`: the dependency cleanup and directory
  handle helpers delegate to `winpath`; `openOrCreateWindowsDependencyDirectoryPath`
  walks from the volume root; ancestors use traverse access.
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

- Non-Windows code paths and the DELETE share-mode contract.
- Git shims in `branch_cleanup_interruption_test.go`.

## Acceptance

1. `OpenDirectoryNoFollow`, `CreateDirectoryNoFollow`, and the managed runtime
   cache root open ordinary directories below a drive letter.
2. A junction at any component below the volume root, including an ancestor of
   the task root, makes open, create, and remove fail before they touch the
   junction target.
3. The packages that consume the boundary (`internal/worktree`,
   `internal/task/inventoryrepair`) pass on Windows without new failures.

## Verification

```bash
# from the repository root, on Windows
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  ./internal/system/winpath/ ./internal/system/storage/workspaces/ \
  ./internal/agent/managedruntime/ ./internal/worktree/ ./internal/task/inventoryrepair/)
(cd apps/backend && GOOS=linux CGO_ENABLED=0 go vet ./internal/system/winpath/ \
  ./internal/system/storage/workspaces/ ./internal/agent/managedruntime/)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- 'apps/backend/*.go')
```

## Likely files

- `apps/backend/internal/system/winpath/winpath_windows.go` (new)
- `apps/backend/internal/system/winpath/winpath_other.go` (new)
- `apps/backend/internal/system/storage/workspaces/dependency_cleanup_windows.go`
- `apps/backend/internal/system/storage/workspaces/directory_handle_windows.go`
- `apps/backend/internal/system/storage/workspaces/directory_handle_windows_test.go`
- `apps/backend/internal/agent/managedruntime/cache_windows.go`
- `apps/backend/internal/agent/managedruntime/repair_lock_windows.go`
- `apps/backend/internal/agent/managedruntime/cache_windows_test.go`
- `apps/backend/internal/worktree/checkout_inspection_test.go`

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
