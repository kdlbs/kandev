import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerGitStatusHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'git.status': (message) => {
      const payload = message.payload;
      console.log('[WS] git.status received:', {
        task_id: payload.task_id,
        branch: payload.branch,
        modified: payload.modified.length,
        added: payload.added.length,
        deleted: payload.deleted.length,
        untracked: payload.untracked.length,
      });
      const state = store.getState();

      // Only update git status if it's for the current task
      // This prevents stale data from showing when switching tasks
      if (state.tasks.activeTaskId !== payload.task_id) {
        console.log('[WS] Ignoring git.status for different task:', {
          current_task: state.tasks.activeTaskId,
          received_task: payload.task_id,
        });
        return;
      }

      state.setGitStatus(payload.task_id, {
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
