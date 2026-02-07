import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { KanbanState } from '@/lib/state/slices/kanban/types';

type KanbanTask = KanbanState['tasks'][number];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildTaskFromPayload(payload: any, existing?: KanbanTask): KanbanTask {
  return {
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
      store.setState((state) => {
        const deletedId = message.payload.task_id;
        const isActive = state.tasks.activeTaskId === deletedId;
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.filter((task) => task.id !== deletedId),
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
