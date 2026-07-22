import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerGitLabHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "gitlab.task_mr.updated": (message) => {
      const mr = message.payload;
      const activeWorkspaceId = store.getState().workspaces.activeId;
      if (!mr.task_id || !mr.workspace_id || mr.workspace_id !== activeWorkspaceId) return;
      store.getState().setTaskMR(mr.workspace_id, mr.task_id, mr);
    },
  };
}
