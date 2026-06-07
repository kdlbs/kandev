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
  const loadTaskSessionsForTask = useCallback(
    (taskId: string) => loadTaskSessionsForTaskFromStore(store, taskId),
    [store],
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
    async (taskId: string, opts?: RemoveFromBoardOptions) => {
      removeTaskFromSnapshots(store, taskId);
      const allRemainingTasks = collectRemainingTasks(store);

      if (!shouldSwitchAfterRemoval(store, taskId, opts)) return;

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
