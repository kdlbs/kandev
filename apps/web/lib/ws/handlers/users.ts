import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerUsersHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'user.settings.updated': (message) => {
      const repositoryIds = Array.from(new Set(message.payload.repository_ids ?? [])).sort();
      store.setState((state) => ({
        ...state,
        userSettings: {
          workspaceId: message.payload.workspace_id || null,
          boardId: message.payload.board_id || null,
          repositoryIds,
          preferredShell: message.payload.preferred_shell || null,
          defaultEditorId: message.payload.default_editor_id || null,
          loaded: true,
        },
      }));
    },
  };
}
