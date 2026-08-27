import { fetchJson, fetchJsonWithRetry, type ApiRequestOptions } from "../client";
import type {
  ListWorkspacesResponse,
  ListRepositoriesResponse,
  ListRepositorySetsResponse,
  ListRepositoryBranchPoliciesResponse,
  RepositoryBranchPolicy,
  RepositorySet,
  RepositoryBranchesResponse,
  ListRepositoryScriptsResponse,
  Workspace,
  Repository,
  TaskSession,
} from "@/lib/types/http";

// Workspace operations
export async function createWorkspace(
  payload: { name: string; description?: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<Workspace>("/api/v1/workspaces", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function listWorkspaces(options?: ApiRequestOptions) {
  return fetchJsonWithRetry<ListWorkspacesResponse>("/api/v1/workspaces", options);
}

// Repository operations
export async function listRepositories(
  workspaceId: string,
  params?: { includeScripts?: boolean },
  options?: ApiRequestOptions,
) {
  const searchParams = new URLSearchParams();
  if (params?.includeScripts) {
    searchParams.set("include_scripts", "true");
  }
  const queryString = searchParams.toString();
  const url = `/api/v1/workspaces/${workspaceId}/repositories${queryString ? `?${queryString}` : ""}`;
  return fetchJson<ListRepositoriesResponse>(url, options);
}

// Repository set operations
//
// Collection routes are workspace-scoped and item routes are flat, mirroring the
// repository routes above.

export async function listRepositorySets(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<ListRepositorySetsResponse>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/repository-sets`,
    options,
  );
}

export async function createRepositorySet(
  workspaceId: string,
  payload: { name: string; description?: string; repositoryIds: string[] },
  options?: ApiRequestOptions,
) {
  return fetchJson<RepositorySet>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/repository-sets`,
    {
      ...options,
      init: {
        method: "POST",
        body: JSON.stringify({
          name: payload.name,
          description: payload.description ?? "",
          repository_ids: payload.repositoryIds,
        }),
        ...(options?.init ?? {}),
      },
    },
  );
}

/**
 * Patches a set. Only the fields present in `payload` are sent: the backend
 * reads a present-but-empty `repository_ids` as a rejected request rather than
 * "leave membership alone", so an omitted field has to stay omitted.
 */
export async function updateRepositorySet(
  setId: string,
  payload: { name?: string; description?: string; repositoryIds?: string[] },
  options?: ApiRequestOptions,
) {
  const body: Record<string, unknown> = {};
  if (payload.name !== undefined) body.name = payload.name;
  if (payload.description !== undefined) body.description = payload.description;
  if (payload.repositoryIds !== undefined) body.repository_ids = payload.repositoryIds;
  return fetchJson<RepositorySet>(`/api/v1/repository-sets/${encodeURIComponent(setId)}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(body), ...(options?.init ?? {}) },
  });
}

export async function deleteRepositorySet(setId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`/api/v1/repository-sets/${encodeURIComponent(setId)}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function listRepositoryBranchPolicies(
  repositoryId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<ListRepositoryBranchPoliciesResponse>(
    `/api/v1/repositories/${encodeURIComponent(repositoryId)}/branch-policies`,
    options,
  );
}

export async function createRepositoryBranchPolicy(
  repositoryId: string,
  payload: Omit<RepositoryBranchPolicy, "id" | "repository_id" | "created_at" | "updated_at">,
  options?: ApiRequestOptions,
) {
  return fetchJson<RepositoryBranchPolicy>(
    `/api/v1/repositories/${encodeURIComponent(repositoryId)}/branch-policies`,
    {
      ...options,
      init: {
        method: "POST",
        body: JSON.stringify(toBranchPolicyPayload(payload)),
        ...(options?.init ?? {}),
      },
    },
  );
}

export async function updateRepositoryBranchPolicy(
  policyId: string,
  payload: Partial<
    Omit<RepositoryBranchPolicy, "id" | "repository_id" | "created_at" | "updated_at">
  >,
  options?: ApiRequestOptions,
) {
  return fetchJson<RepositoryBranchPolicy>(
    `/api/v1/repository-branch-policies/${encodeURIComponent(policyId)}`,
    {
      ...options,
      init: {
        method: "PATCH",
        body: JSON.stringify(toBranchPolicyPayload(payload)),
        ...(options?.init ?? {}),
      },
    },
  );
}

export async function deleteRepositoryBranchPolicy(policyId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`/api/v1/repository-branch-policies/${encodeURIComponent(policyId)}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function createGitflowRepositoryBranchPolicies(
  repositoryId: string,
  payload: { productionBranch: string; developmentBranch: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<ListRepositoryBranchPoliciesResponse>(
    `/api/v1/repositories/${encodeURIComponent(repositoryId)}/branch-policies/gitflow`,
    {
      ...options,
      init: {
        method: "POST",
        body: JSON.stringify({
          production_branch: payload.productionBranch,
          development_branch: payload.developmentBranch,
        }),
        ...(options?.init ?? {}),
      },
    },
  );
}

function toBranchPolicyPayload(
  payload: Partial<
    Omit<RepositoryBranchPolicy, "id" | "repository_id" | "created_at" | "updated_at">
  >,
) {
  return {
    ...(payload.name !== undefined ? { name: payload.name } : {}),
    ...(payload.description !== undefined ? { description: payload.description } : {}),
    ...(payload.base_branch !== undefined ? { base_branch: payload.base_branch } : {}),
    ...(payload.branch_template !== undefined ? { branch_template: payload.branch_template } : {}),
    ...(payload.pull_request_target !== undefined
      ? { pull_request_target: payload.pull_request_target }
      : {}),
  };
}

export async function initializeLocalRepository(
  workspaceId: string,
  payload: { name: string; parentPath: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<Repository>(`/api/v1/workspaces/${workspaceId}/repositories/initialize-local`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ name: payload.name, parent_path: payload.parentPath }),
      ...(options?.init ?? {}),
    },
  });
}

/**
 * Lists git branches for a workspace repo. Pass exactly one of `repositoryId`
 * (an imported workspace repo) or `path` (an on-machine folder discovered
 * but not yet imported). The backend resolves either to an absolute path and
 * runs the same `listGitBranches`. Used by the chip row's per-repo branch
 * picker which needs to handle both shapes.
 */
export async function listBranches(
  workspaceId: string,
  source: { repositoryId: string } | { path: string },
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams();
  if ("repositoryId" in source) params.set("repository_id", source.repositoryId);
  else params.set("path", source.path);
  return fetchJson<RepositoryBranchesResponse>(
    `/api/v1/workspaces/${workspaceId}/branches?${params.toString()}`,
    options,
  );
}

/**
 * Lists git branches for an imported workspace repository, scoped by
 * repository id only. Supports `refresh=true` to force a `git fetch` before
 * returning the list (with the backend's per-repo cooldown applied). Used by
 * single-repo flows that already have the repo id and want to drive the
 * stale-while-revalidate UI in the dialog.
 */
export async function listRepositoryBranches(
  repositoryId: string,
  params?: { refresh?: boolean },
  options?: ApiRequestOptions,
) {
  const qs = params?.refresh ? "?refresh=true" : "";
  return fetchJson<RepositoryBranchesResponse>(
    `/api/v1/repositories/${repositoryId}/branches${qs}`,
    options,
  );
}

export async function listRepositoryScripts(repositoryId: string, options?: ApiRequestOptions) {
  return fetchJson<ListRepositoryScriptsResponse>(
    `/api/v1/repositories/${repositoryId}/scripts`,
    options,
  );
}

// Quick Chat operations
type StartQuickChatCommon = {
  title?: string;
  agent_profile_id?: string;
  executor_id?: string;
  prompt?: string;
};

type StartQuickChatLegacyRepository = {
  repositories?: never;
  repository_id?: string;
  local_path?: string;
  repository_name?: string;
  default_branch?: string;
  base_branch?: string;
};

type StartQuickChatRepositories = {
  repositories: QuickChatRepositoryInput[];
  repository_id?: never;
  local_path?: never;
  repository_name?: never;
  default_branch?: never;
  base_branch?: never;
};

export type StartQuickChatRequest = StartQuickChatCommon &
  (StartQuickChatLegacyRepository | StartQuickChatRepositories);

export type QuickChatRepositoryInput = {
  repository_id: string;
  base_branch: string;
};

export type StartQuickChatResponse = {
  task_id: string;
  session_id: string;
};

export async function startQuickChat(
  workspaceId: string,
  payload: StartQuickChatRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<StartQuickChatResponse>(`/api/v1/workspaces/${workspaceId}/quick-chat`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export type QuickChatSessionResponse = {
  session_id: string;
  task_id: string;
  workspace_id: string;
  kind: "chat" | "config";
  name?: string;
  agent_profile_id?: string;
};

export type ListQuickChatSessionsResponse = {
  sessions: QuickChatSessionResponse[];
  task_sessions: TaskSession[];
};

/**
 * Lists the workspace's restorable quick-chat tabs.
 *
 * Quick chats are created and closed from any device, so this is the same list
 * the Go boot payload embeds — clients re-read it on (re)connect to converge on
 * the server's tabs instead of drifting apart.
 */
export async function listQuickChatSessions(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<ListQuickChatSessionsResponse>(
    `/api/v1/workspaces/${workspaceId}/quick-chats`,
    options,
  );
}

// Config Chat operations
export type StartConfigChatRequest = {
  agent_profile_id?: string;
  executor_id?: string;
  prompt?: string;
};

export type StartConfigChatResponse = {
  task_id: string;
  session_id: string;
};

export async function startConfigChat(
  workspaceId: string,
  payload: StartConfigChatRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<StartConfigChatResponse>(`/api/v1/workspaces/${workspaceId}/config-chat`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}
