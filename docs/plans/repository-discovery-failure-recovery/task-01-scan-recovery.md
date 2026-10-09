---
id: "01-scan-recovery"
title: "Preserve repositories across scan failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-004
acceptance_criteria:
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.6
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.7
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.9
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.10
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.12
  - AC-WORKSPACES-LOCAL-REPOSITORIES-004.2
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
---

# Task 01: Preserve repositories across scan failures

## Summary

Continue discovery after denied descendants and preserve usable results from
successful roots. Retain root failure diagnostics and explicit recovery policy.

## In scope

- Write `TestDiscoveryRecoveryDescendantPermission` before the correction.
  Assert readable siblings before and after EACCES and EPERM remain visible.
- Keep root stat/read failures fatal and context cancellation distinct.
- Forward scan trigger and runtime into warnings for exact denied descendants.
- Store copied result slices per normalized root inside the existing cache.
- Write `TestDiscoveryRecoveryMixedRoots`, including two refreshes while the
  clone root remains absent. Assert fresh successful results replace old ones.
- Cover all-root failure, recovery, empty successes, overlap, and cancellation.
- Update `TestRepoWalkerPropagatesAccessDenied` to distinguish root and child.

## Out of scope

UI, API schema changes, root persistence changes, directory creation, and new
Home exclusions are outside this work order.

## Acceptance

- Descendant errors preserve accessible results and diagnostic context. Root
  errors remain reported. Cancellation does not publish an incomplete snapshot.
- Every successful root supplies fresh results, even when another root fails.
  Only failed roots retain prior results. Overlap produces no duplicate paths.
- Existing single-flight, consent, cache-copy, and root-recovery tests pass.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/service -run 'TestDiscoveryRecovery' -count=1 -v)
(cd apps/backend && go test -tags fts5 -race ./internal/task/service -run 'Discovery|DiscoverLocal|RepoWalker|ScanRoot|MacOSHome' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/common/fsdiagnostics -count=1)
git diff --check
```

First record expected failures from the new regression tests. Use deterministic
walk-callback error injection when filesystem permissions cannot be enforced.
Drive the real scan and service path, not a duplicate traversal implementation.
If a real chmod fixture is used, probe enforcement and skip only that fixture
when the executor bypasses permissions. Deterministic coverage must still run.

## Files likely touched

- `apps/backend/internal/task/service/repository_discovery.go`
- `apps/backend/internal/task/service/repository_discovery_state.go`
- `apps/backend/internal/task/service/repository_discovery_test.go`
- New `apps/backend/internal/task/service/repository_discovery_recovery_test.go`
- `apps/backend/internal/task/service/service.go` if the scan function type changes
- `apps/backend/internal/task/service/filesystem_diagnostics.go`
- `apps/backend/internal/common/fsdiagnostics/context.go` as the existing diagnostic contract

## Dependencies

None.

## Risks

Do not reconstruct provenance from repository path containment. Do not mutate
shared cached maps or slices during an in-flight scan. Keep aggregate scan time
unchanged until all roots succeed.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/local-repositories.md), REQ-003 and REQ-004.
- [Design](../../specs/workspaces/system-design/local-repositories.md), Partial scan recovery and Diagnostics.
- Existing `TestDesktopDiscoveryFailurePreservesCachedRepositories` and discovery concurrency tests.

## Results

Implemented descendant-permission recovery and per-root cache replacement.
Inaccessible descendants now produce bounded structured warnings while root
failures remain visible. Successful roots replace their own cached results,
failed roots retain prior results, and independent exact-root snapshots survive
aggregate invalidation during Add and Reconnect. Successful empty scans clear
stale entries, and cancellation does not publish a partial snapshot.

Verification passed:

- `go test -tags fts5 ./internal/task/service -run 'TestDiscoveryRecovery|TestScanRootForReposRootAccessFailureRemainsFatal|TestRepoWalkerPropagatesRootAccessDeniedButSkipsChild' -count=1 -v`
- `go test -tags fts5 ./internal/task/service -run 'TestDiscoveryRecoveryRootSetChangesRetainUnchangedRootSnapshots' -count=1`
- `go test -tags fts5 -race ./internal/task/service -run 'Discovery|DiscoverLocal|RepoWalker|ScanRoot|MacOSHome' -count=1`
- `go test -tags fts5 ./internal/common/fsdiagnostics -count=1`
- `git diff --check`
