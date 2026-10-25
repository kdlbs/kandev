import { replaceTaskUrl } from "@/lib/links";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import type { TaskPendingAction, TaskSession } from "@/lib/types/http";
import { isAbortError } from "@/lib/utils/abort-error";
import {
  effectiveTaskPendingAction,
  resolvePreferredSessionId,
  resolveTaskSessionId,
  taskPendingSelectionMatches,
  taskPendingSelectionSnapshot,
  type TaskPendingSelectionSnapshot,
} from "../task-select-helpers";

type SelectionActions = {
  loadTaskSessionsForTask: (
    taskId: string,
    options?: { force?: boolean },
  ) => Promise<TaskSession[]>;
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
  navigate?: (taskId: string, sessionId?: string) => void;
  onOpenChange: (open: boolean) => void;
  isSelectionCurrent?: () => boolean;
  getTaskPendingSnapshot?: (taskId: string) => TaskPendingSelectionSnapshot | undefined;
};

type SelectableTask = {
  isArchived?: boolean;
  primarySessionId?: string | null;
  taskPendingAction?: TaskPendingAction | null;
  statusSummary?: {
    revision?: number;
    updated_at?: string;
    pending_action?: TaskPendingAction | null;
  } | null;
};

type SelectionState = {
  lastSessionByTaskId: Record<string, string>;
  environmentIdBySessionId: Record<string, string>;
  taskSessionsById: Record<string, TaskSession>;
};

export type TaskSheetSelectionController = {
  beginSelection: () => number;
  invalidate: () => void;
  isCurrent: (token: number) => boolean;
};

export function createTaskSheetSelectionController(): TaskSheetSelectionController {
  let sequence = 0;
  return {
    beginSelection: () => {
      sequence += 1;
      return sequence;
    },
    invalidate: () => {
      sequence += 1;
    },
    isCurrent: (token) => token === sequence,
  };
}

export function handleTaskSheetOpenChange(
  selectionController: TaskSheetSelectionController,
  open: boolean,
  onOpenChange: (open: boolean) => void,
): void {
  if (!open) selectionController.invalidate();
  onOpenChange(open);
}

function selectionIsCurrent(actions: SelectionActions): boolean {
  return actions.isSelectionCurrent?.() ?? true;
}

function pendingActionState(
  actions: SelectionActions,
  taskId: string,
  initial: TaskPendingSelectionSnapshot,
): "current" | "changed" | "missing" {
  if (!actions.getTaskPendingSnapshot) return "current";
  const current = actions.getTaskPendingSnapshot(taskId);
  if (!current) return "missing";
  return taskPendingSelectionMatches(initial, current) ? "current" : "changed";
}

export async function selectPendingTaskFromSheet(
  params: {
    taskId: string;
    preferredSessionId: string;
    taskPendingAction: TaskPendingAction;
    pendingSnapshot?: TaskPendingSelectionSnapshot;
  } & SelectionActions,
): Promise<void> {
  const navigate = params.navigate ?? replaceTaskUrl;
  let targetSessionId = "";
  try {
    const sessions = await params.loadTaskSessionsForTask(params.taskId, { force: true });
    targetSessionId = resolveTaskSessionId({
      sessions,
      preferredSessionId: params.preferredSessionId,
      taskPendingAction: params.taskPendingAction,
    });
  } catch (error) {
    if (isAbortError(error)) return;
    console.error("Failed to load pending task sessions:", error);
  }
  const initialSnapshot = params.pendingSnapshot ?? {
    revision: null,
    pendingAction: params.taskPendingAction,
  };
  if (!selectionIsCurrent(params)) return;
  const pendingState = pendingActionState(params, params.taskId, initialSnapshot);
  if (pendingState === "missing") {
    return;
  }
  if (pendingState === "changed") {
    params.setActiveTask(params.taskId);
    navigate(params.taskId);
    params.onOpenChange(false);
    return;
  }
  if (targetSessionId) {
    params.setActiveSession(params.taskId, targetSessionId);
  } else {
    params.setActiveTask(params.taskId);
  }
  navigate(params.taskId, targetSessionId || undefined);
  params.onOpenChange(false);
}

async function selectTaskWithoutPrimarySession(taskId: string, actions: SelectionActions) {
  const navigate = actions.navigate ?? replaceTaskUrl;
  try {
    const sessions = await actions.loadTaskSessionsForTask(taskId);
    if (!selectionIsCurrent(actions)) return;
    const sessionId = sessions[0]?.id ?? null;
    if (sessionId) {
      actions.setActiveSession(taskId, sessionId);
      navigate(taskId, sessionId);
      actions.onOpenChange(false);
      return;
    }
    const { request } = buildPrepareRequest(taskId);
    try {
      const response = await launchSession(request);
      if (!selectionIsCurrent(actions)) return;
      if (response.session_id) {
        actions.setActiveSession(taskId, response.session_id);
        navigate(taskId, response.session_id);
        actions.onOpenChange(false);
        return;
      }
    } catch {
      // Fall through to default navigation.
    }
  } catch (error) {
    if (isAbortError(error)) return;
    console.error("Failed to load sessions for task:", error);
  }
  if (!selectionIsCurrent(actions)) return;
  actions.setActiveTask(taskId);
  navigate(taskId);
  actions.onOpenChange(false);
}

export function selectTaskFromSheet(
  params: {
    taskId: string;
    task?: SelectableTask;
    state: SelectionState;
    selectionController: TaskSheetSelectionController;
  } & SelectionActions,
): void {
  const { taskId, task, state } = params;
  const selectionToken = params.selectionController.beginSelection();
  const guardedParams = {
    ...params,
    isSelectionCurrent: () => params.selectionController.isCurrent(selectionToken),
  };
  const navigate = params.navigate ?? replaceTaskUrl;
  if (task?.isArchived) {
    void (async () => {
      let sessionId: string | undefined;
      try {
        const sessions = await params.loadTaskSessionsForTask(taskId);
        if (!selectionIsCurrent(guardedParams)) return;
        const preferredSessionId = task.primarySessionId
          ? resolvePreferredSessionId({
              taskId,
              primarySessionId: task.primarySessionId,
              lastSessionByTaskId: state.lastSessionByTaskId,
              environmentIdBySessionId: state.environmentIdBySessionId,
              taskSessionsById: state.taskSessionsById,
            })
          : "";
        const ownedSessions = sessions.filter((session) => session.task_id === taskId);
        sessionId =
          ownedSessions.find((session) => session.id === preferredSessionId)?.id ??
          ownedSessions.find((session) => session.id === task.primarySessionId)?.id ??
          ownedSessions.find((session) => session.is_primary)?.id ??
          ownedSessions[0]?.id;
      } catch (error) {
        if (isAbortError(error)) return;
      }
      if (!selectionIsCurrent(guardedParams)) return;
      if (sessionId) params.setActiveSession(taskId, sessionId);
      else params.setActiveTask(taskId);
      navigate(taskId, sessionId);
      params.onOpenChange(false);
    })();
    return;
  }
  const pendingAction = effectiveTaskPendingAction(task);
  const pendingSnapshot = taskPendingSelectionSnapshot(task);
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
      ...guardedParams,
      preferredSessionId,
      taskPendingAction: pendingAction,
      pendingSnapshot,
    });
    return;
  }
  if (preferredSessionId) {
    params.setActiveSession(taskId, preferredSessionId);
    void params.loadTaskSessionsForTask(taskId).catch(() => undefined);
    navigate(
      taskId,
      preferredSessionId === task?.primarySessionId ? undefined : preferredSessionId,
    );
    params.onOpenChange(false);
    return;
  }
  void selectTaskWithoutPrimarySession(taskId, guardedParams);
}
