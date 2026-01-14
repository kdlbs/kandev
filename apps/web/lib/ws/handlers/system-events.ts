import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerSystemEventsHandlers(store: StoreApi<AppState>): WsHandlers {
  void store;
  return {
    'system.error': () => {
      // TODO: surface as toast/notification once UI is ready.
    },
  };
}
