import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionState } from "@/lib/types/http";

const TERMINAL_SESSION_STATES: ReadonlySet<TaskSessionState> = new Set([
  "COMPLETED",
  "CANCELLED",
  "FAILED",
]);

export function isTerminalSessionState(state: TaskSessionState | undefined): boolean {
  return !!state && TERMINAL_SESSION_STATES.has(state);
}

/** Newest currently eligible same-task lifecycle recipient. */
export function pickReplacementSessionId(state: AppState, taskId: string): string | null {
  const sessions = state.taskSessionsByTask?.itemsByTaskId[taskId];
  if (!sessions) return null;
  for (let i = sessions.length - 1; i >= 0; i -= 1) {
    const row = state.taskSessions?.items[sessions[i].id] ?? sessions[i];
    if (row.task_id === taskId && row.state && !isTerminalSessionState(row.state)) return row.id;
  }
  return null;
}

function findSessionForTask(state: AppState, taskId: string, sessionId: string) {
  const byTask = state.taskSessionsByTask;
  const sessionsForTask = byTask?.itemsByTaskId?.[taskId];
  if (byTask?.loadedByTaskId?.[taskId]) {
    return sessionsForTask?.find((session) => session.id === sessionId) ?? null;
  }
  // Some task-update call sites use partial stores before taskSessions hydrates.
  const byId = state.taskSessions?.items?.[sessionId];
  if (byId) return byId.task_id === taskId ? byId : null;
  return sessionsForTask?.find((session) => session.id === sessionId) ?? null;
}

function isTaskSessionListHydrating(state: AppState, taskId: string): boolean {
  const byTask = state.taskSessionsByTask;
  if (!byTask) return true;
  if (byTask.loadingByTaskId?.[taskId]) return true;
  return !byTask.loadedByTaskId?.[taskId];
}

/**
 * Manual session selection pins a task-scoped session. Background WS events
 * may only override that pin once the pinned session is known terminal, or
 * when the terminal event is for the pinned session itself.
 */
export function shouldPreservePinnedSessionForTask(
  state: AppState,
  taskId: string,
  incoming?: { sessionId: string; newState: TaskSessionState | undefined },
): boolean {
  const pinnedSessionId = state.tasks.pinnedSessionId;
  if (!pinnedSessionId || state.tasks.activeTaskId !== taskId) return false;
  if (
    incoming?.sessionId === pinnedSessionId &&
    incoming.newState &&
    isTerminalSessionState(incoming.newState)
  ) {
    return false;
  }

  const pinnedSession = findSessionForTask(state, taskId, pinnedSessionId);
  if (!pinnedSession) {
    // Preserve missing rows only while the per-task list is still hydrating.
    // Once loaded, absence means the pinned session was deleted or went stale.
    return isTaskSessionListHydrating(state, taskId);
  }
  return !isTerminalSessionState(pinnedSession.state);
}

export function clearPinnedSessionIfOverridden(store: StoreApi<AppState>, sessionId: string): void {
  const pinnedSessionId = store.getState().tasks.pinnedSessionId;
  if (!pinnedSessionId || pinnedSessionId === sessionId) return;
  store.setState((state) => ({
    ...state,
    tasks: { ...state.tasks, pinnedSessionId: null },
  }));
}
