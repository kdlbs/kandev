import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerBoardsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'board.created': (message) => {
      store.setState((state) => {
        if (state.workspaces.activeId !== message.payload.workspace_id) {
          return state;
        }
        const exists = state.boards.items.some((item) => item.id === message.payload.id);
        if (exists) {
          return state;
        }
        return {
          ...state,
          boards: {
            items: [
              {
                id: message.payload.id,
                workspaceId: message.payload.workspace_id,
                name: message.payload.name,
              },
              ...state.boards.items,
            ],
            activeId: state.boards.activeId ?? message.payload.id,
          },
        };
      });
    },
    'board.updated': (message) => {
      store.setState((state) => ({
        ...state,
        boards: {
          ...state.boards,
          items: state.boards.items.map((item) =>
            item.id === message.payload.id ? { ...item, name: message.payload.name } : item
          ),
        },
      }));
    },
    'board.deleted': (message) => {
      store.setState((state) => {
        const items = state.boards.items.filter((item) => item.id !== message.payload.id);
        const nextActiveId =
          state.boards.activeId === message.payload.id ? items[0]?.id ?? null : state.boards.activeId;
        return {
          ...state,
          boards: {
            items,
            activeId: nextActiveId,
          },
          kanban:
            state.kanban.boardId === message.payload.id
              ? { boardId: nextActiveId, columns: [], tasks: [] }
              : state.kanban,
        };
      });
    },
  };
}
