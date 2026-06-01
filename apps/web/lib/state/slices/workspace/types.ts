/**
 * The workspace slice is now CLIENT-ONLY: it tracks the active workspace
 * selection. All server data (workspaces list, repositories, repository
 * branches, repository scripts) was migrated to TanStack Query — read it via
 * the hooks in `hooks/domains/workspace/` (useWorkspaces, useRepositories,
 * useBranches, useRepositoryScripts) which select from the TQ cache.
 */
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
