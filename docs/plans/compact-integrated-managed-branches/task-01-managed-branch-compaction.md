---
id: "01-managed-branch-compaction"
title: "Implement safe managed-branch compaction and recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.18
  - AC-TASKS-RUNTIME-CLEANUP-001.19
  - AC-TASKS-RUNTIME-CLEANUP-001.20
  - AC-TASKS-RUNTIME-CLEANUP-001.21
  - AC-TASKS-RUNTIME-CLEANUP-001.22
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 01: Implement Safe Managed-Branch Compaction and Recovery

## Summary

Implement the complete backend slice that records authoritative branch
metadata, safely compacts fully integrated managed local branches, restores
their exact heads, and revisits retained archived branches through bounded
storage maintenance.

## In scope

- Canonical task-environment repository schema, migrations, CRUD, and CAS.
- Integration-ref propagation through lifecycle and executor materialization.
- Central manager eligibility, exact local-ref deletion, recovery refs,
  receipts, metrics, and concurrency behavior.
- All task cleanup callers, handoff cleanup, and archived maintenance wiring.
- Exact-head unarchive/recreate recovery and interruption remediation.
- Durable contracts, ADR, public operations guidance, and focused tests.

## Out of scope

- UI, browser E2E, remote mutation, network fetch during cleanup, and historical
  ownership backfill.

## Acceptance

- Only a uniquely owned, inactive, unprotected managed local branch whose exact
  head is contained by its persisted integration ref can be compacted.
- Compaction and recovery preserve an exact reachable head across concurrent or
  interrupted operations, and every ambiguity retains the branch.
- Every named terminal caller and bounded archived maintenance uses the shared
  policy and exposes bounded observability.

## Verification

```bash
(cd apps/backend && go test ./internal/worktree -count=1)
(cd apps/backend && go test -race ./internal/worktree -run 'MaintainArchivedBranches|BranchCleanup|Concurrent|CleanupWorktreesPreservingBranches|RecoverBranchStatus|Recreate_RecoveryHead' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'Cutover|ArchivedBranch|Worktree|RecoveryHead|Compaction' -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'StorageMaintenance|ArchivedBranch|Worktree' -count=1)
(cd apps/backend && go test ./internal/task/service -run 'BranchRecovery|TaskEnvironment|Cleanup|Handoff' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && golangci-lint run --new-from-rev=origin/main ./...)
make -C apps/backend build
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check origin/main...HEAD
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/backendapp/`
- `apps/backend/internal/orchestrator/executor/`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/task/service/`
- `apps/backend/internal/worktree/`
- `docs/decisions/2026-08-30-compact-integrated-managed-branches.md`
- `docs/public/`
- `docs/specs/tasks/requirements/runtime-cleanup.md`
- `docs/specs/tasks/system-design/runtime-cleanup.md`

## Dependencies

None.

## Risks

- A mistake in ownership, ancestry, liveness, or recovery ordering can delete or
  strand local-only work; all gates and mutations must fail closed.

## Parallelism

`sequential`

## Inputs

- REQ-TASKS-RUNTIME-CLEANUP-001 and AC-TASKS-RUNTIME-CLEANUP-001.18 through
  AC-TASKS-RUNTIME-CLEANUP-001.22.
- Task runtime-cleanup system design and the managed-branch compaction ADR.
- Existing worktree manager locking, task cleanup, and storage-maintenance
  patterns.

## Results

- Added conservative ownership and integration metadata, exact recovery refs,
  expected-head local deletion, bounded receipts/metrics, and archived storage
  maintenance through the shared worktree-manager policy.
- Wired every named terminal cleanup path and exact-head unarchive/recreate
  recovery while retaining legacy, external, shared, protected, live,
  ambiguous, and unpublished branches.
- Corrected recovery finalization so an interrupted pre-delete recovery clears
  after confirming the exact local head, and so the recovery ref is removed
  before its database metadata. Both interruption regressions failed before the
  fix and pass afterward.
- All verification commands in this work order passed. Current-main full lint
  has 35 baseline findings outside the PR diff; differential lint passed with
  zero issues.
