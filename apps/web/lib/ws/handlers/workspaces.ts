import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerWorkspacesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'workspace.created': (message) => {
      store.setState((state) => {
        const exists = state.workspaces.items.some((item) => item.id === message.payload.id);
        const items = exists
          ? state.workspaces.items.map((item) =>
              item.id === message.payload.id
                ? {
                    ...item,
                    name: message.payload.name,
                    default_executor_id: message.payload.default_executor_id ?? null,
                    default_environment_id: message.payload.default_environment_id ?? null,
                    default_agent_profile_id: message.payload.default_agent_profile_id ?? null,
                  }
                : item
            )
          : [
              {
                id: message.payload.id,
                name: message.payload.name,
                default_executor_id: message.payload.default_executor_id ?? null,
                default_environment_id: message.payload.default_environment_id ?? null,
                default_agent_profile_id: message.payload.default_agent_profile_id ?? null,
              },
              ...state.workspaces.items,
            ];
        const activeId = state.workspaces.activeId ?? message.payload.id;
        return {
          ...state,
          workspaces: {
            items,
            activeId,
          },
        };
      });
    },
    'workspace.updated': (message) => {
      store.setState((state) => ({
        ...state,
        workspaces: {
          ...state.workspaces,
          items: state.workspaces.items.map((item) =>
            item.id === message.payload.id
              ? {
                  ...item,
                  name: message.payload.name,
                  default_executor_id: message.payload.default_executor_id ?? null,
                  default_environment_id: message.payload.default_environment_id ?? null,
                  default_agent_profile_id: message.payload.default_agent_profile_id ?? null,
                }
              : item
          ),
        },
      }));
    },
    'workspace.deleted': (message) => {
      store.setState((state) => {
        const items = state.workspaces.items.filter((item) => item.id !== message.payload.id);
        const activeId =
          state.workspaces.activeId === message.payload.id ? items[0]?.id ?? null : state.workspaces.activeId;
        const clearBoards = state.workspaces.activeId === message.payload.id;
        return {
          ...state,
          workspaces: {
            items,
            activeId,
          },
          boards: clearBoards ? { items: [], activeId: null } : state.boards,
          kanban: clearBoards ? { boardId: null, columns: [], tasks: [] } : state.kanban,
        };
      });
    },
  };
}
