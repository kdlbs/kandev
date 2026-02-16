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
          ...state.userSettings,
          // Preserve workspaceId and boardId — these are navigation state
          // controlled by SSR and explicit user actions, not WebSocket broadcasts.
          // Overwriting them here causes redirect loops when the broadcast
          // arrives with a stale workspace/board from a previous commit.
          repositoryIds,
          preferredShell: message.payload.preferred_shell || null,
          defaultEditorId: message.payload.default_editor_id || null,
          enablePreviewOnClick: message.payload.enable_preview_on_click ?? false,
          chatSubmitKey: (message.payload.chat_submit_key as 'enter' | 'cmd_enter') ?? 'cmd_enter',
          reviewAutoMarkOnScroll: message.payload.review_auto_mark_on_scroll ?? true,
          lspAutoStartLanguages: message.payload.lsp_auto_start_languages ?? [],
          lspAutoInstallLanguages: message.payload.lsp_auto_install_languages ?? [],
          loaded: true,
        },
      }));
    },
  };
}
