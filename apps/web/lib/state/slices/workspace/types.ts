export type WorkspaceState = {
  items: Array<{
    id: string;
    name: string;
    description?: string | null;
    owner_id: string;
    default_executor_id?: string | null;
    default_environment_id?: string | null;
    default_agent_profile_id?: string | null;
    default_config_agent_profile_id?: string | null;
    office_workflow_id?: string | null;
    created_at: string;
    updated_at: string;
  }>;
  activeId: string | null;
};

export type WorkspaceSliceState = {
  workspaces: WorkspaceState;
};

export type WorkspaceSliceActions = {
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState["items"]) => void;
};

export type WorkspaceSlice = WorkspaceSliceState & WorkspaceSliceActions;
