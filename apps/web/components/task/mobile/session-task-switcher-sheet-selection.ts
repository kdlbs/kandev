import { replaceTaskUrl } from "@/lib/links";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import type { TaskPendingAction, TaskSession } from "@/lib/types/http";
import { resolvePreferredSessionId, resolveTaskSessionId } from "../task-select-helpers";

type SelectionActions = {
  loadTaskSessionsForTask: (taskId: string) => Promise<TaskSession[]>;
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
  navigate?: (taskId: string) => void;
  onOpenChange: (open: boolean) => void;
};

type SelectableTask = {
  isArchived?: boolean;
  primarySessionId?: string | null;
  taskPendingAction?: TaskPendingAction | null;
  statusSummary?: { pending_action?: TaskPendingAction | null } | null;
};

type SelectionState = {
  lastSessionByTaskId: Record<string, string>;
  environmentIdBySessionId: Record<string, string>;
  taskSessionsById: Record<string, TaskSession>;
};

export async function selectPendingTaskFromSheet(
  params: {
    taskId: string;
    preferredSessionId: string;
    taskPendingAction: TaskPendingAction;
  } & SelectionActions,
): Promise<void> {
  let targetSessionId = params.preferredSessionId;
  try {
    const sessions = await params.loadTaskSessionsForTask(params.taskId);
    targetSessionId = resolveTaskSessionId({
      sessions,
      preferredSessionId: params.preferredSessionId,
      taskPendingAction: params.taskPendingAction,
    });
  } catch (error) {
    console.error("Failed to load pending task sessions:", error);
  }
  if (targetSessionId) {
    params.setActiveSession(params.taskId, targetSessionId);
  } else {
    params.setActiveTask(params.taskId);
  }
  (params.navigate ?? replaceTaskUrl)(params.taskId);
  params.onOpenChange(false);
}

async function selectTaskWithoutPrimarySession(taskId: string, actions: SelectionActions) {
  const navigate = actions.navigate ?? replaceTaskUrl;
  try {
    const sessions = await actions.loadTaskSessionsForTask(taskId);
    const sessionId = sessions[0]?.id ?? null;
    if (sessionId) {
      actions.setActiveSession(taskId, sessionId);
      navigate(taskId);
      actions.onOpenChange(false);
      return;
    }
    const { request } = buildPrepareRequest(taskId);
    try {
      const response = await launchSession(request);
      if (response.session_id) {
        actions.setActiveSession(taskId, response.session_id);
        navigate(taskId);
        actions.onOpenChange(false);
        return;
      }
    } catch {
      // Fall through to default navigation.
    }
  } catch (error) {
    console.error("Failed to load sessions for task:", error);
  }
  actions.setActiveTask(taskId);
  navigate(taskId);
  actions.onOpenChange(false);
}

export function selectTaskFromSheet(
  params: {
    taskId: string;
    task?: SelectableTask;
    state: SelectionState;
  } & SelectionActions,
): void {
  const { taskId, task, state } = params;
  const navigate = params.navigate ?? replaceTaskUrl;
  if (task?.isArchived) {
    params.setActiveTask(taskId);
    navigate(taskId);
    params.onOpenChange(false);
    return;
  }
  const pendingAction =
    task?.statusSummary != null ? task.statusSummary.pending_action : task?.taskPendingAction;
  const preferredSessionId = task?.primarySessionId
    ? resolvePreferredSessionId({
        taskId,
        primarySessionId: task.primarySessionId,
        lastSessionByTaskId: state.lastSessionByTaskId,
        environmentIdBySessionId: state.environmentIdBySessionId,
        taskSessionsById: state.taskSessionsById,
      })
    : "";
  if (pendingAction) {
    void selectPendingTaskFromSheet({
      ...params,
      preferredSessionId,
      taskPendingAction: pendingAction,
    });
    return;
  }
  if (preferredSessionId) {
    params.setActiveSession(taskId, preferredSessionId);
    void params.loadTaskSessionsForTask(taskId);
    navigate(taskId);
    params.onOpenChange(false);
    return;
  }
  void selectTaskWithoutPrimarySession(taskId, params);
}
