import { fetchJson, type ApiRequestOptions } from "../client";

export type GrantDTO = {
  id: string;
  coordinator_task_id: string;
  principal_id?: string;
  workspace_id: string;
  scope_kind: string;
  scope_id: string;
  capabilities: string;
  note: string;
  granted_by_user_id: string;
  granted_at: string;
  revoked_at?: string;
  revoked_by_user_id?: string;
};

export type AuditEventDTO = {
  id: string;
  occurred_at: string;
  principal_id: string;
  actor_task_id: string;
  actor_session_id: string;
  target_task_id: string;
  workspace_id: string;
  action: string;
  capability: string;
  decision: string;
  grant_id: string;
  result: string;
  detail: string;
};

type GrantListResponse = {
  grants: GrantDTO[];
  total: number;
};

type AuditListResponse = {
  events: AuditEventDTO[];
  total: number;
};

export type CreateGrantRequest = {
  coordinator_task_id: string;
  scope_kind: "workspace" | "workflow";
  scope_id?: string;
  capabilities: string;
  note?: string;
};

/**
 * List coordinator grants for a workspace.
 */
export function listWorkspaceCoordinatorGrants(
  workspaceId: string,
  options?: ApiRequestOptions & { taskId?: string; includeRevoked?: boolean },
): Promise<GrantListResponse> {
  const params = new URLSearchParams();
  if (options?.taskId) params.set("task_id", options.taskId);
  if (options?.includeRevoked) params.set("include_revoked", "true");
  const qs = params.toString();
  return fetchJson<GrantListResponse>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/coordinator-grants${qs ? `?${qs}` : ""}`,
    { init: options?.init },
  );
}

/**
 * Create a new coordinator grant in a workspace.
 */
export function createWorkspaceCoordinatorGrant(
  workspaceId: string,
  body: CreateGrantRequest,
  options?: ApiRequestOptions,
): Promise<{ grant: GrantDTO }> {
  return fetchJson<{ grant: GrantDTO }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/coordinator-grants`,
    {
      ...options,
      init: {
        ...options?.init,
        method: "POST",
        body: JSON.stringify(body),
        headers: { "Content-Type": "application/json" },
      },
    },
  );
}

/**
 * List coordinator grants by task ID.
 */
export function listTaskCoordinatorGrants(
  taskId: string,
  options?: ApiRequestOptions & { includeRevoked?: boolean },
): Promise<GrantListResponse> {
  const params = new URLSearchParams();
  if (options?.includeRevoked) params.set("include_revoked", "true");
  const qs = params.toString();
  return fetchJson<GrantListResponse>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/coordinator-grants${qs ? `?${qs}` : ""}`,
    { init: options?.init },
  );
}

/**
 * Revoke a coordinator grant.
 */
export function revokeCoordinatorGrant(
  grantId: string,
  options?: ApiRequestOptions,
): Promise<{ id: string; revoked: boolean }> {
  return fetchJson<{ id: string; revoked: boolean }>(
    `/api/v1/coordinator-grants/${encodeURIComponent(grantId)}`,
    {
      ...options,
      init: {
        ...options?.init,
        method: "DELETE",
      },
    },
  );
}

/**
 * List coordinator audit events for a workspace.
 */
export function listWorkspaceCoordinatorAudit(
  workspaceId: string,
  options?: ApiRequestOptions & { taskId?: string; limit?: number },
): Promise<AuditListResponse> {
  const params = new URLSearchParams();
  if (options?.taskId) params.set("task_id", options.taskId);
  if (options?.limit) params.set("limit", String(options.limit));
  const qs = params.toString();
  return fetchJson<AuditListResponse>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/coordinator-audit${qs ? `?${qs}` : ""}`,
    options,
  );
}

/**
 * Parse a capabilities string into a set.
 */
export function parseCapabilities(capabilities: string): Set<string> {
  return new Set(
    capabilities
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean),
  );
}

/**
 * Join a set of capabilities into a comma-separated string.
 */
export function joinCapabilities(caps: Set<string>): string {
  return Array.from(caps).join(",");
}
