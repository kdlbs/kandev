---
id: "01-cutover-repair"
title: "Repair task-worktree cutover precedence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/session-delete-resource-cleanup/spec.md"
---

# Task 01: Repair task-worktree cutover precedence

## Acceptance

- Legacy deleted/terminal session history cannot block an upgrade when a
  canonical `task_environment_repos` owner exists.
- Canonical repository path/branch metadata wins over stale flat environment
  metadata for the same physical worktree identity.
- Unresolved conflicts for non-terminal/live ownership still fail closed and
  roll back the legacy schema and data.
- An unexpected `CREATED` session-worktree reference without a canonical
  repository row is preserved for backfill, not silently discarded.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestCutover' -count=1
make -C apps/backend test
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_targets.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration_test.go`

## Dependencies

None.

## Parallelism

Sequential. The migration and its tests share schema and normalization state.

## Inputs

- Existing `TestCutover_ConflictingWorktreesFailClosed`.
- The affected database pattern: 20 conflicts, 554 deleted legacy session
  rows, 96 active rows, and canonical repository rows already present.
- [Task-owned worktree lifetime decision](../../decisions/2026-08-08-task-owned-worktree-lifetime.md).

## Output contract

Report the source/test diff, exact test commands and results, any unresolved
terminal-state assumptions, and synchronized task/plan status.

## Results

- Added canonical-repository precedence for stale flat metadata.
- Historical deleted/terminal session references no longer override canonical
  ownership; live conflicts still fail closed.
- Added terminal-state coverage for `COMPLETED`, `FAILED`, and `CANCELLED`,
  plus a `CREATED` reference without a canonical repository owner.
- Source/test diff is limited to cutover precedence, historical-row caching,
  and migration regression fixtures; no filesystem or Git state is changed.
- Terminal-state assumption: only `COMPLETED`, `FAILED`, and `CANCELLED` are
  historical; `STARTING`, `RUNNING`, `IDLE`, `WAITING_FOR_INPUT`, and
  unexpected `CREATED` rows remain eligible for ownership resolution.
- `GOCACHE=/tmp/kandev-go-cache go test ./internal/task/repository/sqlite -run
  'TestCutover_(CanonicalRepoWinsOverStaleFlatMetadata|IgnoresDeletedHistoricalSessionConflict)' -count=1` — passed.
- `GOCACHE=/tmp/kandev-go-cache go test ./internal/task/repository/sqlite -run
  'TestCutover' -count=1` — passed.
- `GOCACHE=/tmp/kandev-go-cache go test ./internal/task/repository/sqlite -count=1` — passed.
- `make -C apps/backend test` — passed.
- Plan status is `implemented`; Task 01 and Task 02 statuses are `done` and
  synchronized with the recorded implementation and verification.
