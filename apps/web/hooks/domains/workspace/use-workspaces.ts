import { useQuery } from "@tanstack/react-query";
import type { Workspace } from "@/lib/types/http";
import { workspaceQueryOptions } from "@/lib/query/query-options/workspace";

const EMPTY_WORKSPACES: Workspace[] = [];

/**
 * Returns the list of workspaces the current user has access to, read from the
 * TanStack Query cache (seeded by SSR + kept fresh by the workspace bridge).
 *
 * Replaces the old `useAppStore(state => state.workspaces.items)` mirror read.
 * The active workspace id stays in Zustand (client-only) — see
 * `useActiveWorkspaceId` / `setActiveWorkspace`.
 */
export function useWorkspaces(): {
  workspaces: Workspace[];
  isLoading: boolean;
} {
  const { data, isLoading } = useQuery(workspaceQueryOptions.all());
  return {
    workspaces: data?.workspaces ?? EMPTY_WORKSPACES,
    isLoading,
  };
}

/** Looks up a single workspace by id from the TQ workspaces list. */
export function useWorkspace(workspaceId: string | null): Workspace | null {
  const { data } = useQuery(workspaceQueryOptions.all());
  if (!workspaceId) return null;
  return data?.workspaces.find((w) => w.id === workspaceId) ?? null;
}
