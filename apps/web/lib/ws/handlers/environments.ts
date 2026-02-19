import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { Environment } from "@/lib/types/http";

export function registerEnvironmentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "environment.created": (message) => {
      const payload = message.payload;
      const environment: Environment = {
        id: payload.id,
        name: payload.name,
        kind: payload.kind,
        is_system: payload.is_system,
        worktree_root: payload.worktree_root ?? null,
        image_tag: payload.image_tag ?? null,
        dockerfile: payload.dockerfile ?? null,
        build_config: payload.build_config ?? null,
        created_at: payload.created_at ?? new Date().toISOString(),
        updated_at: payload.updated_at ?? new Date().toISOString(),
      };
      store.setState((state) => ({
        ...state,
        environments: {
          items: [
            ...state.environments.items.filter((item) => item.id !== environment.id),
            environment,
          ],
        },
      }));
    },
    "environment.updated": (message) => {
      store.setState((state) => ({
        ...state,
        environments: {
          items: state.environments.items.map((item) =>
            item.id === message.payload.id ? { ...item, ...message.payload } : item,
          ),
        },
      }));
    },
    "environment.deleted": (message) => {
      store.setState((state) => ({
        ...state,
        environments: {
          items: state.environments.items.filter((item) => item.id !== message.payload.id),
        },
      }));
    },
  };
}
