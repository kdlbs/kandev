import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

export function registerWorkflowsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "workflow.created": (message) => {
      store.setState((state) => {
        if (state.workspaces.activeId !== message.payload.workspace_id) {
          return state;
        }
        const exists = state.workflows.items.some((item) => item.id === message.payload.id);
        if (exists) {
          return state;
        }
        return {
          ...state,
          workflows: {
            items: [
              {
                id: message.payload.id,
                workspaceId: message.payload.workspace_id,
                name: message.payload.name,
              },
              ...state.workflows.items,
            ],
            activeId: state.workflows.activeId ?? message.payload.id,
          },
        };
      });
    },
    "workflow.updated": (message) => {
      store.setState((state) => ({
        ...state,
        workflows: {
          ...state.workflows,
          items: state.workflows.items.map((item) =>
            item.id === message.payload.id ? { ...item, name: message.payload.name } : item,
          ),
        },
      }));
    },
    "workflow.deleted": (message) => {
      store.setState((state) => {
        const items = state.workflows.items.filter((item) => item.id !== message.payload.id);
        const nextActiveId =
          state.workflows.activeId === message.payload.id
            ? (items[0]?.id ?? null)
            : state.workflows.activeId;
        return {
          ...state,
          workflows: {
            items,
            activeId: nextActiveId,
          },
          kanban:
            state.kanban.workflowId === message.payload.id
              ? { workflowId: nextActiveId, steps: [], tasks: [] }
              : state.kanban,
        };
      });
    },
  };
}
