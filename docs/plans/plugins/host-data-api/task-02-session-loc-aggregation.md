---
id: "02-session-loc-aggregation"
title: "Per-session LOC aggregation (committed + peak-pending)"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../../specs/plugins/spec.md"
adr: "../../../decisions/0043-plugin-host-data-api.md"
---

# Task 02: Per-session LOC aggregation (committed + peak-pending)

Add the one net-new read-only query the Host data API needs: per-session code
stats. No existing method computes this. `internal/analytics/repository/sqlite/
stats.go` aggregates git stats at workspace and per-repository granularity only,
and the peak-pending snapshot aggregation exists nowhere in-tree.

## Scope
Add a repository method (recommended home: `internal/analytics/repository/sqlite/`,
alongside `GetGitStats`/`GetRepositoryStats`) returning, per session id:
- `lines_added_committed` = `SUM(insertions)` over `task_session_commits`.
- `lines_deleted_committed` = `SUM(deletions)`.
- `lines_added_peak_pending` = `MAX` over each snapshot's
  `SUM(json_extract(json_each(files).value,'$.additions'))` from
  `task_session_git_snapshots` (peak, not latest — the latest is usually a clean
  tree).
- `lines_deleted_peak_pending` = the deletions equivalent.

Mirror the SQL in the agent-stats plugin
(`/home/jcfs/.kandev/tasks/tokens-per-task-loc_9tk/kandev-plugin-agent-stats/server/stats.go`,
`sessionsQuery`). Use `internal/db/dialect` helpers (`JSONExtract`, etc.) so the
query is portable to Postgres. Support filtering by a session-id set and/or
workspace, plus a limit/offset (or cursor bound) for pagination. Expose it via a
service method so the Host handler (task-04) calls the service, not the repository.

## Acceptance
- New repository + service method returns per-session committed sums and the
  peak-pending `MAX` semantics, filterable by session ids / workspace.
- Peak-pending uses the peak snapshot, not the latest, and does not double-count
  committed work (matches `effectiveLines` intent in the source plugin).
- Table-driven SQLite test with commits + multiple snapshots asserts both.

## Verification
- `cd apps/backend && go test ./internal/analytics/...`

## Files likely touched
- `apps/backend/internal/analytics/repository/sqlite/stats.go`
- `apps/backend/internal/analytics/models/` (new `SessionCodeStats` model)
- analytics service file exposing the method
- `apps/backend/internal/analytics/repository/sqlite/*_test.go`

## Inputs
- Source SQL: agent-stats `server/stats.go` `sessionsQuery` (committed sums +
  snapshot peak).
- Patterns: `GetGitStats` / `buildRepositoryStatsQuery` in
  `internal/analytics/repository/sqlite/stats.go`; `dialect` helpers.
- Spec: "Host data API" → `ListSessionCodeStats` row.

## Dependencies
None. Independent of the proto task (pure Go/SQL).

## Output contract
Summary, method signatures added, chosen home (analytics vs task repo), test
result, portability notes (SQLite/Postgres), and status update here + in `plan.md`.
</content>
