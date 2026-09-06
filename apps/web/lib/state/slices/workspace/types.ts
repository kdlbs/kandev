import type {
  Repository,
  Branch,
  RepositoryBranchPolicy,
  RepositoryScript,
  RepositorySet,
} from "@/lib/types/http";

export type WorkspaceState = {
  items: Array<{
    id: string;
    name: string;
    description?: string | null;
    owner_id: string;
    /** The unit this workspace sits in; reach follows the tree. */
    unit_id?: string;
    /** The requesting user's role here. */
    viewer_role?: string;
    /** Scopes the requesting user holds here; gates every owner-only control. */
    scopes?: string[];
    member_count?: number;
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

export type RepositoriesState = {
  itemsByWorkspaceId: Record<string, Repository[]>;
  loadingByWorkspaceId: Record<string, boolean>;
  loadedByWorkspaceId: Record<string, boolean>;
};

/**
 * Named groups of workspace repositories, keyed by workspace like
 * `RepositoriesState`. Hydrated from the boot payload and kept current by the
 * `repository_set.*` WebSocket events.
 */
export type RepositorySetsState = {
  itemsByWorkspaceId: Record<string, RepositorySet[]>;
  loadingByWorkspaceId: Record<string, boolean>;
  loadedByWorkspaceId: Record<string, boolean>;
  /**
   * Bumped by every `repository_set.*` event for a workspace. A list response
   * captured before an event must not be applied after it, or the response
   * resurrects a deleted set or restores stale membership.
   */
  revisionByWorkspaceId: Record<string, number>;
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

export type RepositoryBranchPoliciesState = {
  itemsByRepositoryId: Record<string, RepositoryBranchPolicy[]>;
  loadingByRepositoryId: Record<string, boolean>;
  loadedByRepositoryId: Record<string, boolean>;
  revisionByRepositoryId: Record<string, number>;
};

export type RepositoryScriptsState = {
  itemsByRepositoryId: Record<string, RepositoryScript[]>;
  loadingByRepositoryId: Record<string, boolean>;
  loadedByRepositoryId: Record<string, boolean>;
};

export type WorkspaceSliceState = {
  workspaces: WorkspaceState;
  repositories: RepositoriesState;
  repositorySets: RepositorySetsState;
  repositoryBranchPolicies: RepositoryBranchPoliciesState;
  repositoryBranches: RepositoryBranchesState;
  repositoryScripts: RepositoryScriptsState;
};

export type WorkspaceSliceActions = {
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState["items"]) => void;
  setRepositories: (workspaceId: string, repositories: Repository[]) => void;
  upsertRepository: (workspaceId: string, repository: Repository) => void;
  setRepositoriesLoading: (workspaceId: string, loading: boolean) => void;
  setRepositoryBranches: (
    repositoryId: string,
    branches: Branch[],
    meta?: { fetchedAt?: string; fetchError?: string },
  ) => void;
  setRepositoryBranchesLoading: (repositoryId: string, loading: boolean) => void;
  setRepositoryBranchesFetchError: (repositoryId: string, error: string | undefined) => void;
  setRepositoryScripts: (repositoryId: string, scripts: RepositoryScript[]) => void;
  setRepositoryScriptsLoading: (repositoryId: string, loading: boolean) => void;
  clearRepositoryScripts: (repositoryId: string) => void;
  invalidateRepositories: (workspaceId: string) => void;
  /**
   * Applies a list response. `expectedRevision` is the value read before the
   * request; a mismatch means a WebSocket event landed meanwhile and the
   * response is stale, so it is dropped.
   */
  setRepositorySets: (
    workspaceId: string,
    sets: RepositorySet[],
    expectedRevision?: number,
  ) => void;
  setRepositorySetsLoading: (workspaceId: string, loading: boolean) => void;
  upsertRepositorySet: (workspaceId: string, set: RepositorySet) => void;
  removeRepositorySet: (workspaceId: string, setId: string) => void;
  invalidateRepositorySets: (workspaceId: string) => void;
  setRepositoryBranchPolicies: (
    repositoryId: string,
    policies: RepositoryBranchPolicy[],
    expectedRevision?: number,
  ) => void;
  setRepositoryBranchPoliciesLoading: (repositoryId: string, loading: boolean) => void;
  upsertRepositoryBranchPolicy: (policy: RepositoryBranchPolicy) => void;
  removeRepositoryBranchPolicy: (repositoryId: string, policyId: string) => void;
};

export type WorkspaceSlice = WorkspaceSliceState & WorkspaceSliceActions;
