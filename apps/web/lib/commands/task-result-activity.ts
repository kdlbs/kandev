import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { pickFreshestStatusSummary } from "@/lib/task-status-summary";
import type { ForegroundActivity, Task, TaskSessionState, TaskState } from "@/lib/types/http";

export type CommandPanelLiveTask = KanbanState["tasks"][number];

export type TaskResultActivity = {
  state: TaskState;
  workflowStepId: string;
  sessionState?: TaskSessionState;
  foregroundActivity?: ForegroundActivity | null;
  hasPendingClarification: boolean;
  hasPendingPermission: boolean;
  isOnLastWorkflowStep: boolean;
  interrupted?: boolean;
};

function currentLiveTask(task: Task, liveTask?: CommandPanelLiveTask) {
  if (!liveTask) return undefined;
  if (!liveTask.updatedAt) {
    // Legacy boot payloads can omit updatedAt. Treat this projection as the
    // current WebSocket-backed reading so its lifecycle and cleared activity
    // fields do not fall back to an older search response.
    return liveTask;
  }
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
  const liveSummary = liveTask?.statusSummary;
  const summary = pickFreshestStatusSummary(liveSummary, task.status_summary);
  if (summary === liveSummary && liveSummary) return liveSummary.foreground_activity ?? null;
  if (directLiveTask) return directLiveTask.foregroundActivity ?? null;
  return summary?.foreground_activity ?? task.foreground_activity ?? null;
}

/** Merge the search response with the live task projection already kept current by WebSocket events. */
export function resolveTaskResultActivity(
  task: Task,
  liveTask?: CommandPanelLiveTask,
  lastStepIdByWorkflowId?: ReadonlyMap<string, string>,
): TaskResultActivity {
  const directLiveTask = currentLiveTask(task, liveTask);
  const pendingAction = resolvePendingAction(task, liveTask, directLiveTask);
  const effectiveWorkflowId = directLiveTask?.workflowId ?? task.workflow_id;
  const effectiveWorkflowStepId = directLiveTask?.workflowStepId ?? task.workflow_step_id;

  return {
    state: directLiveTask?.state ?? task.state,
    workflowStepId: effectiveWorkflowStepId,
    sessionState: resolveSessionState(task, liveTask, directLiveTask),
    foregroundActivity: resolveForegroundActivity(task, liveTask, directLiveTask),
    hasPendingClarification: pendingAction === "clarification",
    hasPendingPermission: pendingAction === "permission",
    isOnLastWorkflowStep:
      lastStepIdByWorkflowId?.get(effectiveWorkflowId) === effectiveWorkflowStepId,
    interrupted: directLiveTask?.interrupted ?? task.interrupted,
  };
}
