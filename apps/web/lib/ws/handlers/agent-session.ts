import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { TaskSessionState } from '@/lib/types/http';

export function registerTaskSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'task_session.state_changed': (message) => {
      const payload = message.payload;
      if (!payload?.task_id || !payload?.new_state) {
        return;
      }
      store.getState().setTaskSessionState(payload.task_id, payload.new_state as TaskSessionState);
    },
  };
}
