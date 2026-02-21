import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { ExecutorProfilePayload } from "@/lib/types/backend";
import type { ExecutorProfile } from "@/lib/types/http";

function toProfile(payload: ExecutorProfilePayload): ExecutorProfile {
  return {
    id: payload.id,
    executor_id: payload.executor_id,
    name: payload.name,
    mcp_policy: payload.mcp_policy,
    config: payload.config,
    prepare_script: payload.prepare_script,
    cleanup_script: payload.cleanup_script,
    created_at: payload.created_at ?? new Date().toISOString(),
    updated_at: payload.updated_at ?? new Date().toISOString(),
  };
}

export function registerExecutorProfileHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "executor.profile.created": (message) => {
      const profile = toProfile(message.payload as ExecutorProfilePayload);
      store.setState((state) => ({
        ...state,
        executors: {
          items: state.executors.items.map((exec) =>
            exec.id === profile.executor_id
              ? { ...exec, profiles: [...(exec.profiles ?? []), profile] }
              : exec,
          ),
        },
      }));
    },
    "executor.profile.updated": (message) => {
      const profile = toProfile(message.payload as ExecutorProfilePayload);
      store.setState((state) => ({
        ...state,
        executors: {
          items: state.executors.items.map((exec) =>
            exec.id === profile.executor_id
              ? {
                  ...exec,
                  profiles: (exec.profiles ?? []).map((p) => (p.id === profile.id ? profile : p)),
                }
              : exec,
          ),
        },
      }));
    },
    "executor.profile.deleted": (message) => {
      const { id } = message.payload as { id: string };
      store.setState((state) => ({
        ...state,
        executors: {
          items: state.executors.items.map((exec) => ({
            ...exec,
            profiles: (exec.profiles ?? []).filter((p) => p.id !== id),
          })),
        },
      }));
    },
  };
}
