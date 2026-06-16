import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import type { ListWorkspacesResponse } from "@/lib/types/http";

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];

export function mapWorkspaceItem(ws: WorkspaceItem): WorkspaceState["items"][number] {
  return {
    id: ws.id,
    name: ws.name,
    description: ws.description ?? null,
    owner_id: ws.owner_id,
    default_executor_id: ws.default_executor_id ?? null,
    default_environment_id: ws.default_environment_id ?? null,
    default_agent_profile_id: ws.default_agent_profile_id ?? null,
    default_config_agent_profile_id: ws.default_config_agent_profile_id ?? null,
    office_workflow_id: ws.office_workflow_id ?? null,
    created_at: ws.created_at,
    updated_at: ws.updated_at,
  };
}

export function readCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const encodedName = `${encodeURIComponent(name)}=`;
  const entry = document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(encodedName));
  return entry ? decodeURIComponent(entry.slice(encodedName.length)) : null;
}
