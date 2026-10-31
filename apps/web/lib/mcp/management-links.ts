import type { MCPOwnerUsage } from "@/lib/types/http-mcp";
import { linkToTask } from "@/lib/links";

export type MCPOwnerDestination =
  | { scope: "repository"; workspaceId: string; ownerId: string }
  | { scope: "profile"; workspaceId: string; ownerId: string; agentId: string }
  | { scope: "task"; workspaceId: string; ownerId: string }
  | { scope: "task_session"; workspaceId: string; ownerId: string; taskId: string };

function validIdentity(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    value.trim() === value &&
    !/[\u0000-\u001f\u007f\ufffd]/.test(value)
  );
}

export function mcpOwnerDestinationHref(destination: MCPOwnerDestination): string | null {
  if (!validIdentity(destination.workspaceId) || !validIdentity(destination.ownerId)) return null;
  const owner = encodeURIComponent(destination.ownerId);
  const workspace = encodeURIComponent(destination.workspaceId);
  switch (destination.scope) {
    case "repository":
      return `/settings/workspaces/${workspace}/repositories?${new URLSearchParams({ repositoryId: destination.ownerId })}#mcp`;
    case "profile":
      if (!validIdentity(destination.agentId)) return null;
      return `/settings/agents/${encodeURIComponent(destination.agentId)}/profiles/${owner}?${new URLSearchParams({ workspaceId: destination.workspaceId })}#mcp`;
    case "task":
      return linkToTask(destination.ownerId);
    case "task_session":
      return validIdentity(destination.taskId)
        ? linkToTask(destination.taskId, { sessionId: destination.ownerId })
        : null;
    default:
      return null;
  }
}

export function mcpCatalogUsageHref(workspaceId: string, definitionId: string): string | null {
  if (!validIdentity(workspaceId) || !validIdentity(definitionId)) return null;
  return `/settings/workspaces/${encodeURIComponent(workspaceId)}/mcp-servers?${new URLSearchParams({ serverId: definitionId, view: "usage" })}`;
}

export function readMCPContextHint(
  params: URLSearchParams,
  key: string,
): { present: boolean; value: string | null } {
  const values = params.getAll(key);
  return {
    present: values.length > 0,
    value: values.length === 1 && validIdentity(values[0]) ? values[0] : null,
  };
}

export function mcpUsageOwnerHref(owner: MCPOwnerUsage, workspaceId: string): string | null {
  const destination = owner.destination;
  if (
    !destination ||
    owner.workspace_id !== workspaceId ||
    destination.workspace_id !== workspaceId ||
    destination.kind !== owner.scope ||
    destination.owner_id !== owner.owner_id
  )
    return null;
  const base = { workspaceId, ownerId: owner.owner_id };
  switch (destination.kind) {
    case "profile":
      return destination.agent_id && destination.agent_id === owner.agent_id
        ? mcpOwnerDestinationHref({ ...base, scope: "profile", agentId: destination.agent_id })
        : null;
    case "task_session":
      return destination.task_id && destination.task_id === owner.task_id
        ? mcpOwnerDestinationHref({ ...base, scope: "task_session", taskId: destination.task_id })
        : null;
    case "repository":
    case "task":
      return mcpOwnerDestinationHref({ ...base, scope: destination.kind });
    default:
      return null;
  }
}
