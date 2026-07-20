import type { QueryClient } from "@tanstack/react-query";
import type { StoreApi } from "zustand";
import { cleanupTaskStorage } from "@/lib/local-storage";
import { getBrowserQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import { workflowSnapshotQueryData } from "@/lib/query/workflow-snapshot-cache";
import { removeRecentTask } from "@/lib/recent-tasks";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { softNavigate } from "@/lib/routing/client-router";
import { isTaskDetailPath, normalizePathname } from "@/lib/links";
import {
  clearPinnedSessionIfOverridden,
  shouldPreservePinnedSessionForTask,
} from "@/lib/ws/handlers/agent-session";
import type { AppState } from "@/lib/state/store";
import type { Task } from "@/lib/types/http";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type TaskUpdatedPayload = {
  task_id: string;
  primary_session_id?: string | null;
  is_ephemeral?: boolean;
  archived_at?: string | null;
};

type TaskDeletedPayload = {
  task_id: string;
  title?: string;
  reason?: string;
};

function clearDeletedTaskWalkthroughClientState(state: AppState, taskId: string): AppState {
  if (!state.walkthroughs) return state;
  if (
    !(taskId in state.walkthroughs.activeStepByTaskId) &&
    !(taskId in state.walkthroughs.lastSeenUpdatedAtByTaskId)
  ) {
    return state;
  }
  const { [taskId]: _removedStep, ...activeStepByTaskId } = state.walkthroughs.activeStepByTaskId;
  const { [taskId]: _removedLastSeen, ...lastSeenUpdatedAtByTaskId } =
    state.walkthroughs.lastSeenUpdatedAtByTaskId;
  return {
    ...state,
    walkthroughs: {
      ...state.walkthroughs,
      activeStepByTaskId,
      lastSeenUpdatedAtByTaskId,
    },
  };
}

function removedTaskRedirectHref(pathname: string, taskId: string): string | null {
  if (isTaskDetailPath(pathname, taskId)) return "/";
  const normalized = normalizePathname(pathname);
  return normalized === `/office/tasks/${taskId}` ? "/office/tasks" : null;
}

/**
 * Soft-redirect away from a removed task's page. Only fires when the user is
 * currently parked on that task's route, so a background removal of some other
 * task never yanks the user elsewhere.
 */
function redirectAwayFromRemovedTask(taskId: string): void {
  if (typeof window === "undefined") return;
  const href = removedTaskRedirectHref(window.location.pathname, taskId);
  if (!href) return;
  softNavigate(href, "replace");
}

export function registerTasksHandlers(
  store: StoreApi<AppState>,
  queryClient: QueryClient = getBrowserQueryClient(),
): WsHandlers {
  return {
    "task.updated": (message) => {
      const payload = message.payload as TaskUpdatedPayload;
      if (payload.archived_at) {
        handleTaskArchived(store, payload);
        return;
      }
      maybeFollowPrimarySession(store, queryClient, payload);
    },
    "task.deleted": (message) => {
      handleTaskDeleted(store, message.payload as TaskDeletedPayload);
    },
  };
}

function maybeFollowPrimarySession(
  store: StoreApi<AppState>,
  queryClient: QueryClient,
  payload: TaskUpdatedPayload,
): void {
  if (payload.is_ephemeral) return;
  const taskId = payload.task_id;
  const newPrimary = payload.primary_session_id ?? null;
  if (!taskId || !newPrimary) return;

  const state = store.getState();
  const previousPrimary = cachedPrimarySessionId(queryClient, taskId);
  if (previousPrimary === undefined) return;
  if (
    newPrimary !== previousPrimary &&
    state.tasks.activeTaskId === taskId &&
    state.tasks.activeSessionId === previousPrimary &&
    !shouldPreservePinnedSessionForTask(state, taskId)
  ) {
    clearPinnedSessionIfOverridden(store, newPrimary);
    state.setActiveSessionAuto(taskId, newPrimary);
  }
}

function cachedPrimarySessionId(
  queryClient: QueryClient,
  taskId: string,
): string | null | undefined {
  const cached = queryClient.getQueryData<Pick<Task, "primary_session_id">>(
    qk.tasks.detail(taskId),
  );
  if (cached && Object.prototype.hasOwnProperty.call(cached, "primary_session_id")) {
    return cached.primary_session_id ?? null;
  }
  for (const snapshot of workflowSnapshotQueryData(queryClient)) {
    const task = snapshot.tasks.find((candidate) => candidate.id === taskId);
    if (task && Object.prototype.hasOwnProperty.call(task, "primary_session_id")) {
      return task.primary_session_id ?? null;
    }
  }
  return undefined;
}

function handleTaskDeleted(store: StoreApi<AppState>, payload: TaskDeletedPayload): void {
  const deletedId = payload.task_id;
  if (!deletedId) return;

  const currentState = store.getState();
  const wasActive = currentState.tasks.activeTaskId === deletedId;
  // Capture the route match before any redirect mutates the pathname. This
  // covers a fresh load where the browser is parked on the task's route
  // (`/t/<id>`, `/tasks/<id>`, or `/office/tasks/<id>`) but TaskPageContent
  // hasn't hydrated `activeTaskId` yet, so `wasActive` is still false.
  const onDeletedRoute =
    typeof window !== "undefined" &&
    removedTaskRedirectHref(window.location.pathname, deletedId) !== null;
  cleanupRemovedTaskClientState(store, deletedId);

  // Only react to genuine auto-deletions, which the backend tags with a
  // reason (e.g. a review task whose PR was approved). User-initiated deletes
  // carry no reason: their local delete flow owns navigation by switching to
  // the next task, so redirecting here would preempt it.
  if (payload.reason && (wasActive || onDeletedRoute)) {
    redirectAwayFromRemovedTask(deletedId);
    store.getState().setTaskDeletedNotification({
      taskId: deletedId,
      title: payload.title,
      reason: payload.reason,
    });
  }
}

function handleTaskArchived(store: StoreApi<AppState>, payload: TaskUpdatedPayload): void {
  const archivedId = payload.task_id;
  if (!archivedId) return;

  const currentState = store.getState();
  const wasActive = currentState.tasks.activeTaskId === archivedId;
  const onArchivedRoute =
    typeof window !== "undefined" &&
    removedTaskRedirectHref(window.location.pathname, archivedId) !== null;

  cleanupRemovedTaskClientState(store, archivedId);

  if (wasActive || onArchivedRoute) {
    redirectAwayFromRemovedTask(archivedId);
  }
}

function cleanupRemovedTaskClientState(store: StoreApi<AppState>, taskId: string): void {
  removeRecentTask(taskId);

  const currentState = store.getState();
  const sessionIds = sessionIdsForDeletedTask(currentState, taskId);
  const envIds = environmentIdsForSessions(currentState, sessionIds);
  cleanupTaskStorage(taskId, sessionIds, envIds);
  currentState.removeTaskFromSidebarPrefs(taskId);
  for (const sid of sessionIds) {
    useContextFilesStore.getState().clearSession(sid);
  }

  store.setState((state) =>
    clearDeletedTaskWalkthroughClientState(cleanupRemovedTaskSelectionState(state, taskId), taskId),
  );
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

function cleanupRemovedTaskSelectionState(state: AppState, deletedId: string): AppState {
  let next = state;
  if (state.tasks.activeTaskId === deletedId) {
    next = {
      ...next,
      tasks: {
        ...next.tasks,
        activeTaskId: null,
        activeSessionId: null,
        pinnedSessionId: null,
      },
    };
  }
  if (next.tasks.lastSessionByTaskId[deletedId]) {
    const rest = { ...next.tasks.lastSessionByTaskId };
    delete rest[deletedId];
    next = { ...next, tasks: { ...next.tasks, lastSessionByTaskId: rest } };
  }
  return next;
}
