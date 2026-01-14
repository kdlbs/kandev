import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerColumnsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'column.created': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextColumns = [
          ...state.kanban.columns,
          {
            id: message.payload.id,
            title: message.payload.name,
            color: message.payload.color,
            position: message.payload.position,
          },
        ].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
        return {
          ...state,
          kanban: {
            ...state.kanban,
            columns: nextColumns,
          },
        };
      });
    },
    'column.updated': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const exists = state.kanban.columns.some((column) => column.id === message.payload.id);
        const updatedColumns = exists
          ? state.kanban.columns.map((column) =>
              column.id === message.payload.id
                ? {
                    ...column,
                    title: message.payload.name,
                    color: message.payload.color,
                    position: message.payload.position,
                  }
                : column
            )
          : [
              ...state.kanban.columns,
              {
                id: message.payload.id,
                title: message.payload.name,
                color: message.payload.color,
                position: message.payload.position,
              },
            ];
        const nextColumns = updatedColumns.sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
        return {
          ...state,
          kanban: {
            ...state.kanban,
            columns: nextColumns,
          },
        };
      });
    },
    'column.deleted': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextColumns = state.kanban.columns.filter((column) => column.id !== message.payload.id);
        return {
          ...state,
          kanban: {
            ...state.kanban,
            columns: nextColumns,
          },
        };
      });
    },
  };
}
