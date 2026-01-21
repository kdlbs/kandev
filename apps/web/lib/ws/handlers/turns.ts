import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerTurnsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'session.turn.started': (message) => {
      const payload = message.payload;
      if (!payload.session_id) {
        return;
      }
      store.getState().addTurn({
        id: payload.id,
        session_id: payload.session_id,
        task_id: payload.task_id,
        started_at: payload.started_at,
        completed_at: payload.completed_at,
        metadata: payload.metadata,
        created_at: payload.created_at,
        updated_at: payload.updated_at,
      });
    },
    'session.turn.completed': (message) => {
      const payload = message.payload;
      if (!payload.session_id || !payload.id) {
        return;
      }
      store.getState().completeTurn(
        payload.session_id,
        payload.id,
        payload.completed_at || new Date().toISOString()
      );
    },
  };
}

