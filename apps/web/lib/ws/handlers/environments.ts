import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerEnvironmentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'environment.created': (message) => {
      store.setState((state) => ({
        ...state,
        environments: {
          items: [
            ...state.environments.items.filter((item) => item.id !== message.payload.id),
            message.payload,
          ],
        },
      }));
    },
    'environment.updated': (message) => {
      store.setState((state) => ({
        ...state,
        environments: {
          items: state.environments.items.map((item) =>
            item.id === message.payload.id ? { ...item, ...message.payload } : item
          ),
        },
      }));
    },
    'environment.deleted': (message) => {
      store.setState((state) => ({
        ...state,
        environments: {
          items: state.environments.items.filter((item) => item.id !== message.payload.id),
        },
      }));
    },
  };
}
