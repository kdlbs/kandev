---
id: "03-trimmed-builds"
title: "Integrate external trimpath work"
status: done
wave: 3
depends_on:
  - "02-busy-cleanup"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-GO-CACHE-001.1
  - AC-SYSTEM-PAGE-GO-CACHE-001.2
  - AC-SYSTEM-PAGE-GO-CACHE-001.3
  - AC-SYSTEM-PAGE-GO-CACHE-001.4
system_design:
  - ../../specs/system-page/system-design/go-cache-reclamation.md
---

# Task 03: Integrate external trimpath work

## Summary

Consume [PR #4160](https://github.com/kdlbs/kandev/pull/4160), which owns trimpath
build defaults and path-dependent fixture fixes. Reconcile existing overlapping
workspace edits and implement only demonstrated residual integration gaps.

## In scope

- Refresh PR state, merged commit, and current base before integration.
- Preserve current work, then integrate the merged PR through a normal rebase.
- Reconcile backend Make and fixture/helper overlaps against the landed implementation.
- Audit root Make, CI, launcher, and E2E command propagation for uncovered direct invocations.
- Assess whether the existing local reuse probe adds missing durable coverage.

## Out of scope

A competing trimpath implementation, duplicate fixture helpers, new cache policy,
host-global Go settings, and a blanket repeat of the external PR's full test suite.

## Acceptance

- PR #4160 is integrated at a recorded landed revision, with no duplicate flag conventions or fixture helpers.
- Actual residual gaps have targeted fixes and evidence; unmodified paths are not assumed to be gaps.
- Cleanup changes pass relevant integration checks under trimmed builds, with shared-cache fallback behavior preserved.

## Verification

Before integration, inspect the dependency without mutating GitHub:

```bash
gh pr view 4160 --repo kdlbs/kandev --json state,mergedAt,mergeCommit,headRefOid,url
```

After integration, run from the repository root:

```bash
make -C apps/backend check-make-shells
bash scripts/release/runtime-bundle.test.sh
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/system/storage/... -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/backendapp -run 'Test.*(Storage|GoCache|Quarantine)' -count=1)
git diff --check
```

If retained or changed, run `bash scripts/go-cache-reuse.test.sh` for the local
probe. Run focused fixture tests for every conflict resolution or residual fix.
Reuse successful external CI only for its exact revision and unchanged paths.
Broaden validation only when combined changes create an unresolved concern.

## Files likely touched

- `apps/backend/Makefile` and overlapping test fixtures: conflict resolution only.
- `Makefile`, `.github/workflows/backend-tests.yml`, launcher/E2E scripts: only proven residual gaps.
- `apps/backend/internal/testutil/`: retain one fixture-path solution after comparison.
- `scripts/go-cache-reuse.test.sh`: only if it supplies missing durable coverage.

## Dependencies

Task 02 supplies the cleanup integration to verify. PR #4160 has landed, so
this dependency is complete. Task 04 can proceed directly after Task 02.

## Risks

Integration must preserve the merged build/test defaults and avoid duplicate
flag conventions or fixture helpers.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/system-page/requirements/go-cache-reclamation.md)
- [Design](../../specs/system-page/system-design/go-cache-reclamation.md)
- [Plan](plan.md)
- PR #4160 merged commit `341f8941376e29f21b0873ae94a989cf2a595c58`, integrated
  with `origin/main` at `14d473d3e0cc747ab3cb9e8455da812a665d6b90`.

## Results

PR #4160 was refreshed, confirmed merged, and integrated. Backend Make build and
test recipes use direct `-trimpath` flags; backend CI retains its direct-command
flag, and the cross-worktree reuse probe verifies cache reuse and source
invalidation. Duplicate trimpath changes and fixture helpers were removed during
integration. No additional root Makefile convention was needed.

Validation passed:

- `make -C apps/backend check-make-shells`
- `bash scripts/go-cache-reuse.test.sh`
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/system/storage/... -count=1)`
- Full backend build and test suite, recorded in the plan results.
- `bash scripts/release/runtime-bundle.test.sh` (8 tests)
