---
id: "03-resume-continuity"
title: "Prove resume continuity"
status: done
wave: 3
depends_on:
  - "02-guarded-snapshot-retry"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.5
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 03: Prove resume continuity

## Summary

Prove the corrected repair through the existing resume flow. Add public guidance only after its behavior passes integration checks.

## In scope

- Add `TestPermissionRecoveryResumeIntegration` in a new executor test file with real manager admission, real local Git, SQLite inventory, and a durable claim.
- Add `TestRecoverSessionPermissionRetryRetiresMatchingError` in a new orchestrator test file. Record external provider startup rather than invoking an agent.
- Cover a CANCELLED session, ordinary resume refusal, a current explicit action, stale stamp refusal, reload of the blocked record, and provider resume-token preservation.
- Prove one unaffected sibling stays unchanged and an invalid sibling prevents all new mutation. Retire only the matched error after relaunch.
- Prove identity verification failures prevent canonical publication and provider startup.
- Update `docs/public/git-operations.md` with supported permissions, manual retry after upgrade, retained copies, staging limits, and refusal for content or ownership drift.
- Keep existing desktop and phone controls and localized copy. Use backend end-to-end evidence because rendered UI does not change.
- Record exact results and synchronize only this package. Validate traceability and public documentation.

## Out of scope

- New UI controls, layouts, translation keys, or Playwright scenario changes.
- Repairing the original live task or seeding from its private files.
- Marking incomplete companion work orders complete.

## Acceptance

1. The service-to-manager fixture proves same-session/provider continuation and complete canonical publication. Ordinary and stale requests never start a runtime.
2. Matching-error retirement succeeds only after relaunch. A newer error and partial multi-slot failure remain visible, with all original copies retained.
3. Public guidance matches the verified behavior. Specification, diff, and PR documentation coverage gates pass.

## Verification

Run this block from the repository root after Red, Green, and Refactor.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run '^TestPermissionRecoveryResumeIntegration$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestRecoverSessionPermissionRetryRetiresMatchingError$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree ./internal/orchestrator/executor -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
# Run the documentation coverage preflight block in plan.md.
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_permission_recovery_integration_test.go (new)`
- `apps/backend/internal/orchestrator/session_permission_recovery_integration_test.go (new)`
- `docs/public/git-operations.md`
- `docs/plans/workspace-recovery-permissions/plan.md`

## Dependencies

02-guarded-snapshot-retry.

## Risks

- A success-only admission callback cannot prove the filesystem repair.
- Do not erase a successor error after an older repair finishes.
- Fixture runtime and sessions must release their claim through the normal lifecycle.

## Parallelism

`sequential`

## Inputs

- Existing executor worktree integration tests and orchestrator relocation error-projection tests.
- `RecoverSessionWithOptions`, `PreflightSessionWorktreeRecovery`, and stamp-guarded error retirement.
- Public Git guide: managed clone relocation and metadata recovery.
- Plan end-to-end evidence and companion package status.

## Results

Implemented both resume-continuity regressions and updated the public Git recovery
guide. The executor integration uses a real SQLite inventory, local Git clones,
managed worktree state, and a durable recovery claim. It proves that ordinary
resume does not launch, the authorized retry publishes the repaired clone, and
the original provider token continues. The orchestrator test proves stale-stamp
refusal and retires only the matching recovery error after the provider starts.
The worktree regressions also prove that an unaffected healthy sibling retains
its path, Git state, and untracked content, while a separate invalid sibling
blocks every retry mutation in a mixed inventory.

- `go test -trimpath -tags fts5 ./internal/orchestrator/executor -run '^TestPermissionRecoveryResumeIntegration$' -count=1 -v`: passed.
- `go test -trimpath -tags fts5 ./internal/orchestrator/executor -run '^TestPermissionRecoveryIdentityRefusalPreventsProviderStartup$' -count=1 -v`: passed.
- `go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestRecoverSessionPermissionRetryRetiresMatchingError$' -count=1 -v`: passed.
- `go test -trimpath -tags fts5 ./internal/worktree ./internal/system/storage/workspaces ./internal/orchestrator ./internal/orchestrator/executor -count=1`: passed.
- Targeted recovery regressions under `go test -trimpath -tags fts5 -race ./internal/worktree`: passed.
- `make -C apps/backend build`: passed after the final refactor.
- `golangci-lint run ./internal/worktree ./internal/system/storage/workspaces ./internal/orchestrator ./internal/orchestrator/executor --timeout=5m`: passed with zero issues.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed; `node scripts/validate-public-docs.mjs`: 47 published pages validated.
- `python3 scripts/list-docs.py validate`: 349 decisions and 1,337 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed; `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight, `gofmt -l` on changed Go files, and `git diff --check`: passed.
- Native Windows execution was not available locally. The worktree-package cross-build is blocked by existing undefined `sqlite3.Error` and `sqlite3.ErrConstraintUnique` references in `internal/task/repository/sqlite/repository_branch_policy_admission.go`. The Windows backend job now runs `./internal/worktree` with `-tags fts5` so the native runner covers it.
