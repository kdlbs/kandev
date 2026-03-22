import { useCallback } from "react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices";
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

/**
 * Hook that provides shared logic for removing a task from the kanban board
 * (after archive or delete) and switching to the next available task.
 *
 * Used by both TaskSessionSidebar and SessionTaskSwitcherSheet.
 */
export function useTaskRemoval({ store, useLayoutSwitch = false }: TaskRemovalOptions) {
  const loadTaskSessionsForTask = useCallback(
    async (taskId: string) => {
      const state = store.getState();
      if (state.taskSessionsByTask.loadedByTaskId[taskId]) {
        return state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
      }
      if (state.taskSessionsByTask.loadingByTaskId[taskId]) {
        return state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
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
    },
    [store],
  );

  /** Remove a task from both multi and single kanban snapshots. */
  const removeTaskFromSnapshots = useCallback(
    (taskId: string) => {
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
    },
    [store],
  );

  /** Switch to the next available task after removal. */
  const switchToNextTask = useCallback(
    async (nextTask: KanbanState["tasks"][number], oldSessionId: string | null) => {
      const { setActiveSession, setActiveTask } = store.getState();

      if (nextTask.primarySessionId) {
        setActiveSession(nextTask.id, nextTask.primarySessionId);
        if (useLayoutSwitch) performLayoutSwitch(oldSessionId, nextTask.primarySessionId);
        replaceTaskUrl(nextTask.id);
        return;
      }

      const sessions = await loadTaskSessionsForTask(nextTask.id);
      const sessionId = sessions[0]?.id ?? null;
      if (sessionId) {
        setActiveSession(nextTask.id, sessionId);
        if (useLayoutSwitch) performLayoutSwitch(oldSessionId, sessionId);
      } else {
        setActiveTask(nextTask.id);
      }
      replaceTaskUrl(nextTask.id);
    },
    [store, useLayoutSwitch, loadTaskSessionsForTask],
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
      removeTaskFromSnapshots(taskId);

      // Collect remaining tasks across snapshots
      const allRemainingTasks: KanbanState["tasks"] = [];
      for (const snapshot of Object.values(store.getState().kanbanMulti.snapshots)) {
        allRemainingTasks.push(...snapshot.tasks);
      }
      if (allRemainingTasks.length === 0) {
        allRemainingTasks.push(...store.getState().kanban.tasks);
      }

      // Use the caller-provided active task ID (captured before the async API
      // call) to avoid the race with the WS handler that may have already
      // cleared it.  Fall back to the current store value for callers that
      // don't provide it (e.g. archive, which doesn't go through the API).
      const activeTaskId =
        opts?.wasActiveTaskId !== undefined
          ? opts.wasActiveTaskId
          : store.getState().tasks.activeTaskId;
      if (activeTaskId !== taskId) return;

      const oldSessionId =
        opts?.wasActiveSessionId !== undefined
          ? opts.wasActiveSessionId
          : store.getState().tasks.activeSessionId;
      if (allRemainingTasks.length > 0) {
        await switchToNextTask(allRemainingTasks[0], oldSessionId);
      } else {
        window.location.href = "/";
      }
    },
    [store, removeTaskFromSnapshots, switchToNextTask],
  );

  return { removeTaskFromBoard, loadTaskSessionsForTask };
}
