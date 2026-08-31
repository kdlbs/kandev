import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { pickFreshestStatusSummary } from "@/lib/task-status-summary";
import type { ForegroundActivity, Task, TaskSessionState, TaskState } from "@/lib/types/http";

export type CommandPanelLiveTask = KanbanState["tasks"][number];

export type TaskResultActivity = {
  state: TaskState;
  sessionState?: TaskSessionState;
  foregroundActivity?: ForegroundActivity | null;
  hasPendingClarification: boolean;
  hasPendingPermission: boolean;
  interrupted?: boolean;
};

function currentLiveTask(task: Task, liveTask?: CommandPanelLiveTask) {
  if (!liveTask) return undefined;
  if (!liveTask.updatedAt) return liveTask;
  const liveUpdatedAt = Date.parse(liveTask.updatedAt);
  const resultUpdatedAt = Date.parse(task.updated_at);
  if (!Number.isFinite(liveUpdatedAt) || !Number.isFinite(resultUpdatedAt)) return liveTask;
  return liveUpdatedAt >= resultUpdatedAt ? liveTask : undefined;
}

function resolvePendingAction(
  task: Task,
  liveTask?: CommandPanelLiveTask,
  directLiveTask?: CommandPanelLiveTask,
) {
  const summary = pickFreshestStatusSummary(liveTask?.statusSummary, task.status_summary);
  return (
    summary?.pending_action ??
    directLiveTask?.taskPendingAction ??
    directLiveTask?.primarySessionPendingAction ??
    task.task_pending_action ??
    task.primary_session_pending_action
  );
}

function resolveSessionState(
  task: Task,
  liveTask?: CommandPanelLiveTask,
  directLiveTask?: CommandPanelLiveTask,
) {
  const summary = pickFreshestStatusSummary(liveTask?.statusSummary, task.status_summary);
  return (
    summary?.primary_session?.state ??
    (directLiveTask?.primarySessionState as TaskSessionState | null | undefined) ??
    task.primary_session_state ??
    undefined
  );
}

function resolveForegroundActivity(
  task: Task,
  liveTask?: CommandPanelLiveTask,
  directLiveTask?: CommandPanelLiveTask,
) {
  const summary = pickFreshestStatusSummary(liveTask?.statusSummary, task.status_summary);
  return (
    summary?.foreground_activity ??
    directLiveTask?.foregroundActivity ??
    task.foreground_activity ??
    null
  );
}

/** Merge the search response with the live task projection already kept current by WebSocket events. */
export function resolveTaskResultActivity(
  task: Task,
  liveTask?: CommandPanelLiveTask,
): TaskResultActivity {
  const directLiveTask = currentLiveTask(task, liveTask);
  const pendingAction = resolvePendingAction(task, liveTask, directLiveTask);

  return {
    state: directLiveTask?.state ?? task.state,
    sessionState: resolveSessionState(task, liveTask, directLiveTask),
    foregroundActivity: resolveForegroundActivity(task, liveTask, directLiveTask),
    hasPendingClarification: pendingAction === "clarification",
    hasPendingPermission: pendingAction === "permission",
    interrupted: directLiveTask?.interrupted ?? task.interrupted,
  };
}
