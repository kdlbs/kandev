import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerTasksHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'task.created': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const exists = state.kanban.tasks.some((task) => task.id === message.payload.task_id);
        const nextTask = {
          id: message.payload.task_id,
          columnId: message.payload.column_id,
          title: message.payload.title,
          description: message.payload.description,
          position: message.payload.position ?? 0,
          state: message.payload.state,
          repositoryId: message.payload.repository_id,
        };
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: exists
              ? state.kanban.tasks.map((task) => (task.id === nextTask.id ? nextTask : task))
              : [...state.kanban.tasks, nextTask],
          },
        };
      });
    },
    'task.updated': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextTask = {
          id: message.payload.task_id,
          columnId: message.payload.column_id,
          title: message.payload.title,
          description: message.payload.description,
          position: message.payload.position ?? 0,
          state: message.payload.state,
          repositoryId: message.payload.repository_id,
        };
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.some((task) => task.id === nextTask.id)
              ? state.kanban.tasks.map((task) => (task.id === nextTask.id ? nextTask : task))
              : [...state.kanban.tasks, nextTask],
          },
        };
      });
    },
    'task.deleted': (message) => {
      store.setState((state) => ({
        ...state,
        kanban: {
          ...state.kanban,
          tasks: state.kanban.tasks.filter((task) => task.id !== message.payload.task_id),
        },
      }));
    },
    'task.state_changed': (message) => {
      console.log('[WS Router] task.state_changed received:', {
        task_id: message.payload.task_id,
        board_id: message.payload.board_id,
        column_id: message.payload.column_id,
        state: message.payload.state,
      });
      store.setState((state) => {
        console.log('[WS Router] Current board_id:', state.kanban.boardId, 'Event board_id:', message.payload.board_id);
        if (state.kanban.boardId !== message.payload.board_id) {
          console.log('[WS Router] Skipping - board_id mismatch');
          return state;
        }
        const existingTask = state.kanban.tasks.find((t) => t.id === message.payload.task_id);
        console.log('[WS Router] Existing task:', existingTask);
        const nextTask = {
          id: message.payload.task_id,
          columnId: message.payload.column_id,
          title: message.payload.title,
          description: message.payload.description,
          position: message.payload.position ?? 0,
          state: message.payload.state,
          repositoryId: message.payload.repository_id,
        };
        console.log('[WS Router] Next task:', nextTask);
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.some((task) => task.id === nextTask.id)
              ? state.kanban.tasks.map((task) => (task.id === nextTask.id ? nextTask : task))
              : [...state.kanban.tasks, nextTask],
          },
        };
      });
    },
  };
}
