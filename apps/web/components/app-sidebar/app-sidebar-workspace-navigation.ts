export type SidebarWorkspace = {
  id: string;
  office_workflow_id?: string | null;
};

export const LAST_KANBAN_WORKSPACE_KEY = "kandev.lastKanbanWorkspaceId";
export const OFFICE_ACTIVE_WORKSPACE_COOKIE = "office-active-workspace";

export function isOfficeWorkspace(workspace: SidebarWorkspace | undefined): boolean {
  return Boolean(workspace?.office_workflow_id);
}

export function workspaceHomeHref(workspace: SidebarWorkspace | undefined): string {
  if (!workspace) return "/";
  const path = isOfficeWorkspace(workspace) ? "/office" : "/";
  return `${path}?workspaceId=${workspace.id}`;
}

export function rememberLastKanbanWorkspace(workspace: SidebarWorkspace | undefined): void {
  if (!workspace || isOfficeWorkspace(workspace) || typeof window === "undefined") return;
  window.localStorage.setItem(LAST_KANBAN_WORKSPACE_KEY, workspace.id);
}

export function rememberLastOfficeWorkspace(workspace: SidebarWorkspace | undefined): void {
  if (!workspace || !isOfficeWorkspace(workspace) || typeof document === "undefined") return;
  document.cookie = `${OFFICE_ACTIVE_WORKSPACE_COOKIE}=${encodeURIComponent(workspace.id)}; path=/; max-age=86400; samesite=strict; secure`;
}

export function resolveLastKanbanWorkspace(
  workspaces: SidebarWorkspace[],
): SidebarWorkspace | null {
  const kanbanWorkspaces = workspaces.filter((workspace) => !isOfficeWorkspace(workspace));
  if (kanbanWorkspaces.length === 0) return null;

  if (typeof window !== "undefined") {
    const storedId = window.localStorage.getItem(LAST_KANBAN_WORKSPACE_KEY);
    const stored = kanbanWorkspaces.find((workspace) => workspace.id === storedId);
    if (stored) return stored;
  }

  return kanbanWorkspaces[0] ?? null;
}

export function resolveLastOfficeWorkspace(
  workspaces: SidebarWorkspace[],
): SidebarWorkspace | null {
  const officeWorkspaces = workspaces.filter(isOfficeWorkspace);
  if (officeWorkspaces.length === 0) return null;

  const storedId =
    typeof document === "undefined" ? null : readCookieValue(OFFICE_ACTIVE_WORKSPACE_COOKIE);
  const stored = officeWorkspaces.find((workspace) => workspace.id === storedId);
  return stored ?? officeWorkspaces[0] ?? null;
}

function readCookieValue(name: string): string | null {
  const prefix = `${name}=`;
  const match = document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(prefix));
  if (!match) return null;
  return decodeURIComponent(match.slice(prefix.length));
}
