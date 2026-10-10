---
id: "01-private-artifacts"
title: "Own recovery artifacts outside working files"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-004
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.5
  - AC-TASKS-MANAGED-CLONE-RELOCATION-004.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-004.3
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
  - ../../specs/tasks/system-design/managed-clone-relocation-experience.md
---

# Task 01: Own recovery artifacts outside working files

## Summary

Create new clone-relocation artifacts in the private recovery namespace.
Retain legacy journal discovery and snapshots. Supply exact trusted exclusions
to workspace tree and search without deleting or renaming files.

## In scope

- Add the `task_environment_recovery_artifacts` schema and registry capability.
  Fence every registration with canonical ownership and operation proof.
- Extend new clone-relocation journals with version 2 layout and private paths.
  Persist intended paths before mutation. Preserve no-follow validation.
- Keep the current original archive convention and physical replacement paths.
- Keep version 1 journals and permission-only retries at their original paths.
  Register proven legacy ownership during authoritative reconciliation only.
- Pass exact exclusions to workspace tracker tree, filename search, and content
  search. Refresh trusted exclusions on inventory changes without runtime restart.
- Retain originals, failed snapshots, special modes, ignored files, and symlinks.

## Out of scope

Progress projection, display labels, rendered copy, artifact cleanup, and
changes to generic metadata-recovery eligibility or placement.

## Acceptance

1. A real-Git new relocation preserves full content and places artifacts outside
   the task root. Registry creation fails safely before transfer if unavailable.
2. Legacy complete, interrupted, and permission-only blocked operations retain
   their identity and copies. Substituted paths and stale authority cannot be adopted.
3. Tree and both searches exclude only registered artifacts. User lookalikes and
   uncertain entries remain visible. Existing canonical file APIs still work.

## TDD and regression evidence

Start with failing tests in `managed_clone_relocation_artifacts_test.go`:
`TestManagedCloneRecoveryPrivateArtifactsPreserveContent`,
`TestManagedCloneRecoveryLegacyJournalAndModeRetryContinuity`, and
`TestManagedCloneRecoveryArtifactRegistryRejectsSubstitution`.

Add registry migration/idempotence and stale-owner tests in the task repository.
Add `TestWorkspaceTreeRecoveryExclusionsKeepLookalikes` to the existing tree
suite and `TestWorkspaceSearchRecoveryExclusions` to a focused search test file.
Cover a multi-repository partial publication and changing source/snapshot path.
Preserve existing generic recovery and mode-retry fixtures as compatibility evidence.

## Verification

Run from the repository root. Existing package suites are required because the
new locator touches shared readers and tracker setup.

```bash
(cd apps/backend && go test ./internal/worktree ./internal/task/repository/sqlite ./internal/agentctl/server/process ./internal/agent/handlers -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Repository PostgreSQL tests require `KANDEV_TEST_POSTGRES_DSN`. Record a skip
explicitly when unavailable; do not count a skip as PostgreSQL evidence. Task 02
owns the final configured database concurrency gate.

## Files likely touched

- `apps/backend/internal/worktree/managed_clone_relocation.go`
- `apps/backend/internal/worktree/managed_clone_relocation_archive.go`
- `apps/backend/internal/worktree/recovery.go`
- `apps/backend/internal/worktree/recovery_admission.go`
- `apps/backend/internal/worktree/recovery_mode_retry.go`
- `apps/backend/internal/worktree/managed_clone_relocation_artifacts_test.go` (new)
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_recovery_artifacts.go` (new)
- Corresponding task repository interfaces and registry injection in backend composition.
- `apps/backend/internal/agent/handlers/workspace_file_handlers.go`
- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- Existing agentctl/runtime workspace setup types and tree/search tests.

## Dependencies

None. Read the existing relocation, convergence, and permission packages first.

## Risks

Legacy filenames are not ownership proof. Moving live locks breaks exclusion.
Generic snapshot validation must remain strict while clone layout changes.
Preserved copies must never enter ordinary temporary artifact cleanup.

## Parallelism

`sequential`

## Inputs

- [Plan evidence, compatibility matrix, and test mapping](plan.md)
- Design sections: Artifact layout and registry; Legacy compatibility; Files and workspace search.
- `managed_clone_relocation_recovery_test.go`, `recovery_review_test.go`, and
  existing mode-retry tests under `internal/worktree`.
- Existing environment claim and exact-slot publication repository patterns.

## Results

Implemented private version-2 artifact storage with a claim-fenced registry,
retained version-1 journal and permission-retry paths, exact live tree/search
exclusions, and no-follow private record reads. The real-Git content test
preserves ignored files, modes, symlinks, and dirty files while keeping new
artifacts outside the task root.

Verification passed: the required Go package gate for `internal/worktree`,
`internal/task/repository/sqlite`, `internal/agentctl/server/process`, and
`internal/agent/handlers`; `python3 scripts/list-docs.py validate` (351
decisions, 1,347 specifications); `python3 scripts/lint-spec-files.py --all`;
and `git diff --check`. The configured PostgreSQL gate was not run because
`KANDEV_TEST_POSTGRES_DSN` is unavailable; Task 02 owns that final gate.
