import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { KanbanState } from '@/lib/state/slices/kanban/types';
import { cleanupTaskStorage } from '@/lib/local-storage';
import { useContextFilesStore } from '@/lib/state/context-files-store';

type KanbanTask = KanbanState['tasks'][number];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildTaskFromPayload(payload: any, existing?: KanbanTask): KanbanTask {
  const task = {
    id: payload.task_id,
    workflowStepId: payload.workflow_step_id,
    title: payload.title,
    description: payload.description,
    position: payload.position ?? 0,
    state: payload.state,
    repositoryId: payload.repository_id ?? existing?.repositoryId,
    primarySessionId: payload.primary_session_id ?? existing?.primarySessionId,
    sessionCount: payload.session_count ?? existing?.sessionCount,
    reviewStatus: payload.review_status ?? existing?.reviewStatus,
    updatedAt: payload.updated_at ?? existing?.updatedAt,
  };
  return task;
}

function upsertTask(tasks: KanbanTask[], nextTask: KanbanTask): KanbanTask[] {
  const exists = tasks.some((task) => task.id === nextTask.id);
  return exists
    ? tasks.map((task) => (task.id === nextTask.id ? nextTask : task))
    : [...tasks, nextTask];
}

export function registerTasksHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'task.created': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const existing = state.kanban.tasks.find((task) => task.id === message.payload.task_id);
        const nextTask = buildTaskFromPayload(message.payload, existing);
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: upsertTask(state.kanban.tasks, nextTask),
          },
        };
      });
    },
    'task.updated': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const existing = state.kanban.tasks.find((task) => task.id === message.payload.task_id);
        const nextTask = buildTaskFromPayload(message.payload, existing);
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: upsertTask(state.kanban.tasks, nextTask),
          },
        };
      });
    },
    'task.deleted': (message) => {
      const deletedId = message.payload.task_id;

      // Clean up persisted storage before removing from state
      const currentState = store.getState();
      const sessionIds = (currentState.taskSessionsByTask.itemsByTaskId[deletedId] ?? []).map((s) => s.id);
      // Also include primarySessionId in case sessions weren't loaded into the store
      const task = currentState.kanban.tasks.find((t) => t.id === deletedId);
      if (task?.primarySessionId && !sessionIds.includes(task.primarySessionId)) {
        sessionIds.push(task.primarySessionId);
      }
      cleanupTaskStorage(deletedId, sessionIds);
      for (const sid of sessionIds) {
        useContextFilesStore.getState().clearSession(sid);
      }

      store.setState((state) => {
        const isActive = state.tasks.activeTaskId === deletedId;
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.filter((t) => t.id !== deletedId),
          },
          tasks: isActive
            ? { ...state.tasks, activeTaskId: null, activeSessionId: null }
            : state.tasks,
        };
      });
    },
    'task.state_changed': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const existing = state.kanban.tasks.find((t) => t.id === message.payload.task_id);
        const nextTask = buildTaskFromPayload(message.payload, existing);
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: upsertTask(state.kanban.tasks, nextTask),
          },
        };
      });
    },
  };
}
