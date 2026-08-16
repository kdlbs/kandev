import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import type { ListWorkspacesResponse } from "@/lib/types/http";

export const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
export const LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE = "office-active-workspace";

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

/**
 * The active workspace for a settings boot: whatever the user last had active,
 * then their stored preference, then the first workspace that exists.
 *
 * Deliberately not filtered to kanban workspaces. It used to prefer them, which
 * was invisible while Office-vs-kanban chrome came from the pathname — Settings
 * is not an `/office` route, so it rendered kanban chrome regardless. Now that
 * the chrome follows the active workspace, that filter would *write* a kanban
 * workspace into the store on every settings visit, so an Office user opening
 * Settings would silently have their workspace switched underneath them.
 *
 * Settings is shared chrome, reachable from either mode. It has no business
 * preferring one.
 */
export function resolveSettingsActiveWorkspaceId(
  workspaceItems: SettingsWorkspaceItem[],
  activeCookieWorkspaceId: string | null,
  settingsWorkspaceId: string | null,
): string | null {
  return (
    workspaceItems.find((workspace) => workspace.id === activeCookieWorkspaceId)?.id ??
    workspaceItems.find((workspace) => workspace.id === settingsWorkspaceId)?.id ??
    workspaceItems[0]?.id ??
    null
  );
}
