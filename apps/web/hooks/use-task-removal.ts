import { useCallback } from "react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";
import { replaceTaskUrl } from "@/lib/links";
import { listTaskSessions } from "@/lib/api";
import { performLayoutSwitch } from "@/lib/state/dockview-store";

type TaskRemovalOptions = {
  store: StoreApi<AppState>;
  /** Whether to call performLayoutSwitch when switching sessions (desktop sidebar uses this) */
  useLayoutSwitch?: boolean;
};

type RemoveFromBoardOptions = {
  /**
   * The active task ID captured **before** the async delete API call.
   * Avoids a race with the WS "task.deleted" handler that may clear
   * activeTaskId before removeTaskFromBoard runs.
   */
  wasActiveTaskId?: string | null;
  /** The active session ID captured before the async delete API call. */
  wasActiveSessionId?: string | null;
};

function cachedSessionsHaveEnvIds(sessions: TaskSession[]): boolean {
  return sessions.length === 0 || sessions.every((session) => !!session.task_environment_id);
}

async function loadTaskSessionsForTaskFromStore(
  store: StoreApi<AppState>,
  taskId: string,
): Promise<TaskSession[]> {
  const state = store.getState();
  const cachedSessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
  if (state.taskSessionsByTask.loadedByTaskId[taskId]) {
    if (cachedSessionsHaveEnvIds(cachedSessions)) return cachedSessions;
  }
  if (state.taskSessionsByTask.loadingByTaskId[taskId]) {
    return cachedSessions;
  }
  store.getState().setTaskSessionsLoading(taskId, true);
  try {
    const response = await listTaskSessions(taskId, { cache: "no-store" });
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

function removeTaskFromSnapshots(store: StoreApi<AppState>, taskId: string): void {
  const currentSnapshots = store.getState().kanbanMulti.snapshots;
  for (const [wfId, snapshot] of Object.entries(currentSnapshots)) {
    const hadTask = snapshot.tasks.some((t: KanbanState["tasks"][number]) => t.id === taskId);
    if (hadTask) {
      store.getState().setWorkflowSnapshot(wfId, {
        ...snapshot,
        tasks: snapshot.tasks.filter((t: KanbanState["tasks"][number]) => t.id !== taskId),
      });
    }
  }

  const currentKanbanTasks = store.getState().kanban.tasks;
  if (currentKanbanTasks.some((t: KanbanState["tasks"][number]) => t.id === taskId)) {
    store.setState((state) => ({
      ...state,
      kanban: {
        ...state.kanban,
        tasks: state.kanban.tasks.filter((t: KanbanState["tasks"][number]) => t.id !== taskId),
      },
    }));
  }
}

function collectRemainingTasks(store: StoreApi<AppState>): KanbanState["tasks"] {
  const allRemainingTasks: KanbanState["tasks"] = [];
  for (const snapshot of Object.values(store.getState().kanbanMulti.snapshots)) {
    allRemainingTasks.push(...snapshot.tasks);
  }
  if (allRemainingTasks.length === 0) {
    allRemainingTasks.push(...store.getState().kanban.tasks);
  }
  return allRemainingTasks;
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
  const newEnvId = store.getState().environmentIdBySessionId[sessionId] ?? null;
  if (newEnvId) performLayoutSwitch(oldEnvId, newEnvId, sessionId);
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

function resolveActiveTaskId(
  store: StoreApi<AppState>,
  opts?: RemoveFromBoardOptions,
): string | null {
  return opts?.wasActiveTaskId !== undefined
    ? opts.wasActiveTaskId
    : store.getState().tasks.activeTaskId;
}

/**
 * Hook that provides shared logic for removing a task from the kanban board
 * (after archive or delete) and switching to the next available task.
 *
 * Used by both TaskSessionSidebar and SessionTaskSwitcherSheet.
 */
export function useTaskRemoval({ store, useLayoutSwitch = false }: TaskRemovalOptions) {
  const loadTaskSessionsForTask = useCallback(
    (taskId: string) => loadTaskSessionsForTaskFromStore(store, taskId),
    [store],
  );

  /**
   * Remove a task from the kanban board state (both single and multi snapshots)
   * and switch to the next available task if the removed task was active.
   *
   * Pass `opts.wasActiveTaskId` / `opts.wasActiveSessionId` when calling after
   * an async API call (e.g. deleteTaskById) — the WS "task.deleted" handler may
   * clear activeTaskId before this function runs.
   */
  const removeTaskFromBoard = useCallback(
    async (taskId: string, opts?: RemoveFromBoardOptions) => {
      removeTaskFromSnapshots(store, taskId);
      const allRemainingTasks = collectRemainingTasks(store);

      // Use the caller-provided active task ID (captured before the async API
      // call) to avoid the race with the WS handler that may have already
      // cleared it.  Fall back to the current store value for callers that
      // don't provide it (e.g. archive, which doesn't go through the API).
      const activeTaskId = resolveActiveTaskId(store, opts);
      if (activeTaskId !== taskId) return;

      const oldEnvId = resolveOldEnvId(store, opts);
      if (allRemainingTasks.length > 0) {
        await switchToNextTask({
          store,
          nextTask: allRemainingTasks[0],
          oldEnvId,
          useLayoutSwitch,
          loadTaskSessionsForTask,
        });
      } else {
        window.location.href = "/";
      }
    },
    [store, useLayoutSwitch, loadTaskSessionsForTask],
  );

  return { removeTaskFromBoard, loadTaskSessionsForTask };
}
