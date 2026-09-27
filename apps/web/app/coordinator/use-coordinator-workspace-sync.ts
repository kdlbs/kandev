"use client";

import { useEffect } from "react";
import { useAppStoreApi } from "@/components/state-provider";

/**
 * Coordinator screens read task/workflow data that follows the store's active
 * workspace (via `useEnsureWorkspaceWorkflows`, mounted once in `AppSidebar`),
 * not the route's own `workspaceId`. Deep-linking or cold-loading a coordinator
 * URL for a workspace other than the active one left that data permanently
 * empty. Route through `setActiveWorkspace` (not a raw hydrate) so
 * `activeIdRevision`-keyed consumers see a correct revision bump, mirroring
 * `office-routes.tsx`'s bootstrap.
 */
export function useSyncActiveWorkspaceToRoute(workspaceId: string): void {
  const store = useAppStoreApi();

  useEffect(() => {
    const state = store.getState();
    if (state.workspaces.activeId === workspaceId) return;
    const known = state.workspaces.items.some((item) => item.id === workspaceId);
    if (!known) return;
    state.setActiveWorkspace(workspaceId);
  }, [store, workspaceId]);
}
