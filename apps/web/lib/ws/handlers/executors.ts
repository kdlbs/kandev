import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerExecutorsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'executor.created': (message) => {
      store.setState((state) => ({
        ...state,
        executors: {
          items: [
            ...state.executors.items.filter((item) => item.id !== message.payload.id),
            message.payload,
          ],
        },
      }));
    },
    'executor.updated': (message) => {
      store.setState((state) => ({
        ...state,
        executors: {
          items: state.executors.items.map((item) =>
            item.id === message.payload.id ? { ...item, ...message.payload } : item
          ),
        },
      }));
    },
    'executor.deleted': (message) => {
      store.setState((state) => ({
        ...state,
        executors: {
          items: state.executors.items.filter((item) => item.id !== message.payload.id),
        },
      }));
    },
  };
}
