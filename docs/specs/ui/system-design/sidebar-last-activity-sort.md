---
status: draft
system: ui
requirements:
  - REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-001
---

# Sidebar Last Activity Sort System Design

## Purpose and boundaries

The UI contract distinguishes work on a task from background status maintenance. The task repository and status-summary projector provide the durable activity value; desktop and phone sidebars consume it through the same task summary. This design records the recovery boundary needed for that existing contract. It does not change the saved-view key or the sidebar layout.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-001` | [Activity source and projection](#activity-source-and-projection), [Completed move recovery](#completed-move-recovery), [Verification](#verification) |

## Activity source and projection

`TaskStatusSummary.LastActivityAt` is the monotonic, semantic time used by the sidebar's `lastActivityAt` sort. `TaskStatusSummary.UpdatedAt` remains summary freshness. The projector in `apps/backend/internal/task/statussummary/projector_events.go` advances activity on `task.updated` and `task.state_changed` using the event's persisted task `updated_at`. The batched reconstruction in `apps/backend/internal/task/repository/sqlite/task_status_summary.go` also includes `tasks.updated_at`. Live projection and reconstruction must therefore agree about which task writes change that timestamp.

`apps/web/components/task/task-session-sidebar-item.ts` maps `status_summary.last_activity_at` to each row, with task update and creation fallbacks. `apps/web/lib/sidebar/apply-view.ts` orders task trees by that value. No frontend mapping or saved-view format changes are needed for the recovery correction.

## Completed move recovery

The orchestrator's startup lifecycle sweep calls `recoverTaskLifecycleAttempt` for tasks with `manual_move_lifecycle_completed`. After feeder reconciliation succeeds, `clearManualMoveLifecycleMarkersIfCompleted` asks the repository to atomically clear the completed marker and any stale pending marker. This is bookkeeping for a move that has already happened, not a new task action.

`ClearManualMoveLifecycleMarkersIfCompleted` in `apps/backend/internal/task/repository/sqlite/task.go` shall preserve `tasks.updated_at` while removing only those markers. Its existing predicate must still require the observed `updated_at` generation and a present completed marker. A newer manual move writes a new generation before the old clear can succeed, including when the newer completed marker has the same boolean value. A missing marker or stale generation must remain a no-op. Keep the existing `task.updated` publication after a successful clear so subscribers converge on the changed metadata; the event carries the preserved task timestamp and cannot advance semantic activity.

Real task moves and edits continue to update `tasks.updated_at` and advance activity. This correction is limited to completed-marker cleanup; it does not change general task metadata write behavior. The repository has SQLite and PostgreSQL implementations, so both branches must preserve the same predicate and timestamp behavior. No schema or API migration is needed.

## Recovery and limits

On each restart, a successfully cleared marker no longer qualifies for another lifecycle sweep. A failed feeder reconciliation leaves the marker for retry. Concurrent newer moves remain protected by the generation predicate. Because the reconstruction query reads the preserved `tasks.updated_at`, a subsequent summary rebuild also keeps the pre-cleanup activity time.

Rows already advanced by older versions cannot be automatically restored from current task records: their previous task update time was overwritten, and messages and turns do not capture every legitimate task edit. Historical repair requires separate, source-specific evidence and is outside this correction.

## Verification

- A SQLite and PostgreSQL repository contract proves successful cleanup removes only the two markers while preserving task `updated_at` and reconstructed activity. It also proves missing and stale-generation clears do nothing.
- An orchestrator integration regression starts from a completed marker and an older projected activity time, runs startup recovery, and verifies marker convergence, unchanged `last_activity_at`, and unchanged task state. Existing newer-move race tests continue to pass.
- The existing desktop and phone sidebar Playwright flows verify that `lastActivityAt` remains the displayed and sorted value; backend tests own the restart-specific trigger. The phone task-switcher drawer and desktop sidebar consume the same summary field without a composition change.

## Related design

- [Activity timestamp decision](../../../decisions/2026-08-17-separate-task-activity-from-summary-freshness.md)
- [Task tree activity order](sidebar-task-tree-activity-sort.md)
