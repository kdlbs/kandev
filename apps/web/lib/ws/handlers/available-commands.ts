import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { AvailableCommand } from '@/lib/state/slices/session-runtime/types';

export function registerAvailableCommandsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'session.available_commands': (message) => {
      const payload = message.payload;
      if (!payload?.session_id) {
        return;
      }
      const sessionId = payload.session_id as string;
      const commands = (payload.available_commands || []) as AvailableCommand[];

      store.getState().setAvailableCommands(sessionId, commands);
    },
  };
}
