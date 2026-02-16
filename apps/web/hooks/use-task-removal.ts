import { useCallback } from 'react';
import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { KanbanState } from '@/lib/state/slices';
import { linkToSession } from '@/lib/links';
import { listTaskSessions } from '@/lib/api';
import { performLayoutSwitch } from '@/lib/state/dockview-store';

type TaskRemovalOptions = {
  store: StoreApi<AppState>;
  /** Whether to call performLayoutSwitch when switching sessions (desktop sidebar uses this) */
  useLayoutSwitch?: boolean;
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
        const response = await listTaskSessions(taskId, { cache: 'no-store' });
        store.getState().setTaskSessionsForTask(taskId, response.sessions ?? []);
        return response.sessions ?? [];
      } catch (error) {
        console.error('Failed to load task sessions:', error);
        store.getState().setTaskSessionsForTask(taskId, []);
        return [];
      } finally {
        store.getState().setTaskSessionsLoading(taskId, false);
      }
    },
    [store]
  );

  /**
   * Remove a task from the kanban board state (both single and multi snapshots)
   * and switch to the next available task if the removed task was active.
   */
  const removeTaskFromBoard = useCallback(
    async (taskId: string) => {
      const { setActiveSession, setActiveTask } = store.getState();

      // Remove task from multi snapshots
      const currentSnapshots = store.getState().kanbanMulti.snapshots;
      for (const [wfId, snapshot] of Object.entries(currentSnapshots)) {
        const hadTask = snapshot.tasks.some((t: KanbanState['tasks'][number]) => t.id === taskId);
        if (hadTask) {
          store.getState().setWorkflowSnapshot(wfId, {
            ...snapshot,
            tasks: snapshot.tasks.filter((t: KanbanState['tasks'][number]) => t.id !== taskId),
          });
        }
      }

      // Also update single kanban state
      const currentKanbanTasks = store.getState().kanban.tasks;
      if (currentKanbanTasks.some((t: KanbanState['tasks'][number]) => t.id === taskId)) {
        store.setState((state) => ({
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.filter((t: KanbanState['tasks'][number]) => t.id !== taskId),
          },
        }));
      }

      // Collect remaining tasks across snapshots
      const allRemainingTasks: KanbanState['tasks'] = [];
      for (const snapshot of Object.values(store.getState().kanbanMulti.snapshots)) {
        allRemainingTasks.push(...snapshot.tasks);
      }
      // Fallback to single kanban if no multi-snapshots
      if (allRemainingTasks.length === 0) {
        allRemainingTasks.push(...store.getState().kanban.tasks);
      }

      // If removed task was active, switch to another task or go home
      const currentActiveTaskId = store.getState().tasks.activeTaskId;
      if (currentActiveTaskId === taskId) {
        const oldSessionId = store.getState().tasks.activeSessionId;
        if (allRemainingTasks.length > 0) {
          const nextTask = allRemainingTasks[0];
          if (nextTask.primarySessionId) {
            setActiveSession(nextTask.id, nextTask.primarySessionId);
            if (useLayoutSwitch) performLayoutSwitch(oldSessionId, nextTask.primarySessionId);
            window.history.replaceState({}, '', linkToSession(nextTask.primarySessionId));
          } else {
            const sessions = await loadTaskSessionsForTask(nextTask.id);
            const sessionId = sessions[0]?.id ?? null;
            if (sessionId) {
              setActiveSession(nextTask.id, sessionId);
              if (useLayoutSwitch) performLayoutSwitch(oldSessionId, sessionId);
              window.history.replaceState({}, '', linkToSession(sessionId));
            } else {
              setActiveTask(nextTask.id);
            }
          }
        } else {
          window.location.href = '/';
        }
      }
    },
    [store, useLayoutSwitch, loadTaskSessionsForTask]
  );

  return { removeTaskFromBoard, loadTaskSessionsForTask };
}
