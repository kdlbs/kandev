import type { QueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { Workspace } from "@/lib/types/http";

export function updateWorkspacesCache(
  queryClient: QueryClient,
  updater: (items: Workspace[]) => Workspace[],
) {
  queryClient.setQueryData<Workspace[]>(qk.workspaces.all(), (current) => updater(current ?? []));
}

export function upsertWorkspaceCache(queryClient: QueryClient, workspace: Workspace) {
  updateWorkspacesCache(queryClient, (items) => {
    const existingIndex = items.findIndex((item) => item.id === workspace.id);
    if (existingIndex === -1) return [...items, workspace];
    return items.map((item) => (item.id === workspace.id ? workspace : item));
  });
}

export function patchWorkspaceCache(
  queryClient: QueryClient,
  workspaceId: string,
  patch: Partial<Workspace>,
) {
  updateWorkspacesCache(queryClient, (items) =>
    items.map((item) => (item.id === workspaceId ? { ...item, ...patch } : item)),
  );
}

export function removeWorkspaceCache(queryClient: QueryClient, workspaceId: string) {
  updateWorkspacesCache(queryClient, (items) => items.filter((item) => item.id !== workspaceId));
}
