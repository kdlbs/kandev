import { listWorkspaces, fetchUserSettings } from "@/lib/api";

/**
 * Server-side helper to resolve the active workspace ID.
 * Re-fetches workspaces and user settings (Next.js will deduplicate
 * these calls within the same request when the layout also fetches them).
 *
 * Priority order:
 * 1. urlWorkspaceId when it matches a valid office workspace.
 * 2. userSettings.workspace_id when it matches a valid office workspace.
 * 3. First available office workspace as fallback.
 *
 * Does NOT write to user settings - the caller must not pollute the shared
 * workspace_id that kanban uses.
 */
export async function getActiveWorkspaceId(urlWorkspaceId?: string): Promise<string | null> {
  const [workspacesRes, settingsRes] = await Promise.all([
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
  ]);

  // Only consider office workspaces (those with office_workflow_id set).
  const workspaces = workspacesRes.workspaces.filter((w) => w.office_workflow_id);

  // 1. URL param wins when it matches a valid office workspace.
  if (urlWorkspaceId) {
    const urlMatch = workspaces.find((w) => w.id === urlWorkspaceId);
    if (urlMatch) return urlMatch.id;
  }

  // 2. Check if the settings workspace_id points to a valid office workspace.
  const settingsWorkspaceId = settingsRes?.settings?.workspace_id || null;
  const matched = workspaces.find((w) => w.id === settingsWorkspaceId);
  if (matched) {
    return matched.id;
  }

  // 3. Fall back to the first available office workspace.
  // Do NOT persist this - userSettings.workspace_id belongs to kanban.
  return workspaces[0]?.id ?? null;
}
