---
id: "01-backend-queue-state"
title: "Backend queue-state projection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-pr-merge-queue.md"
---

# Task 01: Backend queue-state projection

## Acceptance

- Batched GitHub GraphQL status reads normalize `mergeQueueEntry.state` and
  retain its one-based position and optional estimate in seconds, while
  queue-unaware reads explicitly preserve the last complete observation.
- The three `github_task_prs` queue fields survive every TaskPR persistence
  path, clear atomically on an authoritative queue exit or terminal lifecycle
  state, and emit the normal task-PR update when any value changes.
- Live and restart-built bounded task summaries derive the same `queued`
  aggregate state, including the existing multi-PR attention ordering.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/github/... ./internal/task/statussummary/... ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/client_helpers.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/mock_controller.go`
- `apps/backend/internal/github/graphql_merge_queue_status_test.go`
- `apps/backend/internal/github/service_pr_merge_queue_status_test.go`
- `apps/backend/internal/github/store_taskpr_schema_drift_test.go`
- `apps/backend/internal/github/store_merge_queue_status_test.go`
- `apps/backend/internal/task/statussummary/constants.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_pr.go`
- `apps/backend/internal/task/statussummary/rebuild.go`
- `apps/backend/internal/task/statussummary/projector_test.go`
- `apps/backend/internal/task/statussummary/rebuild_test.go`
- `apps/backend/internal/backendapp/status_summary_adapter.go`
- `apps/backend/internal/backendapp/status_summary_adapter_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task owns the shared queue-state contract, schema, sync, and
  bounded projection and queue metadata contract that later tasks consume.

## Inputs

- Spec sections: **Data Model**, **API Surface**, **State Machine**, **Failure
  Modes**, and queue-status scenarios.
- Plan sections: **GitHub queue observation**, **Persistence and
  synchronization**, and **Bounded task status**.
- Existing patterns: GraphQL populated guards on `PRStatus`, TaskPR explicit
  column lists, and `mergeable_state` event/rebuild projection.

## Risks

- Treating a queue-unaware read as an authoritative empty value would cause
  queued status to flicker or disappear when the detail panel refreshes.
- Missing one explicit TaskPR column/write path would break rollback-safe reads
  or silently drop queue metadata on association replacement.

## Output contract

Report the summary, exact files changed, exact test command and result,
blockers, remaining risks, and synchronized task/plan status in this
conversation.

## Results

Passed:

- `cd apps/backend && go test -tags fts5 ./internal/github/... ./internal/task/statussummary/... ./internal/backendapp`
  (2,193 tests passed across 3 packages, including terminal → queued
  precedence and failing-sibling ranking).
- Focused RED run recorded the expected missing-contract compile failures before
  implementation; the focused GREEN run passed 13 queue/projection tests.

No generated artifacts or external GitHub writes were produced. The trust
boundary remains GitHub's GraphQL `mergeQueueEntry`; queue-unaware REST and
`gh pr view` reads preserve the last observed queue fields.
