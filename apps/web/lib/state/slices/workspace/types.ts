export type WorkspaceState = {
  activeId: string | null;
};

export type WorkspaceSliceState = {
  workspaces: WorkspaceState;
};

export type WorkspaceSliceActions = {
  setActiveWorkspace: (workspaceId: string | null) => void;
};

export type WorkspaceSlice = WorkspaceSliceState & WorkspaceSliceActions;
