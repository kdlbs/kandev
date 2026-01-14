import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerAgentSessionHandlers(store: StoreApi<AppState>): WsHandlers {
  void store;
  return {};
}
