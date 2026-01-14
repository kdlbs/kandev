import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerKanbanHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'kanban.update': (message) => {
      store.setState((state) => ({
        ...state,
        kanban: {
          boardId: message.payload.boardId,
          columns: message.payload.columns.map((column, index) => ({
            id: column.id,
            title: column.title,
            color: column.color ?? 'bg-neutral-400',
            position: column.position ?? index,
          })),
          tasks: message.payload.tasks.map((task) => ({
            id: task.id,
            columnId: task.columnId,
            title: task.title,
            description: task.description,
            position: task.position ?? 0,
            state: task.state,
          })),
        },
      }));
    },
  };
}
