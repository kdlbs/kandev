import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { Executor } from "@/lib/types/http";

export function registerExecutorsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "executor.created": (message) => {
      const payload = message.payload;
      const executor: Executor = {
        id: payload.id,
        name: payload.name,
        type: payload.type,
        status: payload.status,
        is_system: payload.is_system,
        config: payload.config,
        created_at: payload.created_at ?? new Date().toISOString(),
        updated_at: payload.updated_at ?? new Date().toISOString(),
      };
      store.setState((state) => ({
        ...state,
        executors: {
          items: [...state.executors.items.filter((item) => item.id !== executor.id), executor],
        },
      }));
    },
    "executor.updated": (message) => {
      store.setState((state) => ({
        ...state,
        executors: {
          items: state.executors.items.map((item) =>
            item.id === message.payload.id ? { ...item, ...message.payload } : item,
          ),
        },
      }));
    },
    "executor.deleted": (message) => {
      store.setState((state) => ({
        ...state,
        executors: {
          items: state.executors.items.filter((item) => item.id !== message.payload.id),
        },
      }));
    },
  };
}
