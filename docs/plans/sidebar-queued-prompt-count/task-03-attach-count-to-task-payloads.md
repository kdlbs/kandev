---
id: "03-attach-count-to-task-payloads"
title: "Attach the count to task list/snapshot payloads"
status: done
wave: 3
depends_on: ["02-status-summary-queued-count"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-queued-prompt-count.md"
---

# Task 03: Attach the Count to Task List/Snapshot Payloads

## Acceptance

- The task service gains an optional `QueuedPromptCounter` interface (batched:
  `CountPendingByTaskIDs(ctx, taskIDs []string) (map[string]int, error)`) plus
  `SetQueuedPromptCounter` (pattern of `ForegroundActivityProvider`), and a
  batch helper `CountPendingQueuedByTaskIDs`. The task service must not import
  `internal/orchestrator/messagequeue`.
- `buildTaskDTOsWithSessionInfo` (the single assembly used by HTTP list,
  workspace list, single task, WS list, and workflow/workspace snapshot)
  loads fresh counts for the listed tasks and stamps
  `StatusSummary.QueuedPromptCount` onto each task's summary. It never
  fabricates a summary for a task that has none — a synthetic summary would
  flip `sidebarSessionStatus` to summary-authoritative and hide the relative
  time (see Risks). Tasks without a summary keep no count field.
- `HydrateMissingTaskStatusSummaries` includes the count in summaries it
  repairs: `statussummary.RebuildInput` gains the count and the hydrate path
  fills it from the counter. Repaired rows therefore carry the badge on the
  next load even before the projector touches them.
- backendapp wires the messagequeue-backed adapter into both the task service
  and (already done by Task 02) the projector.
- Tests prove: assembly stamps fresh counts on tasks with summaries; repaired
  summaries include counts; tasks without summaries omit the field; counter
  failure degrades to no badge without failing the list; the JSON payload of a
  list/snapshot response carries `status_summary.queued_prompt_count`.
- Counter-failure DTO contract: when the counter errors, the response DTO
  clears `queued_prompt_count` to `0` for that request only — never persisted.
  When the counter is unwired (provider unavailable), the DTO preserves the
  projected count instead of stamping a zero. Regression tests cover both: an
  errored counter yields a non-nil summary with count `0`; an unwired counter
  preserves a positive projected count (`TestTaskDTOBuilderPreservesProjectedCountWhenCounterUnwired`).

## TDD Sequence

1. Write service/assembly/hydration tests for the acceptance cases. Run RED.
2. Implement the counter interface + setter, the batch helper, the
   `RebuildInput` extension, the assembly stamping, and the gateway wiring.
3. Run the task-package and backendapp tests GREEN, then refactor.

## Verification

```bash
cd apps/backend
go test ./internal/task/... ./internal/backendapp/...
```

## Files Likely Touched

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild_test.go`
- `apps/backend/internal/task/statussummary/rebuild.go` (RebuildInput)
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/backendapp/gateway.go`

## Dependencies

Task 02 defines the summary field this task stamps; Task 01 supplies the
underlying count query.

## Risks

- Never construct a `TaskStatusSummary` solely to carry the count; the
  frontend treats a non-nil summary as authoritative for session/updatedAt
  fields. Stamp only existing summaries; hydration is the only creator.

## Output Contract

Report RED/GREEN evidence, the final DTO JSON for a task with and without a
summary, and changed files. Update this task and `plan.md` status in the same
implementation conversation.

## Results

- RED: assembly/hydration/rebuild tests failed against the missing count plumbing.
- GREEN: `go test -tags fts5 ./internal/task/... ./internal/backendapp/...` — assembly stamps fresh counts on
  existing summaries, hydration includes counts in repaired rows, tasks without summaries omit the field,
  and a counter failure clears the response count without persisting.
- `TestBootKanbanSnapshotStampsQueuedPromptCount` proves the boot kanban snapshot carries `queued_prompt_count`;
  `TestTaskDTOBuilderPreservesProjectedCountWhenCounterUnwired` proves an unwired counter leaves a projected
  positive count intact.
- DTO JSON for a queued task carries `"queued_prompt_count":3` inside `status_summary`; a task with no summary
  carries no count field.
