import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import { isConfirmedSessionRemoved } from "./session-removal";
import { getProvenRemovalPredecessor } from "./session-removal-handoff";
import { requestTaskRouteFocus } from "./task-navigation-focus";
import { observedRemovals } from "./session-removal-observations";

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
  const newerRequest =
    workflow.nextRequestId !== owner.workflowRequestId && workflow.request?.taskId === taskId;
  return state.taskRemoval.navigationRevision === owner.navigationRevision && !newerRequest;
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

function hasNewerRemoval(
  store: StoreApi<AppState>,
  owner: TaskNavigationSelectionOwner,
  taskId: string,
) {
  const state = store.getState();
  const predecessor = getProvenRemovalPredecessor(store, taskId, state.tasks.activeSessionId);
  return (
    !!predecessor &&
    (owner.removal?.taskId !== taskId ||
      owner.removal.predecessor !== predecessor ||
      owner.removal.target !== state.tasks.activeSessionId)
  );
}
export function isRequestedSessionUnavailable(store: StoreApi<AppState>, sessionId: string) {
  return (
    isConfirmedSessionRemoved(store.getState().taskSessions, sessionId) ||
    observedRemovals(store)?.has(sessionId)
  );
}

/** Valid explicit routes supersede entry-time ownership, never a later owner. */
export function admitExplicitTaskNavigationSelection(
  store: StoreApi<AppState>,
  incoming: Partial<AppState>,
  requestedSessionId: string | undefined,
  owner: TaskNavigationSelectionOwner | undefined,
): "deferred" | void {
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
    isRequestedSessionUnavailable(store, requestedSessionId)
  )
    return;
  if (hasNewerRemoval(store, owner, taskId)) return;
  const workflow = state.workflowSessionFocus;
  if (workflow.nextRequestId !== owner.workflowRequestId && workflow.intent?.taskId === taskId)
    return "deferred";
  state.setActiveSession(taskId, requestedSessionId);
  requestTaskRouteFocus(store, taskId, requestedSessionId);
}
