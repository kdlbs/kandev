---
id: "01-absent-canonical-environment"
title: "Tolerate an absent canonical environment on archive"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/detached-workspace-continuity.md"
---

# Task 01: Tolerate an absent canonical environment on archive

Satisfies `AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.6` and
`AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.7`.

## Root cause

`transferWorkspaceGroupEnvironmentOwnership`
(`apps/backend/internal/task/service/handoff_cascade.go`) resolves
`group.MaterializedEnvironmentID` before it checks whether the environment owner
is departing or whether survivors exist. A missing environment row fails on the
`err != nil` branch; the `env == nil` branch is fatal too. Nothing repairs the
group reference, so the failure repeats forever and every archive route
(MCP `archive_task_kandev`, `POST /api/v1/tasks/:id/archive`, the UI button)
fails identically through `HandoffService.ArchiveTaskTree`.

## Acceptance

- Archiving a task whose workspace group names a positively absent canonical
  environment succeeds and sets `archived_at`. Positive absence is the
  repository's typed not-found sentinel, or a nil row returned with a nil error.
- The skipped transfer records no transfer, increments no environment or group
  ownership generation, and changes no `owner_task_id`.
- Archiving still fails, without changing workspace-group or environment
  ownership, when the environment lookup fails for any other reason. A generic
  DB error must not be read as absence.
- Behavior is unchanged when the environment resolves: a departing owner with
  surviving members still transfers stewardship exactly as before.

## Regression test that must fail first

Add to `apps/backend/internal/task/service/handoff_cascade_test.go`:

1. `TestArchiveTaskTreeSucceedsWhenCanonicalEnvironmentAbsent` — group names an
   environment ID with no row; archive succeeds; `archived_at` set; no ownership
   generation moved. This fails before the fix with the wrapped
   `task environment not found` error.
2. `TestArchiveTaskTreeFailsWhenEnvironmentLookupErrors` — the environment
   repository returns a non-sentinel error; archive fails and ownership is
   unchanged. Guards against a blanket `err != nil` no-op.

Cover both shapes of the affected inventory in a mixed-state case, since an
empty `materialized_path` is not universal: one group with an empty
`materialized_path` and one with a non-empty path plus recorded worktree IDs.
Both must archive; neither may be treated as authority to delete a resource.

## Verification

```bash
cd apps/backend && go test ./internal/task/service/... -run 'ArchiveTaskTree' -count=1
```

```bash
cd apps/backend && make lint
```

Note: `make test` on this host fails on roughly a dozen unrelated backend
packages because of the macOS `/var` symlink. Compare against a base-commit
control before attributing any failure to this change.

## Files likely touched

- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/handoff_cascade_test.go`

## Completion report

Implemented as scoped, with one adjustment surfaced during PR review: the
absent-environment tolerance is threaded through as an explicit
`tolerateAbsentEnvironment bool` parameter on
`transferSharedWorkspaceEnvironmentOwnership` /
`transferWorkspaceGroupEnvironmentOwnership`, rather than applying
unconditionally. `ArchiveTaskTree` passes `true`; `DeleteTaskTree` (via
`prepareDeleteTaskTree`) passes `false` and keeps the pre-fix fail-closed
behavior, because delete is destructive and this plan only evaluated archive.
Without this, the same code change that unblocks archive would have silently
also unblocked deleting a task whose group names an absent environment — never
evaluated against ADR-0009's fail-closed requirement for destructive paths.

Files touched:
- `apps/backend/internal/task/service/handoff_cascade.go` — the fix described
  above, plus threading the new parameter through both call sites.
- `apps/backend/internal/task/service/handoff_cascade_absent_environment_test.go`
  (new file; `handoff_cascade_test.go` is already at the 800-line revive
  limit) — `TestArchiveTaskTree_SucceedsWhenCanonicalEnvironmentAbsent`
  (table-driven: typed sentinel with no worktrees, typed sentinel with a
  recorded `materialized_path`, nil row with nil error),
  `TestArchiveTaskTree_FailsWhenEnvironmentLookupErrors` (a generic lookup
  error, e.g. `database is locked`, must still fail closed), and
  `TestDeleteTaskTree_FailsWhenCanonicalEnvironmentAbsent` (delete must still
  fail closed on the same two absence shapes archive now tolerates).
- `apps/backend/internal/task/service/handoff_workspace_test.go` — shared
  fixture support (`taskEnvironmentErrs`) for injecting environment lookup
  results.

Verification (apps/backend, 2026-09-14):

```
go test ./internal/task/service/... -run 'ArchiveTaskTree|DeleteTaskTree' -count=1
ok  	github.com/kandev/kandev/internal/task/service	0.817s
```

```
golangci-lint run ./internal/task/service/... --new-from-rev="e5987421107e46237f74a58a1e2fa8bad0d7e773" --timeout=5m
0 issues.
```

A full `go test ./internal/task/service/...` run shows 11 pre-existing
failures on this macOS host (worktree/discovery-root clusters, e.g.
`TestTaskLifecycleCleanup_MissingWorktree`,
`TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity`,
`TestDesktopDiscoveryRootPersistsAcrossServiceRestart`), unrelated to this
change and reproducing identically against the base commit.
