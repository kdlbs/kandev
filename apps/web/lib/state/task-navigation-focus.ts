import type { StoreApi } from "zustand";
import type { AppState } from "./store";
import { storeReadScopeIdentity } from "./shared-resource-reads";
import { isConfirmedSessionRemoved } from "./session-removal";

type RouteFocus = {
  taskId: string;
  sessionId: string;
  navigationRevision: number;
  scope: string;
};
const requests = new WeakMap<StoreApi<AppState>, RouteFocus>();

export function requestTaskRouteFocus(
  store: StoreApi<AppState>,
  taskId: string,
  sessionId: string,
) {
  requests.set(store, {
    taskId,
    sessionId,
    navigationRevision: store.getState().taskRemoval.navigationRevision,
    scope: storeReadScopeIdentity(store),
  });
}

export function hasTaskRouteFocus(
  store: StoreApi<AppState>,
  taskId: string | null,
  sessionId: string | null,
): boolean {
  const request = requests.get(store);
  const state = store.getState();
  return (
    !!request &&
    request.taskId === taskId &&
    request.sessionId === sessionId &&
    state.tasks.activeTaskId === taskId &&
    state.tasks.activeSessionId === sessionId &&
    request.navigationRevision === state.taskRemoval.navigationRevision &&
    request.scope === storeReadScopeIdentity(store) &&
    !isConfirmedSessionRemoved(state.taskSessions, request.sessionId)
  );
}

export function consumeTaskRouteFocus(
  store: StoreApi<AppState>,
  taskId: string | null,
  sessionId: string,
  layoutEnvironmentId: string | null,
) {
  const state = store.getState();
  const targetEnvironment =
    state.environmentIdBySessionId[sessionId] ??
    state.taskSessions.items[sessionId]?.task_environment_id;
  if (targetEnvironment && targetEnvironment !== layoutEnvironmentId) return;
  if (hasTaskRouteFocus(store, taskId, sessionId)) requests.delete(store);
}
