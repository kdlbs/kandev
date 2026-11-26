import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "./types";
import { pluginRegistry } from "@/lib/plugins/registry";
import { refreshSidebarGroupingCatalog } from "@/lib/plugins/sidebar-grouping-catalog";

export function registerSidebarGroupingHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "plugin.sidebar-grouping.updated": ({ payload }) => {
      if (payload.catalog) {
        if (payload.plugin_id) pluginRegistry.invalidateSidebarGroupings(payload.plugin_id);
        void refreshSidebarGroupingCatalog(store);
        return;
      }
      if (
        !payload.plugin_id ||
        (payload.workspace_id && payload.workspace_id !== store.getState().workspaces.activeId)
      )
        return;
      pluginRegistry.invalidateSidebarGroupings(payload.plugin_id, payload.workspace_id);
    },
  };
}
