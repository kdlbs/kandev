---
id: "03-project-live-task-activity"
title: "Project task activity live"
status: completed
wave: 2
depends_on: ["02-add-activity-reconstruction"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-last-activity-sort.md"
---

# Task 03: Project task activity live

Extend the bounded task summary with a monotonic activity time and repair older
summary rows from the new batched source.

## Acceptance

- Summary JSON, equality, persistence, DTOs, and replacement events carry
  optional `last_activity_at` without changing `updated_at`.
- Task creation, task update, state-only task transition, user-message, user
  queue admission, and turn events advance activity by source time. Queue
  bookkeeping, excluded background status, and older replay events do not
  advance it.
- Initial hydration repairs missing values in batch and preserves newer stored
  activity through compare-and-set retries.

## Verification

```bash
cd apps/backend && go test ./internal/task/statussummary -run 'LastActivity'
cd apps/backend && go test ./internal/task/service -run 'StatusSummary.*LastActivity'
cd apps/backend && go test ./internal/backendapp -run 'StatusSummary'
```

## Files likely touched

- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/model_test.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_derive.go`
- `apps/backend/internal/task/statussummary/projector_test.go`
- `apps/backend/internal/task/statussummary/projector_restart_rehydration_test.go`
- `apps/backend/internal/task/statussummary/rebuild.go`
- `apps/backend/internal/task/statussummary/rebuild_test.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild_test.go`
- `apps/backend/internal/backendapp/gateway.go`

## Dependencies

Task 02.

## Risks

- A status-only event must not copy projection time into activity time.
- State transitions publish `task.state_changed` without a matching
  `task.updated` on several service paths, so the projector must consume both.
- Message events count only user-authored additions.
- Repair and live projection must share the same monotonic rule.

## Result

- Added optional semantic `last_activity_at` to live summary replacement
  payloads and projector state, with durable loader rehydration.
- Subscribed to task creation, state-only transitions, user queue admission,
  and turn start and completion. User-authored message additions and queued
  admissions use their persisted source time; background status, provider,
  queue bookkeeping, agent-message, and older replay events do not advance the
  monotonic maximum. Queue merges retain the newest admission time.
- Focused verification passed:
  - `go test ./internal/task/statussummary -run 'LastActivity'` (3 tests)
  - `go test ./internal/task/service -run 'StatusSummary.*LastActivity'` (no matching tests)
  - `go test ./internal/backendapp -run 'StatusSummary'` (7 tests)
  - `go test ./internal/task/statussummary` (64 tests)
- Review fixup verification: affected backend packages passed 1,031 tests,
  including live queued-prompt projection and SQLite/Postgres reconstruction
  coverage.
