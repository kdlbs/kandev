import type { StoreApi } from "zustand";
import type { AppState, WorkspaceState } from "@/lib/state/store";
import type { WorkspacePayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type WorkspaceItem = WorkspaceState["items"][number];

function workspacePlacementUpdate(
  item: WorkspaceItem,
  payload: WorkspacePayload,
): Pick<WorkspaceItem, "default_config_agent_profile_id" | "unit_id"> {
  return {
    default_config_agent_profile_id:
      "default_config_agent_profile_id" in payload
        ? (payload.default_config_agent_profile_id ?? null)
        : (item.default_config_agent_profile_id ?? null),
    unit_id: "unit_id" in payload ? (payload.unit_id ?? "") : (item.unit_id ?? ""),
  };
}

function workspaceIdlePolicyUpdate(
  item: WorkspaceItem,
  payload: WorkspacePayload,
): Pick<WorkspaceItem, "acp_idle_suspension_enabled" | "acp_idle_timeout_minutes"> {
  return {
    acp_idle_suspension_enabled:
      "acp_idle_suspension_enabled" in payload
        ? (payload.acp_idle_suspension_enabled ?? false)
        : item.acp_idle_suspension_enabled,
    acp_idle_timeout_minutes:
      "acp_idle_timeout_minutes" in payload
        ? (payload.acp_idle_timeout_minutes ?? 120)
        : item.acp_idle_timeout_minutes,
  };
}

function updateWorkspaceItem(item: WorkspaceItem, payload: WorkspacePayload): WorkspaceItem {
  return {
    ...item,
    name: payload.name,
    description: payload.description ?? item.description,
    default_executor_id: payload.default_executor_id ?? null,
    default_environment_id: payload.default_environment_id ?? null,
    default_agent_profile_id: payload.default_agent_profile_id ?? null,
    ...workspacePlacementUpdate(item, payload),
    ...workspaceIdlePolicyUpdate(item, payload),
    updated_at: payload.updated_at ?? item.updated_at,
  };
}

function nextActiveWorkspaceRevision(workspaces: WorkspaceState, activeId: string | null): number {
  const revision = workspaces.activeIdRevision ?? 0;
  return workspaces.activeId === activeId ? revision : revision + 1;
}

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
    const activeIdRevision = nextActiveWorkspaceRevision(state.workspaces, activeId);
    const clearBoards = state.workspaces.activeId === workspaceId;
    return {
      ...state,
      workspaces: { items, activeId, activeIdRevision },
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
          acp_idle_suspension_enabled: payload.acp_idle_suspension_enabled ?? false,
          acp_idle_timeout_minutes: payload.acp_idle_timeout_minutes ?? 120,
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
        const activeIdRevision = nextActiveWorkspaceRevision(state.workspaces, activeId);
        return {
          ...state,
          workspaces: {
            items,
            activeId,
            activeIdRevision,
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
            item.id === message.payload.id ? updateWorkspaceItem(item, message.payload) : item,
          ),
        },
      }));
    },
    "workspace.deleted": (message) => handleWorkspaceDeleted(store, message.payload.id),
  };
}
