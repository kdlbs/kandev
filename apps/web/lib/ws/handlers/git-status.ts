import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { GitSnapshot, SessionCommit } from '@/lib/state/slices/session-runtime/types';

export function registerGitStatusHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'session.git.status': (message) => {
      const payload = message.payload;
      if (!payload.session_id) {
        return;
      }
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
    'session.git.snapshot': (message) => {
      const payload = message.payload;
      if (!payload.session_id || !payload.snapshot) {
        return;
      }
      store.getState().addGitSnapshot(payload.session_id, payload.snapshot as GitSnapshot);
    },
    'session.git.commit': (message) => {
      const payload = message.payload;
      if (!payload.session_id || !payload.commit) {
        return;
      }
      store.getState().addSessionCommit(payload.session_id, payload.commit as SessionCommit);
    },
  };
}
