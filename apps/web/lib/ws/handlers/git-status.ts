import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerGitStatusHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'session.git.status': (message) => {
      const payload = message.payload;
      if (!payload.session_id) {
        return;
      }
      console.log('[WS] git.status received:', {
        session_id: payload.session_id,
        branch: payload.branch,
        modified: payload.modified.length,
        added: payload.added.length,
        deleted: payload.deleted.length,
        untracked: payload.untracked.length,
      });
      store.getState().setGitStatus(payload.session_id, {
        branch: payload.branch,
        remote_branch: payload.remote_branch ?? null,
        modified: payload.modified,
        added: payload.added,
        deleted: payload.deleted,
        untracked: payload.untracked,
        renamed: payload.renamed,
        ahead: payload.ahead,
        behind: payload.behind,
        files: payload.files,
        timestamp: payload.timestamp,
      });
    },
  };
}
