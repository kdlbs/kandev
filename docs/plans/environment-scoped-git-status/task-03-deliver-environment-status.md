---
id: "03-deliver-environment-status"
title: "Deliver environment status"
status: completed
wave: 3
depends_on:
  - "02-scope-snapshot-capture"
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9
system_design:
  - ../../specs/tasks/system-design/environment-owned-git-status.md
---

# Task 03: Deliver Environment Status

## Summary

Read current Git status by task environment for hydration and task summaries.
Put task environment identity in every live and persisted status payload.

## In scope

- Detect live executions across all sessions in an environment.
- Use live agentctl status when any environment execution is live.
- Do not use persisted fallback after a failed live query.
- Read persisted status by environment when no execution is live.
- Put `task_environment_id` in live and boot status payloads.
- Load task-card and status-summary Git observations by unique environment.
- Adapt delivery ancestry and sweep queries to environment ownership.
- Keep analytics reads explicitly provenance-based.
- Add sibling, shared-task, multi-repository, summary, and direct-reader tests.

## Out of scope

- Frontend payload consumption.
- Git polling inside agentctl.
- Changes to commit, reset, and branch-switch payload identity.

## Acceptance

- Sibling hydration emits one authoritative observation for each repository.
- Every status payload contains the environment that the status describes.
- A failed live query cannot emit an older persisted snapshot.
- Task summaries do not count one shared environment once per sibling session.
- Delivery reads survive capture-session deletion when the environment remains.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/backendapp ./internal/task/service ./internal/delivery ./internal/analytics/repository/sqlite -run 'Test.*(GitStatus|GitObservation|GitSnapshot|Delivery).*Environment' -count=1
```

## Files likely touched

- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/git_status_sources.go`
- `apps/backend/internal/backendapp/helpers_git_status_test.go`
- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/backendapp/gateway_git_observations_test.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild_test.go`
- `apps/backend/internal/delivery/observe.go`
- `apps/backend/internal/delivery/sweep.go`
- `apps/backend/internal/delivery/observe_test.go`
- `apps/backend/internal/delivery/sweep_test.go`
- `apps/backend/internal/analytics/repository/sqlite/stats.go`
- `apps/backend/internal/analytics/repository/sqlite/session_code_stats_test.go`

## Dependencies

Task 02 publishes environment identity and writes environment-scoped rows.

## Risks

- Shared environments can bind sessions from different tasks.
- Live execution discovery must use one request deadline.
- The delivery repository has direct SQL against the snapshot table.

## Parallelism

`sequential`

## Inputs

- System-design sections "Source precedence", "Delivery identity", and
  "Task summaries and direct consumers"
- Existing boot hydration and status-summary batch readers
- Existing delivery snapshot and sweep queries

## Results

- Hydration and refresh now select live executions and environment-owned
  persisted observations.
- Failed live queries do not fall back to older persisted snapshots.
- Task summaries, delivery readers, sweep queries, and analytics readers now
  use environment ownership or explicit session provenance.
- Verified the required environment-focused tests and all affected backend
  packages. The package run passed 2,348 tests.
- The final seven-package backend run passed 7,537 tests.
- Status-summary rebuilds now include the task-owned environment when no
  session rows remain. Delivery readers accept that owner binding while still
  supporting shared environments through session bindings.
- Recovered live Git status events now resolve and publish the session's
  environment identity. The final eight-package backend run passed 7,624
  tests.
- The final affected-package run passed 7,749 tests across nine backend
  packages after the replayable worktree/snapshot cutover fix.
