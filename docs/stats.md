# Statistics System

## Overview

The stats system provides workspace-level analytics about tasks, agent sessions, repository activity, and git commits. It computes everything on-demand from existing tables — there are no dedicated stats tables.

**Route**: `/stats` (with `?range=week|month|all`)

## Architecture

```
Frontend                        Backend                         Database
───────                         ───────                         ────────
/stats (server component)       GET /api/v1/workspaces/:id/     SQLite
  └─ fetchStats()                   stats?range=week
      └─ stats-page-client.tsx  stats_handlers.go               Source tables:
         ├─ Overview cards        └─ httpGetStats()              ├─ tasks
         ├─ Heatmap                   ├─ GetGlobalStats()        ├─ task_sessions
         ├─ Completed chart           ├─ GetTaskStats()          ├─ task_session_turns
         ├─ Agent usage               ├─ GetDailyActivity()      ├─ task_session_messages
         ├─ Repo stats               ├─ GetCompletedTaskActivity()├─ task_session_commits
         └─ Workload                  ├─ GetAgentUsage()         ├─ task_repositories
                                      ├─ GetRepositoryStats()    ├─ repositories
                                      └─ GetGitStats()           └─ workflow_steps
```

## Data Flow

### Write path (automatic during task execution)

| Event | Table | Key columns |
|-------|-------|-------------|
| Session start | `task_sessions` | task_id, agent_profile_id, started_at |
| Turn start | `task_session_turns` | task_session_id, started_at |
| Message sent | `task_session_messages` | task_session_id, author_type, type |
| Git commit | `task_session_commits` | session_id, files_changed, insertions, deletions |
| Turn end | `task_session_turns` | completed_at (UPDATE) |
| Session end | `task_sessions` | completed_at, state (UPDATE) |

Write code lives in `apps/backend/internal/task/repository/sqlite/` (session.go, message.go, git_snapshots.go).

### Read path (stats page load)

The handler at `apps/backend/internal/analytics/handlers/stats_handlers.go` calls 7 repository methods sequentially, each running a SQL aggregation query against the source tables. Results are assembled into a single `StatsResponse` JSON.

## Key Files

| File | Purpose |
|------|---------|
| `apps/backend/internal/analytics/handlers/stats_handlers.go` | HTTP handler, DTO conversion |
| `apps/backend/internal/analytics/repository/sqlite/stats.go` | All SQL queries |
| `apps/backend/internal/analytics/repository/sqlite/repository.go` | Init + auto-created indexes |
| `apps/backend/internal/analytics/models/models.go` | Go data models |
| `apps/web/app/stats/page.tsx` | Server component (data fetching) |
| `apps/web/app/stats/stats-page-client.tsx` | Client component (all UI) |
| `apps/web/app/stats/loading.tsx` | Loading skeleton |
| `apps/web/lib/api/domains/stats-api.ts` | Frontend API client |

## Metrics

### How key metrics are computed

| Metric | Method |
|--------|--------|
| Completed tasks | Tasks whose workflow step has `step_type = 'done'` |
| In-progress tasks | Tasks with `state = 'IN_PROGRESS'` |
| Duration | `SUM((julianday(completed_at) - julianday(started_at)) * 86400000)` on turns |
| Tool calls | Messages with `type LIKE 'tool_%'` |
| User messages | Messages with `author_type = 'user'` |
| Agent name/model | `json_extract()` from `agent_profile_snapshot` on sessions |
| Git stats | Aggregated from `task_session_commits` (only in-session commits) |

### Time ranges

| Range | Backend behavior |
|-------|-----------------|
| `week` | `start = now - 7 days`, daily activity = 7 days |
| `month` | `start = now - 30 days`, daily activity = 30 days |
| `all` | `start = nil` (no filter), daily activity = 365 days |

Range is parsed in `parseStatsRange()`. The `start` value is passed to each query as `WHERE (? IS NULL OR timestamp >= ?)`.

## Performance

### Indexes

Performance indexes are **automatically created** on startup in `repository.go` via `ensureStatsIndexes()`. Uses `CREATE INDEX IF NOT EXISTS` so it's idempotent.

Key indexes: tasks(workspace_id, created_at), task_sessions(task_id, started_at), task_session_turns(task_session_id, started_at, completed_at), task_session_messages(task_session_id, author_type, type), task_session_commits(session_id, committed_at).

### Query planner statistics

`PRAGMA optimize` runs automatically on database connection close (see `persistence/provider.go`). This keeps SQLite's query planner statistics up to date by only re-analyzing tables whose stats are stale. No manual `ANALYZE` is needed.

### Query patterns

- Separate subqueries to avoid row multiplication from JOINs
- `COUNT(DISTINCT id)` to prevent duplicate counting across LEFT JOINs
- `COALESCE(..., 0)` for NULL safety
- Recursive CTE for date series (heatmap/charts fill in zero-activity days)

## Known Limitations

1. **No caching** — every page load runs all 7 queries
2. **No real-time updates** — single fetch on page load, no WebSocket/polling
3. **Git tracking gaps** — only commits made during task sessions are tracked
4. **Duration precision** — `julianday()` returns float, cast to int64 (minor precision loss)
5. **Incomplete turns** — turns with NULL `completed_at` count in totals but contribute 0ms to duration
6. **"Month" = 30 days** — not a calendar month
7. **Sequential queries** — the 7 repo calls in the handler run sequentially, not in parallel
