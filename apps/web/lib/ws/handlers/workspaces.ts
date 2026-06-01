import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/**
 * Workspace server data (the list, repositories, branches, scripts) now lives
 * in the TanStack Query cache — see `lib/query/bridge/workspace.ts` for the
 * WS→TQ writes and `hooks/domains/workspace/` for the reads.
 *
 * This Zustand handler only retains the CLIENT-STATE side effects that the TQ
 * bridge does not own: when the *active* workspace is deleted, re-point the
 * active selection and clear the (still-Zustand) kanban / workflows board view
 * so it doesn't show a deleted workspace's board.
 */
export function registerWorkspacesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "workspace.deleted": (message) => {
      store.setState((state) => {
        if (state.workspaces.activeId !== message.payload.id) return state;
        return {
          ...state,
          workspaces: { activeId: null },
          workflows: { items: [], activeId: null },
          kanban: { workflowId: null, steps: [], tasks: [] },
        };
      });
    },
  };
}
