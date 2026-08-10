import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { ForgejoConfig } from "@/lib/types/forgejo";

export function registerForgejoHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "forgejo.config.updated": (message) => {
      const config = message.payload as ForgejoConfig;
      const workspaceId = config?.workspace_id;
      if (!workspaceId || store.getState().workspaces.activeId !== workspaceId) return;
      store.getState().setForgejoConfigState(workspaceId, config.origin ? config : null);
    },
  };
}
