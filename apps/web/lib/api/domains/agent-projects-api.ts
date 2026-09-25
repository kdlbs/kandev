import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  AgentProject,
  AgentProjectContextEntry,
  AgentProjectContextFile,
} from "@/lib/types/http-agent-projects";

export type CreateAgentProjectPayload = {
  name: string;
  repositoryIds: string[];
  primaryRepositoryId: string;
  coordinatorProfileId: string;
  economyProfileId: string;
  frontierProfileId: string;
  requestKey: string;
};

export type UpdateAgentProjectPayload = {
  revision: number;
  applyWorkspaceDefaultExecutor?: boolean;
  name?: string;
  repositoryIds?: string[];
  primaryRepositoryId?: string;
  coordinatorProfileId?: string;
  economyProfileId?: string;
  frontierProfileId?: string;
};

function projectCollectionPath(workspaceId: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/agent-projects`;
}

function projectPath(workspaceId: string, projectId: string): string {
  return `${projectCollectionPath(workspaceId)}/${encodeURIComponent(projectId)}`;
}

export async function listAgentProjects(
  workspaceId: string,
  archived = false,
  options?: ApiRequestOptions,
): Promise<{ projects: AgentProject[] }> {
  const query = archived ? "?archived=true" : "";
  return fetchJson<{ projects: AgentProject[] }>(
    `${projectCollectionPath(workspaceId)}${query}`,
    options,
  );
}

export async function createAgentProject(
  workspaceId: string,
  payload: CreateAgentProjectPayload,
  options?: ApiRequestOptions,
): Promise<AgentProject> {
  return fetchJson<AgentProject>(projectCollectionPath(workspaceId), {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({
        name: payload.name,
        repository_ids: payload.repositoryIds,
        primary_repository_id: payload.primaryRepositoryId,
        coordinator_profile_id: payload.coordinatorProfileId,
        economy_profile_id: payload.economyProfileId,
        frontier_profile_id: payload.frontierProfileId,
        request_key: payload.requestKey,
      }),
      ...(options?.init ?? {}),
    },
  });
}

export async function updateAgentProject(
  workspaceId: string,
  projectId: string,
  payload: UpdateAgentProjectPayload,
  options?: ApiRequestOptions,
): Promise<AgentProject> {
  const body: Record<string, string | number | string[] | boolean> = {
    revision: payload.revision,
  };
  if (payload.applyWorkspaceDefaultExecutor) body.apply_workspace_default_executor = true;
  if (payload.name !== undefined) body.name = payload.name;
  if (payload.repositoryIds !== undefined) body.repository_ids = payload.repositoryIds;
  if (payload.primaryRepositoryId !== undefined) {
    body.primary_repository_id = payload.primaryRepositoryId;
  }
  if (payload.coordinatorProfileId !== undefined) {
    body.coordinator_profile_id = payload.coordinatorProfileId;
  }
  if (payload.economyProfileId !== undefined) body.economy_profile_id = payload.economyProfileId;
  if (payload.frontierProfileId !== undefined) {
    body.frontier_profile_id = payload.frontierProfileId;
  }
  return fetchJson<AgentProject>(projectPath(workspaceId, projectId), {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(body), ...(options?.init ?? {}) },
  });
}

export async function archiveAgentProject(
  workspaceId: string,
  projectId: string,
  options?: ApiRequestOptions,
): Promise<AgentProject> {
  return fetchJson<AgentProject>(`${projectPath(workspaceId, projectId)}/archive`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function restoreAgentProject(
  workspaceId: string,
  projectId: string,
  options?: ApiRequestOptions,
): Promise<AgentProject> {
  return fetchJson<AgentProject>(`${projectPath(workspaceId, projectId)}/restore`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function deleteAgentProject(
  workspaceId: string,
  projectId: string,
  deleteContext: boolean,
  discardWorktreeChanges = false,
  options?: ApiRequestOptions,
): Promise<void> {
  const query = new URLSearchParams();
  if (deleteContext) query.set("delete_context", "true");
  if (discardWorktreeChanges) query.set("discard_worktree_changes", "true");
  const suffix = query.size ? `?${query.toString()}` : "";
  return fetchJson<void>(`${projectPath(workspaceId, projectId)}${suffix}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

function contextPath(workspaceId: string, projectId: string, endpoint: string): string {
  return `${projectPath(workspaceId, projectId)}/context/${endpoint}`;
}

export async function listAgentProjectContext(
  workspaceId: string,
  projectId: string,
  path = "",
  options?: ApiRequestOptions,
): Promise<{ entries: AgentProjectContextEntry[] }> {
  const query = new URLSearchParams();
  if (path) query.set("path", path);
  const suffix = query.size ? `?${query.toString()}` : "";
  return fetchJson<{ entries: AgentProjectContextEntry[] }>(
    `${contextPath(workspaceId, projectId, "tree")}${suffix}`,
    options,
  );
}

export async function readAgentProjectContextFile(
  workspaceId: string,
  projectId: string,
  path: string,
  options?: ApiRequestOptions,
): Promise<AgentProjectContextFile> {
  const query = new URLSearchParams({ path });
  return fetchJson<AgentProjectContextFile>(
    `${contextPath(workspaceId, projectId, "content")}?${query.toString()}`,
    options,
  );
}

export async function writeAgentProjectContextFile(
  workspaceId: string,
  projectId: string,
  payload: { path: string; content: string; expectedHash: string },
  options?: ApiRequestOptions,
): Promise<{ path: string; hash: string }> {
  return fetchJson<{ path: string; hash: string }>(contextPath(workspaceId, projectId, "content"), {
    ...options,
    init: {
      method: "PUT",
      body: JSON.stringify({
        path: payload.path,
        content: payload.content,
        expected_hash: payload.expectedHash,
      }),
      ...(options?.init ?? {}),
    },
  });
}
