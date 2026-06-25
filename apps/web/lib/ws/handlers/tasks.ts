import type { StoreApi } from "zustand";
import { cleanupTaskStorage } from "@/lib/local-storage";
import { removeRecentTask } from "@/lib/recent-tasks";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type TaskUpdatedPayload = {
  task_id: string;
  primary_session_id?: string | null;
  is_ephemeral?: boolean;
};

type TaskDeletedPayload = {
  task_id: string;
};

export function registerTasksHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.updated": (message) => {
      maybeFollowPrimarySession(store, message.payload as TaskUpdatedPayload);
    },
    "task.deleted": (message) => {
      handleTaskDeleted(store, message.payload as TaskDeletedPayload);
    },
  };
}

function maybeFollowPrimarySession(store: StoreApi<AppState>, payload: TaskUpdatedPayload): void {
  if (payload.is_ephemeral) return;
  const taskId = payload.task_id;
  const newPrimary = payload.primary_session_id ?? null;
  if (!taskId || !newPrimary) return;

  const state = store.getState();
  const previousPrimary = state.tasks.activeSessionId ?? null;
  if (
    newPrimary !== previousPrimary &&
    state.tasks.activeTaskId === taskId &&
    state.tasks.activeSessionId === previousPrimary &&
    state.tasks.pinnedSessionId !== previousPrimary
  ) {
    state.setActiveSessionAuto(taskId, newPrimary);
  }
}

function handleTaskDeleted(store: StoreApi<AppState>, payload: TaskDeletedPayload): void {
  const deletedId = payload.task_id;
  if (!deletedId) return;

  removeRecentTask(deletedId);

  const currentState = store.getState();
  const sessionIds = sessionIdsForDeletedTask(currentState, deletedId);
  const envIds = environmentIdsForSessions(currentState, sessionIds);
  cleanupTaskStorage(deletedId, sessionIds, envIds);
  currentState.removeTaskFromSidebarPrefs(deletedId);
  for (const sid of sessionIds) {
    useContextFilesStore.getState().clearSession(sid);
  }

  store.setState((state) => cleanupDeletedTaskClientState(state, deletedId));
}

function sessionIdsForDeletedTask(state: AppState, taskId: string): string[] {
  const ids = new Set<string>(
    (state.taskSessionsByTask?.itemsByTaskId[taskId] ?? []).map((session) => session.id),
  );
  for (const session of Object.values(state.taskSessions?.items ?? {})) {
    if (session.task_id === taskId) ids.add(session.id);
  }
  return [...ids];
}

function environmentIdsForSessions(state: AppState, sessionIds: string[]): string[] {
  return Array.from(
    new Set(
      sessionIds
        .map((sid) => state.environmentIdBySessionId[sid])
        .filter((eid): eid is string => Boolean(eid)),
    ),
  );
}

function cleanupDeletedTaskClientState(state: AppState, deletedId: string): AppState {
  let next = state;
  if (state.tasks.activeTaskId === deletedId) {
    next = { ...next, tasks: { ...next.tasks, activeTaskId: null, activeSessionId: null } };
  }
  if (next.tasks.lastSessionByTaskId[deletedId]) {
    const rest = { ...next.tasks.lastSessionByTaskId };
    delete rest[deletedId];
    next = { ...next, tasks: { ...next.tasks, lastSessionByTaskId: rest } };
  }
  return next;
}
