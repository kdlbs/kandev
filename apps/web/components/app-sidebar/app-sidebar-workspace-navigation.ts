export type SidebarWorkspace = {
  id: string;
  office_workflow_id?: string | null;
};

export const LAST_KANBAN_WORKSPACE_KEY = "kandev.lastKanbanWorkspaceId";

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

export function resolveFirstOfficeWorkspace(
  workspaces: SidebarWorkspace[],
): SidebarWorkspace | null {
  return workspaces.find(isOfficeWorkspace) ?? null;
}
