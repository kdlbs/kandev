import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  AssignableWorkspaceRole,
  ListDirectoryUsersResponse,
  ListWorkspaceMembersResponse,
  WorkspaceMember,
} from "@/lib/types/team-access";

export async function listWorkspaceMembers(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<ListWorkspaceMembersResponse>(
    `/api/v1/workspaces/${workspaceId}/members`,
    options,
  );
}

export async function upsertWorkspaceMember(
  workspaceId: string,
  userId: string,
  role: AssignableWorkspaceRole,
  options?: ApiRequestOptions,
) {
  return fetchJson<WorkspaceMember>(`/api/v1/workspaces/${workspaceId}/members/${userId}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify({ role }), ...(options?.init ?? {}) },
  });
}

export async function removeWorkspaceMember(
  workspaceId: string,
  userId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ success: boolean }>(`/api/v1/workspaces/${workspaceId}/members/${userId}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function transferWorkspaceOwnership(
  workspaceId: string,
  userId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ success: boolean }>(`/api/v1/workspaces/${workspaceId}/transfer-ownership`, {
    ...options,
    init: { method: "POST", body: JSON.stringify({ user_id: userId }), ...(options?.init ?? {}) },
  });
}

export async function listDirectoryUsers(options?: ApiRequestOptions) {
  return fetchJson<ListDirectoryUsersResponse>("/api/v1/users/directory", options);
}
