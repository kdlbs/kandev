import type { ListWorkspacesResponse, Workspace } from "@/lib/types/http";

export const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
export const LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE = "office-active-workspace";

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];

export function mapWorkspaceItem(ws: WorkspaceItem): Workspace {
  return {
    id: ws.id,
    name: ws.name,
    description: ws.description ?? null,
    owner_id: ws.owner_id,
    default_executor_id: ws.default_executor_id ?? null,
    default_environment_id: ws.default_environment_id ?? null,
    default_agent_profile_id: ws.default_agent_profile_id ?? null,
    default_config_agent_profile_id: ws.default_config_agent_profile_id ?? null,
    office_workflow_id: ws.office_workflow_id ?? undefined,
    created_at: ws.created_at,
    updated_at: ws.updated_at,
  };
}

export function readCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const encodedName = `${encodeURIComponent(name)}=`;
  const entries = document.cookie
    .split(";")
    .map((part) => part.trim())
    .filter((part) => part.startsWith(encodedName));
  const entry = entries[entries.length - 1];
  return entry ? decodeURIComponent(entry.slice(encodedName.length)) : null;
}

export function readActiveWorkspaceCookie(): string | null {
  return readCookie(ACTIVE_WORKSPACE_COOKIE) || null;
}

type SettingsWorkspaceItem = {
  id: string;
  office_workflow_id?: string | null;
};

export function resolveSettingsActiveWorkspaceId(
  workspaceItems: SettingsWorkspaceItem[],
  activeCookieWorkspaceId: string | null,
  settingsWorkspaceId: string | null,
): string | null {
  const kanbanWorkspaceItems = workspaceItems.filter((workspace) => !workspace.office_workflow_id);

  return (
    kanbanWorkspaceItems.find((workspace) => workspace.id === activeCookieWorkspaceId)?.id ??
    kanbanWorkspaceItems.find((workspace) => workspace.id === settingsWorkspaceId)?.id ??
    kanbanWorkspaceItems[0]?.id ??
    workspaceItems[0]?.id ??
    null
  );
}
