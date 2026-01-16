import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerTerminalsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'terminal.output': (message) => {
      store.getState().setTerminalOutput(message.payload.terminalId, message.payload.data);
    },
    'shell.output': (message) => {
      const { task_id, type, data } = message.payload;
      if (type === 'output' && data) {
        store.getState().appendShellOutput(task_id, data);
      } else if (type === 'exit') {
        // Shell exited - update status
        store.getState().setShellStatus(task_id, { available: false });
      }
    },
  };
}
