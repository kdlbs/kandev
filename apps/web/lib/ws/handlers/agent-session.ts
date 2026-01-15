import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { AgentSessionState } from '@/lib/types/http';

export function registerAgentSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'agent_session.state_changed': (message) => {
      const payload = message.payload;
      if (!payload?.task_id || !payload?.new_state) {
        return;
      }
      store.getState().setAgentSessionState(payload.task_id, payload.new_state as AgentSessionState);
    },
  };
}
