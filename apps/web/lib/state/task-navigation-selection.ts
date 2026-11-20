import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import { isConfirmedSessionRemoved } from "./session-removal";
import { getProvenRemovalPredecessor } from "./session-removal-handoff";

export type TaskNavigationSelectionOwner = {
  navigationRevision: number;
  workflowRequestId: number;
  removal: { taskId: string; predecessor: string; target: string | null } | null;
};

export function captureTaskNavigationSelection(
  store: StoreApi<AppState>,
): TaskNavigationSelectionOwner {
  const state = store.getState();
  const taskId = state.tasks.activeTaskId;
  const target = state.tasks.activeSessionId;
  const predecessor = taskId ? getProvenRemovalPredecessor(store, taskId, target) : null;
  return {
    navigationRevision: state.taskRemoval.navigationRevision,
    workflowRequestId: state.workflowSessionFocus.nextRequestId,
    removal: taskId && predecessor ? { taskId, predecessor, target } : null,
  };
}

function isTaskEntryCurrent(state: AppState, owner: TaskNavigationSelectionOwner, taskId: string) {
  const workflow = state.workflowSessionFocus;
  const newerWorkflow =
    workflow.nextRequestId !== owner.workflowRequestId &&
    (workflow.intent?.taskId === taskId || workflow.request?.taskId === taskId);
  return state.taskRemoval.navigationRevision === owner.navigationRevision && !newerWorkflow;
}

function hasRequestedMembership(incoming: Partial<AppState>, taskId: string, sessionId: string) {
  const membership = incoming.taskSessionsByTask;
  return (
    membership?.loadedByTaskId[taskId] &&
    membership.itemsByTaskId[taskId]?.some(
      (session) => session.id === sessionId && session.task_id === taskId,
    )
  );
}

/** Valid explicit routes supersede entry-time ownership, never a later owner. */
export function admitExplicitTaskNavigationSelection(
  store: StoreApi<AppState>,
  incoming: Partial<AppState>,
  requestedSessionId: string | undefined,
  owner: TaskNavigationSelectionOwner | undefined,
): void {
  const taskId = incoming.tasks?.activeTaskId;
  if (
    !owner ||
    !taskId ||
    !requestedSessionId ||
    incoming.tasks?.activeSessionId !== requestedSessionId
  )
    return;
  if (!hasRequestedMembership(incoming, taskId, requestedSessionId)) return;
  const state = store.getState();
  if (
    !isTaskEntryCurrent(state, owner, taskId) ||
    isConfirmedSessionRemoved(state.taskSessions, requestedSessionId)
  )
    return;
  const predecessor = getProvenRemovalPredecessor(store, taskId, state.tasks.activeSessionId);
  if (
    predecessor &&
    (owner.removal?.taskId !== taskId ||
      owner.removal.predecessor !== predecessor ||
      owner.removal.target !== state.tasks.activeSessionId)
  )
    return;
  state.setActiveSession(taskId, requestedSessionId);
}
