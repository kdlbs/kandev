import { listWorkspaces, fetchUserSettings } from "@/lib/api";

/**
 * Server-side helper to resolve the active workspace ID.
 * Re-fetches workspaces and user settings (Next.js will deduplicate
 * these calls within the same request when the layout also fetches them).
 */
export async function getActiveWorkspaceId(): Promise<string | null> {
  const [workspacesRes, settingsRes] = await Promise.all([
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
  ]);

  const workspaces = workspacesRes.workspaces;
  const settingsWorkspaceId = settingsRes?.settings?.workspace_id || null;

  return workspaces.find((w) => w.id === settingsWorkspaceId)?.id ?? workspaces[0]?.id ?? null;
}
