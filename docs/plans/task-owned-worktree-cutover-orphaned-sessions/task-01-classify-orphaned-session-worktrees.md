---
id: "01-classify-orphaned-session-worktrees"
title: "Classify orphaned session worktrees"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/session-delete-resource-cleanup.md"
---

# Task 01: Classify Orphaned Session Worktrees

## Acceptance

- A missing-session row is ignored only when it is deleted history or its exact
  non-empty physical worktree ID already has a non-deleted normalized target
  built from a canonical or surviving flat owner.
- A recoverable orphan contributes no ownership or stale metadata, while an
  active orphan without a higher-precedence owner still fails closed with the
  current diagnostic and complete rollback.
- SQLite and PostgreSQL cutovers use the same classification, and existing
  cutover compatibility, precedence, inventory, rollback, and replay behavior
  remains valid.

## Verification

```bash
make -C apps/backend test GOFLAGS="-v -run=TestCutover.*Orphan.* -count=1"
make -C apps/backend test GOFLAGS="-v -run=TestCutover -count=1"
make -C apps/backend test GOFLAGS="-v -count=1"
```

The first focused command is the RED/GREEN gate: before changing production
code, add the orphan fixtures and confirm the recoverable cases fail with
`references missing session` while the irreconcilable cases already fail for
that expected reason.

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_orphan_migration_test.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_postgres_test.go`
- `docs/specs/tasks/requirements/session-delete-resource-cleanup.md`
- `docs/plans/task-owned-worktree-cutover-orphaned-sessions/plan.md`
- `docs/plans/task-owned-worktree-cutover-orphaned-sessions/task-01-classify-orphaned-session-worktrees.md`

## Dependencies

None.

## Parallelism

Sequential. The production classifier and both database-engine fixtures share
one migration and persistence boundary.

## Inputs

- Schema normalization, failure modes, and migration scenarios in the linked
  repair spec.
- Root cause and classification rule in `plan.md`.
- Existing `loadLegacySessionWorktrees`, `pickSurvivingEnvironments`,
  `isSupersededSessionWorktree`, `isLegacyDeletedWorktree`,
  authoritative-source, inventory, and rollback behavior.
- Read-only affected-database evidence: the missing-session row repeats the
  exact non-empty worktree ID owned by the surviving flat environment.

## Output Contract

Report the classification change, files changed, exact RED/GREEN and broader
test commands with results/counts, assumptions about deleted sources, and any
temporary reproduction cleanup. Update this task to `done`, synchronize the
plan checkbox/status and verification results, and run `git diff --check` in
the same conversation.

## Results

- **RED:** `make -C apps/backend test
  GOFLAGS="-v -run=TestCutover.*Orphan.* -count=1"` failed for the recoverable
  flat, canonical, and deleted-history cases with `references missing session`.
  The unique active and deleted-canonical controls already passed by asserting
  the expected fail-closed behavior.
- **Implementation:** missing-session rows are retained in a separate orphan
  inventory until surviving environments and canonical rows are known. Deleted
  orphans and exact non-empty physical IDs represented by a non-deleted
  normalized target contribute no ownership or metadata. All other orphans
  retain the original missing-session conflict.
- **GREEN:** `go test -tags fts5 ./internal/task/repository/sqlite -run
  'TestCutover.*Orphan.*' -count=1 -v` passed four SQLite top-level tests and
  three fail-closed subtests; the PostgreSQL test skipped because
  `KANDEV_TEST_POSTGRES_DSN` is not set.
- **Focused gate:** `make -C apps/backend test
  GOFLAGS="-v -run=TestCutover.*Orphan.* -count=1"` passed.
- **Cutover gate:** `make -C apps/backend test
  GOFLAGS="-v -run=TestCutover -count=1"` passed; 44 top-level cutover tests
  are discovered, with environment-gated PostgreSQL cases skipped locally.
- **Backend gate:** `make -C apps/backend test GOFLAGS="-v -count=1"` passed.
- **Affected database:** a temporary `-tags fts5` repository test initialized
  an SQLite backup of the reported 1.4 GB database. It removed the legacy table
  and retained the reported worktree ID, path, and branch. The temporary test
  and database copy were removed; the live database was untouched.
- **Files changed:**
  `worktree_ownership_normalize.go`,
  `worktree_ownership_orphan_migration_test.go`,
  `worktree_ownership_postgres_test.go`, the repair spec, and this plan/task
  package.
- **Hygiene:** `gofmt` and `git diff --check` passed. No temporary reproduction
  artifacts remain. External side effects: none beyond the later authorized
  branch push and PR workflow.
- **PR review remediation:** the raw-flat-plus-deleted-canonical regression
  failed before the follow-up with a nil cutover error, proving an active orphan
  could be suppressed without a persisted active owner. The classifier now
  inspects non-deleted normalized targets. Empty worktree IDs and PostgreSQL
  unique active orphans also have explicit fail-closed coverage. The focused
  orphan and complete cutover Make gates passed after the change.
