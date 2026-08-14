---
title: "Archive retains fresh zeroed summary"
status: pending
parallelism: sequential
parallel-safe: no
---

# Task 03: Archive Retains Fresh Zeroed Summary

## Goal

After archive, any sidebar surface that still shows the task (archived
filter / archived cache) ends with no queued badge. Live
`task.status_summary.updated` with `queued_prompt_count` omitted/0 must
win over the preserved pre-archive summary.

## Why This Task Exists

Frontend `mergeTaskUpdate` preserves `statusSummary` when a
`task.updated` payload omits `status_summary` (archive events do). Task 01
fixes the projector to zero and broadcast. This task verifies the
end-to-end client behavior and closes any remaining gap if the archived
cache stops applying summary updates for archived ids.

## RED / Verify

Prefer a focused unit test; only add code if Task 01's broadcast is not
already applied to archived rows.

1. Backend (if not covered by Task 01): archive with projector wired →
   stored summary `QueuedPromptCount == 0` and
   `task.status_summary.updated` published.
2. Frontend (`apps/web/lib/ws/handlers/tasks-status-summary.test.ts` or
   archive sibling):
   - Seed an archived (or about-to-be-archived) task in
     `sidebarArchivedTasks` / kanban with
     `statusSummary.queued_prompt_count = 11`.
   - Deliver `task.status_summary.updated` with a higher revision and
     `queued_prompt_count` omitted or `0`.
   - Assert the archived/active task's `statusSummary.queued_prompt_count`
     is 0/undefined so `buildSidebarItem` yields no badge.

If `updateTaskStatusSummaryInBothKanbans` already updates archived cache
rows, document that and only add the missing coverage. If it only touches
`kanban` + `kanbanMulti`, extend it to also update matching
`sidebarArchivedTasks` entries (minimal change).

## Implementation Notes

- Do not hardcode badge clearing in `task.deleted` / archive handlers if
  the summary broadcast already covers live clients.
- `buildArchivedSidebarItem` (route-focused placeholder) intentionally
  omits `queuedCount`; the multi-task archived filter uses
  `buildSidebarItem` on full `KanbanTask`s and is the badge surface.

## Acceptance

- Archiving a task with a positive badge results in no mail badge on the
  archived row after the summary update, without reload.
- Summary updates still apply to active kanban rows (no regression).
- Deleted tasks remain absent (Task 01 delete path + existing
  `removeTaskFromBothKanbans`).

## Validation

```bash
cd apps/backend
go test ./internal/task/statussummary/ ./internal/task/service/ ./internal/orchestrator/ -count=1

cd apps
pnpm --filter @kandev/web exec vitest run \
  lib/ws/handlers/tasks-status-summary.test.ts \
  lib/ws/handlers/tasks-archive.test.ts \
  components/task/task-session-sidebar-item.test.ts
```

## Dependencies

Task 01 (and preferably Task 02 if session-delete residual is in play).

## Output Contract

Report whether archived-cache summary updates needed a code change or
only a test; RED/GREEN; mark task + plan done when implementing.
