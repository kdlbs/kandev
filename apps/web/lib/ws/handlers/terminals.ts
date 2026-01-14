import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerTerminalsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'terminal.output': (message) => {
      store.getState().setTerminalOutput(message.payload.terminalId, message.payload.data);
    },
  };
}
