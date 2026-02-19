import type { StoreApi } from "zustand";
import type { AppState, WorkspaceState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type WorkspaceItem = WorkspaceState["items"][number];

export function registerWorkspacesHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "workspace.created": (message) => {
      store.setState((state) => {
        const payload = message.payload;
        const newWorkspace: WorkspaceItem = {
          id: payload.id,
          name: payload.name,
          description: payload.description ?? null,
          owner_id: payload.owner_id ?? "",
          default_executor_id: payload.default_executor_id ?? null,
          default_environment_id: payload.default_environment_id ?? null,
          default_agent_profile_id: payload.default_agent_profile_id ?? null,
          created_at: payload.created_at ?? new Date().toISOString(),
          updated_at: payload.updated_at ?? new Date().toISOString(),
        };
        const exists = state.workspaces.items.some((item) => item.id === payload.id);
        const items = exists
          ? state.workspaces.items.map((item) =>
              item.id === payload.id ? { ...item, ...newWorkspace } : item,
            )
          : [newWorkspace, ...state.workspaces.items];
        const activeId = state.workspaces.activeId ?? payload.id;
        return {
          ...state,
          workspaces: {
            items,
            activeId,
          },
        };
      });
    },
    "workspace.updated": (message) => {
      store.setState((state) => ({
        ...state,
        workspaces: {
          ...state.workspaces,
          items: state.workspaces.items.map((item) =>
            item.id === message.payload.id
              ? {
                  ...item,
                  name: message.payload.name,
                  description: message.payload.description ?? item.description,
                  default_executor_id: message.payload.default_executor_id ?? null,
                  default_environment_id: message.payload.default_environment_id ?? null,
                  default_agent_profile_id: message.payload.default_agent_profile_id ?? null,
                  updated_at: message.payload.updated_at ?? item.updated_at,
                }
              : item,
          ),
        },
      }));
    },
    "workspace.deleted": (message) => {
      store.setState((state) => {
        const items = state.workspaces.items.filter((item) => item.id !== message.payload.id);
        const activeId =
          state.workspaces.activeId === message.payload.id
            ? (items[0]?.id ?? null)
            : state.workspaces.activeId;
        const clearBoards = state.workspaces.activeId === message.payload.id;
        return {
          ...state,
          workspaces: {
            items,
            activeId,
          },
          workflows: clearBoards ? { items: [], activeId: null } : state.workflows,
          kanban: clearBoards ? { workflowId: null, steps: [], tasks: [] } : state.kanban,
        };
      });
    },
  };
}
