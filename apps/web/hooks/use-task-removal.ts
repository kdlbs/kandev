import { useCallback } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";
import { workflowSnapshotToKanbanData } from "@/lib/kanban/snapshot";
import { replaceTaskUrl } from "@/lib/links";
import { taskSessionsQueryOptions } from "@/lib/query/query-options";
import { taskSessionsAreNavigationReady } from "@/lib/session/task-session-navigation";
import {
  removeTasksFromWorkflowSnapshotQueries,
  workflowSnapshotQueryData,
} from "@/lib/query/workflow-snapshot-cache";
import { useRouter } from "@/lib/routing/client-router";
import { performLayoutSwitch } from "@/lib/state/dockview-store";
import { getRecentTasks } from "@/lib/recent-tasks";

type TaskRemovalOptions = {
  store: StoreApi<AppState>;
  /** Whether to call performLayoutSwitch when switching sessions (desktop sidebar uses this) */
  useLayoutSwitch?: boolean;
};

type RemoveFromBoardOptions = {
  /**
   * The active task ID captured **before** the async delete/archive API call.
   * Only honored when the current `activeTaskId` has been cleared to `null`
   * by the WS "task.deleted" / "task.updated(archived_at)" handler racing
   * ahead of this function. If the user has manually navigated to a different
   * task during the in-flight API call, the current store value wins and
   * this captured value is ignored.
   */
  wasActiveTaskId?: string | null;
  /** The active session ID captured before the async delete API call. */
  wasActiveSessionId?: string | null;
  /** Switch away from the task without removing it from board state yet. */
  switchOnly?: boolean;
};

type RemoveFromBoardResult = {
  switchedTaskId: string | null;
};

async function loadTaskSessionsForTaskFromStore(
  store: StoreApi<AppState>,
  taskId: string,
  queryClient: QueryClient,
): Promise<TaskSession[]> {
  const state = store.getState();
  const cachedSessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
  if (state.taskSessionsByTask.loadedByTaskId[taskId]) {
    if (taskSessionsAreNavigationReady(cachedSessions)) return cachedSessions;
  }
  // Do not return partial WS-seeded rows while a Query fetch is in flight.
  // fetchQuery dedupes against the active request and resolves with the
  // canonical API payload, including fields needed for layout decisions such
  // as is_passthrough.
  store.getState().setTaskSessionsLoading(taskId, true);
  try {
    const response = await queryClient.fetchQuery({
      ...taskSessionsQueryOptions(taskId),
      staleTime: 0,
    });
    store.getState().setTaskSessionsForTask(taskId, response.sessions ?? []);
    return response.sessions ?? [];
  } catch (error) {
    console.error("Failed to load task sessions:", error);
    store.getState().setTaskSessionsForTask(taskId, []);
    return [];
  } finally {
    store.getState().setTaskSessionsLoading(taskId, false);
  }
}

function removeTaskFromSnapshots(queryClient: QueryClient, taskId: string): void {
  removeTasksFromWorkflowSnapshotQueries(queryClient, new Set([taskId]));
}

function collectRemainingTasks(
  store: StoreApi<AppState>,
  queryClient: QueryClient,
): KanbanState["tasks"] {
  return workflowSnapshotQueryData(queryClient).flatMap(
    (snapshot) => workflowSnapshotToKanbanData(snapshot).tasks,
  );
}

function selectNextTaskAfterRemoval(
  remainingTasks: KanbanState["tasks"],
  removedTaskId: string,
): KanbanState["tasks"][number] | null {
  const remainingById = new Map(
    remainingTasks.filter((task) => task.id !== removedTaskId).map((task) => [task.id, task]),
  );
  for (const recent of getRecentTasks()) {
    const task = remainingById.get(recent.taskId);
    if (task) return task;
  }
  return remainingTasks.find((task) => task.id !== removedTaskId) ?? null;
}

function switchToSessionForTask(params: {
  store: StoreApi<AppState>;
  nextTask: KanbanState["tasks"][number];
  sessionId: string;
  oldEnvId: string | null;
  useLayoutSwitch: boolean;
}): void {
  const { store, nextTask, sessionId, oldEnvId, useLayoutSwitch } = params;
  store.getState().setActiveSession(nextTask.id, sessionId);
  if (!useLayoutSwitch) return;
  const state = store.getState();
  const newEnvId = state.environmentIdBySessionId[sessionId] ?? null;
  const sessionIds = (state.taskSessionsByTask.itemsByTaskId[nextTask.id] ?? []).map(
    (session) => session.id,
  );
  if (newEnvId) performLayoutSwitch(oldEnvId, newEnvId, sessionId, sessionIds);
}

async function switchToNextTask(params: {
  store: StoreApi<AppState>;
  nextTask: KanbanState["tasks"][number];
  oldEnvId: string | null;
  useLayoutSwitch: boolean;
  loadTaskSessionsForTask: (taskId: string) => Promise<TaskSession[]>;
}): Promise<void> {
  const { store, nextTask, oldEnvId, useLayoutSwitch, loadTaskSessionsForTask } = params;
  if (nextTask.primarySessionId) {
    if (useLayoutSwitch && !store.getState().environmentIdBySessionId[nextTask.primarySessionId]) {
      await loadTaskSessionsForTask(nextTask.id);
    }
    switchToSessionForTask({
      store,
      nextTask,
      sessionId: nextTask.primarySessionId,
      oldEnvId,
      useLayoutSwitch,
    });
    replaceTaskUrl(nextTask.id);
    return;
  }

  const sessions = await loadTaskSessionsForTask(nextTask.id);
  const sessionId = sessions[0]?.id ?? null;
  if (sessionId) {
    switchToSessionForTask({ store, nextTask, sessionId, oldEnvId, useLayoutSwitch });
  } else {
    store.getState().setActiveTask(nextTask.id);
  }
  replaceTaskUrl(nextTask.id);
}

function resolveOldEnvId(store: StoreApi<AppState>, opts?: RemoveFromBoardOptions): string | null {
  const oldSessionId =
    opts?.wasActiveSessionId !== undefined
      ? opts.wasActiveSessionId
      : store.getState().tasks.activeSessionId;
  return oldSessionId ? (store.getState().environmentIdBySessionId[oldSessionId] ?? null) : null;
}

/**
 * Decide whether the removed task is the one the user is currently viewing.
 *
 * Two cases count as "still on the removed task":
 *   1. `stillOnRemoved` — the store's current `activeTaskId` matches `taskId`.
 *   2. `wsCleared` — the store's `activeTaskId` has been cleared to `null`
 *      (the WS `task.deleted` / `task.updated(archived_at)` handler raced
 *      ahead of us) AND the caller-captured `wasActiveTaskId` matches `taskId`.
 *
 * Any other state means the user manually moved to a different task during
 * the in-flight API call — leave them on their chosen task.
 */
function shouldSwitchAfterRemoval(
  store: StoreApi<AppState>,
  taskId: string,
  opts?: RemoveFromBoardOptions,
): boolean {
  const currentActiveTaskId = store.getState().tasks.activeTaskId;
  const stillOnRemoved = currentActiveTaskId === taskId;
  const wsCleared = currentActiveTaskId === null && opts?.wasActiveTaskId === taskId;
  return stillOnRemoved || wsCleared;
}

/**
 * Hook that provides shared logic for removing a task from the kanban board
 * (after archive or delete) and switching to the next available task.
 *
 * Used by both TaskSessionSidebar and SessionTaskSwitcherSheet.
 */
export function useTaskRemoval({ store, useLayoutSwitch = false }: TaskRemovalOptions) {
  const queryClient = useQueryClient();
  const router = useRouter();
  const loadTaskSessionsForTask = useCallback(
    (taskId: string) => loadTaskSessionsForTaskFromStore(store, taskId, queryClient),
    [store, queryClient],
  );

  /**
   * Remove a task from the kanban board state (both single and multi snapshots)
   * and switch to the next available task if the removed task was active.
   *
   * Pass `opts.wasActiveTaskId` / `opts.wasActiveSessionId` when calling after
   * an async API call (e.g. deleteTaskById, archiveTask) — the WS handler may
   * clear activeTaskId before this function runs. The captured value is only
   * consulted as a fallback when the current store value has been cleared; if
   * the user manually navigated to a different task mid-flight, the store
   * wins and the captured value is ignored (no auto-switch).
   */
  const removeTaskFromBoard = useCallback(
    async (taskId: string, opts?: RemoveFromBoardOptions): Promise<RemoveFromBoardResult> => {
      if (!opts?.switchOnly) removeTaskFromSnapshots(queryClient, taskId);
      const allRemainingTasks = collectRemainingTasks(store, queryClient);

      if (!shouldSwitchAfterRemoval(store, taskId, opts)) {
        return { switchedTaskId: null };
      }

      const oldEnvId = resolveOldEnvId(store, opts);
      const nextTask = selectNextTaskAfterRemoval(allRemainingTasks, taskId);
      if (nextTask) {
        await switchToNextTask({
          store,
          nextTask,
          oldEnvId,
          useLayoutSwitch,
          loadTaskSessionsForTask,
        });
        return { switchedTaskId: nextTask.id };
      }

      router.replace("/");
      return { switchedTaskId: null };
    },
    [store, queryClient, router, useLayoutSwitch, loadTaskSessionsForTask],
  );

  return { removeTaskFromBoard, loadTaskSessionsForTask };
}
