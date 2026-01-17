import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerTerminalsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'terminal.output': (message) => {
      store.getState().setTerminalOutput(message.payload.terminalId, message.payload.data);
    },
    'session.shell.output': (message) => {
      const { session_id, type, data } = message.payload;
      if (!session_id) {
        return;
      }
      if (type === 'output' && data) {
        store.getState().appendShellOutput(session_id, data);
      } else if (type === 'exit') {
        // Shell exited - update status
        store.getState().setShellStatus(session_id, { available: false });
      }
    },
  };
}
