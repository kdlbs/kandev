import type { StoreApi } from "zustand";
import type { AppState, WorkspaceState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type WorkspaceItem = WorkspaceState["items"][number];

function handleWorkspaceDeleted(store: StoreApi<AppState>, workspaceId: string): void {
  const currentState = store.getState();
  const workspaceTaskIds = new Set(
    [
      ...currentState.kanban.tasks,
      ...Object.values(currentState.kanbanMulti.snapshots).flatMap((snapshot) => snapshot.tasks),
    ]
      .filter((task) => task.workspaceId === workspaceId)
      .map((task) => task.id),
  );
  const sessionIds = Object.values(currentState.taskSessions.items)
    .filter((session) => workspaceTaskIds.has(session.task_id))
    .map((session) => session.id);
  for (const sessionId of sessionIds) {
    currentState.clearQueueStatus(sessionId);
  }
  store.setState((state) => {
    const items = state.workspaces.items.filter((item) => item.id !== workspaceId);
    const activeId =
      state.workspaces.activeId === workspaceId
        ? (items[0]?.id ?? null)
        : state.workspaces.activeId;
    const clearBoards = state.workspaces.activeId === workspaceId;
    return {
      ...state,
      workspaces: { items, activeId },
      workflows: clearBoards ? { items: [], activeId: null } : state.workflows,
      kanban: clearBoards ? { workflowId: null, steps: [], tasks: [] } : state.kanban,
    };
  });
}

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
          default_config_agent_profile_id: payload.default_config_agent_profile_id ?? null,
          unit_id: payload.unit_id ?? "",
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
                  default_config_agent_profile_id:
                    "default_config_agent_profile_id" in message.payload
                      ? (message.payload.default_config_agent_profile_id ?? null)
                      : (item.default_config_agent_profile_id ?? null),
                  // Placement decides who reaches this workspace, so a move
                  // made in another tab has to land here. Presence-checked
                  // like default_config_agent_profile_id above: an older
                  // backend omits the key entirely, and reading it as ""
                  // would silently unplace the workspace.
                  unit_id:
                    "unit_id" in message.payload
                      ? (message.payload.unit_id ?? "")
                      : (item.unit_id ?? ""),
                  updated_at: message.payload.updated_at ?? item.updated_at,
                }
              : item,
          ),
        },
      }));
    },
    "workspace.deleted": (message) => handleWorkspaceDeleted(store, message.payload.id),
  };
}
