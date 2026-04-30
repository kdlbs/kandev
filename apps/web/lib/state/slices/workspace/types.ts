import type { Repository, Branch, RepositoryScript } from "@/lib/types/http";

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
    created_at: string;
    updated_at: string;
  }>;
  activeId: string | null;
};

export type RepositoriesState = {
  itemsByWorkspaceId: Record<string, Repository[]>;
  loadingByWorkspaceId: Record<string, boolean>;
  loadedByWorkspaceId: Record<string, boolean>;
};

export type RepositoryBranchesState = {
  itemsByRepositoryId: Record<string, Branch[]>;
  loadingByRepositoryId: Record<string, boolean>;
  loadedByRepositoryId: Record<string, boolean>;
  // RFC3339 timestamp of the most recent successful refresh from the backend.
  fetchedAtByRepositoryId: Record<string, string | undefined>;
  // Last fetch error for each repository, when a refresh attempt failed.
  fetchErrorByRepositoryId: Record<string, string | undefined>;
};

export type RepositoryScriptsState = {
  itemsByRepositoryId: Record<string, RepositoryScript[]>;
  loadingByRepositoryId: Record<string, boolean>;
  loadedByRepositoryId: Record<string, boolean>;
};

export type WorkspaceSliceState = {
  workspaces: WorkspaceState;
  repositories: RepositoriesState;
  repositoryBranches: RepositoryBranchesState;
  repositoryScripts: RepositoryScriptsState;
};

export type WorkspaceSliceActions = {
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState["items"]) => void;
  setRepositories: (workspaceId: string, repositories: Repository[]) => void;
  setRepositoriesLoading: (workspaceId: string, loading: boolean) => void;
  setRepositoryBranches: (
    repositoryId: string,
    branches: Branch[],
    meta?: { fetchedAt?: string; fetchError?: string },
  ) => void;
  setRepositoryBranchesLoading: (repositoryId: string, loading: boolean) => void;
  setRepositoryScripts: (repositoryId: string, scripts: RepositoryScript[]) => void;
  setRepositoryScriptsLoading: (repositoryId: string, loading: boolean) => void;
  clearRepositoryScripts: (repositoryId: string) => void;
  invalidateRepositories: (workspaceId: string) => void;
};

export type WorkspaceSlice = WorkspaceSliceState & WorkspaceSliceActions;
