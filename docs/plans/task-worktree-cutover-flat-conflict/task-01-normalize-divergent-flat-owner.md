---
id: "01-normalize-divergent-flat-owner"
title: "Normalize divergent flat worktree owner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/session-delete-resource-cleanup.md"
---

# Task 01: Normalize Divergent Flat Worktree Owner

## Acceptance

- A non-deleted canonical row wins when deprecated flat fields name another
  worktree for the same normalized task, repository, and empty branch slot,
  including when the canonical row was re-homed from a collapsed environment.
- The migration logs the demoted flat identity only after commit. It does not
  change the directory, Git registration, or branch.
- A canonical row in another task, repository, or slot does not suppress the
  flat worktree.
- SQLite and PostgreSQL retain the existing rollback, replay, and fail-closed
  behavior for unrelated conflicts.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover_(CanonicalRepoSupersedesDivergentFlatOwner|PreservesDivergentFlatOutsideCanonicalSlot|DuplicateCanonicalRowsRetainSurvivingEnvironmentPrecedence|RehomedCanonicalRowSupersedesSurvivingFlatOwner|DoesNotLogRolledBackFlatDemotion)|TestCutoverPostgres_CanonicalRepoSupersedesDivergentFlatOwner' -count=1
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1
cd apps/backend && go test ./internal/task/repository/sqlite -count=1
```

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_targets.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_flat_precedence_test.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_postgres_test.go`
- `docs/specs/tasks/requirements/session-delete-resource-cleanup.md`
- `docs/plans/task-worktree-cutover-flat-conflict/plan.md`
- `docs/plans/task-worktree-cutover-flat-conflict/task-01-normalize-divergent-flat-owner.md`

## Dependencies

None.

## Parallelism

Sequential. The source classifier, inventory, log, and migration tests share
one persistence boundary.

## Inputs

- The schema-normalization, failure-mode, and scenario sections of the repair
  spec.
- The root-cause and canonical-precedence sections of `plan.md`.
- Existing flat metadata precedence and slot-election tests.
- Existing post-commit demotion log behavior.

## Output Contract

Report the source rule, changed files, and exact test results. Report all
temporary-file cleanup. Update this task and `plan.md` in the same conversation.

## Results

- Canonical repository rows now supersede divergent deprecated flat worktree
  fields for the same normalized task, repository, and empty branch slot,
  including canonical rows re-homed from collapsed environments.
- Demoted flat worktrees are excluded from normalized inventory validation and
  are reported through the existing post-commit demotion warning with their
  environment, repository, identity, path, branch, and canonical winner.
- A canonical row in another task, repository, or branch slot remains
  independent. Empty flat metadata does not trigger this demotion rule.
- Changed files: `worktree_ownership_normalize.go`,
  `worktree_ownership_targets.go`, `worktree_ownership_migration.go`,
  `worktree_ownership_flat_precedence_test.go`,
  `worktree_ownership_postgres_test.go`, and the repair spec/plan files.
- `cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover_(CanonicalRepoSupersedesDivergentFlatOwner|PreservesDivergentFlatOutsideCanonicalSlot|DuplicateCanonicalRowsRetainSurvivingEnvironmentPrecedence|RehomedCanonicalRowSupersedesSurvivingFlatOwner|DoesNotLogRolledBackFlatDemotion)|TestCutoverPostgres_CanonicalRepoSupersedesDivergentFlatOwner' -count=1` — 7 SQLite tests passed; the PostgreSQL test is environment-gated and skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- `cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1` — 54 tests passed.
- `cd apps/backend && go test ./internal/task/repository/sqlite -count=1` — 421 tests passed.
- No throwaway reproduction files remain. Test databases use `t.TempDir()` and
  are cleaned up by the test framework.
