import { listWorkspaces, fetchUserSettings } from "@/lib/api";
import { updateUserSettings } from "@/lib/api/domains/settings-api";

/**
 * Server-side helper to resolve the active workspace ID.
 * Re-fetches workspaces and user settings (Next.js will deduplicate
 * these calls within the same request when the layout also fetches them).
 *
 * If the saved workspace ID no longer exists (e.g. workspace was deleted),
 * falls back to the first available workspace and persists the correction
 * so subsequent requests don't hit the stale ID.
 */
export async function getActiveWorkspaceId(): Promise<string | null> {
  const [workspacesRes, settingsRes] = await Promise.all([
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
  ]);

  // Only consider office workspaces (those with office_workflow_id set).
  const workspaces = workspacesRes.workspaces.filter((w) => w.office_workflow_id);
  const settingsWorkspaceId = settingsRes?.settings?.workspace_id || null;
  const matched = workspaces.find((w) => w.id === settingsWorkspaceId);

  if (matched) {
    return matched.id;
  }

  // Settings point to a workspace that no longer exists — fall back and persist.
  const fallbackId = workspaces[0]?.id ?? null;
  if (fallbackId && settingsWorkspaceId && fallbackId !== settingsWorkspaceId) {
    updateUserSettings({ workspace_id: fallbackId }).catch(() => {});
  }
  return fallbackId;
}
