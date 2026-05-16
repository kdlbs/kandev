---
status: shipped
created: 2026-05-16
owner: cfl
---

# Office GC Worktree Safety

## Why

`internal/office/infra/gc.GarbageCollector.sweepWorktrees` deletes any directory under `~/.kandev/tasks/` whose name doesn't match a row in `tasks` keyed by `id`. But directory names are produced by `worktree.SemanticWorktreeName(taskTitle, suffix)` (slug like `locstat-github-actio_5gz`), not task UUIDs - so every directory is classified as orphaned and `os.RemoveAll`'d on the first sweep. The sweep runs immediately at process start and then every 3h.

A real incident: on a work machine with an existing prod kandev DB (on `main`), checking out `feature/orchestrate` and launching the backend wiped 307 worktrees - every in-progress task's working directory was destroyed.

Three compounding defects in `shouldDeleteWorktree`:

1. **Wrong key.** Directory name (slug + suffix) compared against `tasks.id` PK. Lookup never matches.
2. **No cross-reference.** The authoritative inventory of live worktrees is `task_session_worktrees.worktree_path`. GC never consults it.
3. **Fail-open delete.** `info, err := repo.GetTaskBasicInfo(...); if err != nil || info == nil { return true }`. An error, a missing row, or any other "unknown" condition triggers deletion. The destructive action should require a positive signal of orphaning, not its absence.

Compounding context:

- `~/.kandev/tasks/` is shared by regular and office tasks (`worktree.Config.TasksBasePath` defaults to `<data dir>/tasks` for everyone). Blast radius is not scoped to office.
- The GC is started unconditionally once office services are wired (`cmd/kandev/main.go:958-964`); not gated on the office feature flag.
- Existing `gc_test.go` uses mock repos that resolve `dirName` to a synthetic task, so the schema mismatch was never exercised in tests.

The container sweep (`shouldRemoveContainer`) has the same fail-open anti-pattern (`gc.go:220-227`: DB error -> "orphan" -> remove). Blast radius is smaller (kandev-labeled containers only, and the post-error path only fires if the task ID looked up fails) but the pattern is identical and should be corrected in the same change.

## What

### Behavior changes

1. **Stop-gap commit (lands first).** `sweepWorktrees` becomes a no-op until the redesigned algorithm and tests land. Container sweep continues. This is a small, surgical change that can ship in a patch release.

2. **Redesigned worktree sweep.** The sweep operates against the authoritative inventory in `task_session_worktrees`:

   - At the start of each sweep, fetch all live worktree paths via a new `WorktreeInventory.ListActiveWorktreePaths(ctx)` method. The query is: `SELECT worktree_path FROM task_session_worktrees WHERE status = 'active' AND deleted_at IS NULL AND worktree_path <> ''`.
   - Normalize every path with `filepath.Clean` and resolve to absolute form. Build a set `liveSet`. Also build `ancestorSet`: the set of every ancestor directory of every live path, up to (but not including) `worktreeBase`. The ancestor set covers the multi-repo task layout `{base}/{taskDir}/{repoName}` - a sweep at `{base}` level must not delete `{taskDir}` while `{taskDir}/{repoName}` is live.
   - For each `entry` under `worktreeBase`:
     - Compute `absPath = filepath.Join(worktreeBase, entry.Name())`.
     - Keep if `absPath` is in `liveSet` or in `ancestorSet`.
     - Stat the directory. Keep if `now - mtime < gracePeriod` (24h). Covers the race where the directory is created on disk before its DB row is inserted, and operator-created scratch dirs.
     - Keep if stat fails. Fail-closed.
     - Otherwise delete.
   - If `ListActiveWorktreePaths` returns an error, log a warning and skip the entire worktree sweep for this tick. Never proceed with deletions on a partial or empty inventory.

3. **Container sweep fix.** Same fail-open bug, same fix: `shouldRemoveContainer` currently treats `GetTaskExecutionFields` error as "orphan, delete." Change to: log the error, keep the container. Only delete on a positive signal (row exists AND state is terminal AND container state is non-running, as today). Add a test covering "DB error keeps container."

### New repo method

`internal/worktree/store.go`:

```go
// ListActiveWorktreePaths returns all non-empty worktree paths
// for sessions with active, non-deleted worktrees.
func (s *SQLiteStore) ListActiveWorktreePaths(ctx context.Context) ([]string, error)
```

Indexed by the existing `idx_task_session_worktrees_status`. Returns a flat `[]string`.

### Wiring

- New interface inside `internal/office/infra`:

  ```go
  type WorktreeInventory interface {
      ListActiveWorktreePaths(ctx context.Context) ([]string, error)
  }
  ```

- `NewGarbageCollector` adds a `WorktreeInventory` parameter (or accepts it via a typed option struct to avoid argument creep).
- `cmd/kandev/main.go` passes the existing `*worktree.SQLiteStore` (already constructed elsewhere; check call path during planning).
- If `WorktreeInventory` is nil, worktree sweep is skipped entirely (defensive default).

### Tests (the part that would have caught the original bug)

`gc_test.go` rewrites:

- `TestSweepWorktrees_KeepsActiveTaskDirs` - seed `task_session_worktrees` row with a real path, create the directory on a `t.TempDir()` base, sweep, assert directory exists.
- `TestSweepWorktrees_DeletesOrphanOlderThanGracePeriod` - directory exists, no DB row, mtime backdated past 24h via `os.Chtimes` - deleted.
- `TestSweepWorktrees_KeepsFreshOrphan` - directory exists, no DB row, mtime recent - kept.
- `TestSweepWorktrees_KeepsParentOfLivePath` - live path is `{base}/task1/repo`, sweep sees `task1` at `{base}` level - kept (ancestor check).
- `TestSweepWorktrees_FailClosedOnInventoryError` - inventory returns error - no deletions, no panic, logged warning.
- `TestSweepWorktrees_PathNormalization` - live path stored with trailing slash, disk has same dir without - matches.
- `TestSweepContainers_KeepsContainerOnDBError` - container exists, repo returns error, container is not removed.

The existing mock-based test that exercises `shouldDeleteWorktree(dirName) -> deletion` is removed; it tested wrong behavior.

### ADR

Short ADR under `docs/decisions/` titled "Fail-closed GC semantics." Records:

- The incident (307 worktrees lost on prod-DB launch of `feature/orchestrate`).
- The decision: GC code never deletes on uncertainty. The authoritative source must return a positive "this is not tracked" signal.
- Project-wide rule: any new GC / cleanup code follows the same pattern. Inventory query first, fail-closed on error, grace period for races.

## Out of scope

- Office feature-flag gating of GC startup. Worth doing, but orthogonal - the algorithm should be safe whether or not the GC runs.
- Dry-run flag / phased rollout. Ship the corrected algorithm directly (user decision).
- Tombstone marker files in each worktree directory. Inventory + grace period is sufficient.
- Worktree creation flow changes. The inventory already exists and is populated; nothing to change there.
- Refactoring the container sweep beyond the fail-open fix. Same loop, same labels, same terminal-state policy.

## Success criteria

- Launching kandev against a DB with active task worktrees never deletes them, regardless of which branch the binary was built from.
- A directory with no matching DB row and `mtime > 24h` is deleted on the next sweep.
- Any error from the worktree inventory aborts the sweep with a warning log; no deletions occur.
- A container whose task lookup errors is kept, not removed.
- Tests cover all six worktree scenarios above plus the container error case, and run in `make -C apps/backend test`.
