import { fetchJson, type ApiRequestOptions } from "../client";
import { listWorkspaces } from "./workspace-api";

export const COLLABORATION_ACTIONS = ["read", "report", "prompt", "control"] as const;
export type CollaborationAction = (typeof COLLABORATION_ACTIONS)[number];
export type CollaborationFlags = Record<CollaborationAction, boolean>;
export type CollaborationGrant = CollaborationFlags & {
  caller_task_id: string;
  source_workspace_id: string;
  target_workspace_id: string;
};

function grantPath(taskId: string) {
  return `/api/v1/tasks/${encodeURIComponent(taskId)}/workspace-collaboration`;
}

function validateGrant(value: CollaborationGrant, taskId: string, workspaceId: string) {
  if (
    !value ||
    value.caller_task_id !== taskId ||
    value.source_workspace_id !== workspaceId ||
    typeof value.target_workspace_id !== "string" ||
    !value.target_workspace_id ||
    !COLLABORATION_ACTIONS.every((action) => typeof value[action] === "boolean")
  ) {
    throw new Error("Invalid workspace collaboration response");
  }
  return value;
}

export async function readWorkspaceCollaboration(
  taskId: string,
  workspaceId: string,
  signal: AbortSignal,
) {
  const options = { cache: "no-store" as const, init: { signal } };
  const [response, workspaces] = await Promise.all([
    fetchJson<{ grants: CollaborationGrant[] | null }>(grantPath(taskId), options),
    listWorkspaces(options),
  ]);
  if (
    !response ||
    !(response.grants === null || Array.isArray(response.grants)) ||
    !Array.isArray(workspaces.workspaces) ||
    !workspaces.workspaces.every(
      (workspace) =>
        workspace &&
        typeof workspace.id === "string" &&
        workspace.id &&
        typeof workspace.name === "string" &&
        (workspace.scopes === undefined ||
          (Array.isArray(workspace.scopes) &&
            workspace.scopes.every((scope) => typeof scope === "string"))),
    )
  ) {
    throw new Error("Invalid workspace collaboration collection");
  }
  return {
    grants: (response.grants ?? []).map((grant) => validateGrant(grant, taskId, workspaceId)),
    workspaces: workspaces.workspaces,
  };
}

export async function putWorkspaceCollaboration(
  taskId: string,
  workspaceId: string,
  targetId: string,
  flags: CollaborationFlags,
  options: ApiRequestOptions,
) {
  const result = await fetchJson<CollaborationGrant>(
    `${grantPath(taskId)}/${encodeURIComponent(targetId)}`,
    { ...options, init: { ...options.init, method: "PUT", body: JSON.stringify(flags) } },
  );
  validateGrant(result, taskId, workspaceId);
  if (
    result.target_workspace_id !== targetId ||
    !COLLABORATION_ACTIONS.every((action) => result[action] === flags[action])
  ) {
    throw new Error("Unexpected workspace collaboration save response");
  }
  return result;
}
