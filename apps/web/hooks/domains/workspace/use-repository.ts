"use client";

import { useQuery, useQueries } from "@tanstack/react-query";
import type { Repository } from "@/lib/types/http";
import { workspaceQueryOptions } from "@/lib/query/query-options/workspace";

/**
 * Looks up a single repository by id across all cached workspace repo lists.
 *
 * Implementation:
 *  1. Fetches the workspace list (usually already cached from app bootstrap).
 *  2. Fetches repos for every workspace reactively via useQueries.
 *  3. Returns the first matching repository, or null if not found.
 *
 * Replaces the old Zustand implementation that searched
 * repositories.itemsByWorkspaceId across all workspaces.
 *
 * Signature-compatible with the old hook.
 */
export function useRepository(repositoryId: string | null): Repository | null {
  const { data: wsData } = useQuery({
    ...workspaceQueryOptions.all(),
    enabled: !!repositoryId,
  });

  const workspaceIds = wsData?.workspaces.map((w) => w.id) ?? [];

  const found = useQueries({
    queries: workspaceIds.map((wsId) => ({
      ...workspaceQueryOptions.repos(wsId),
      enabled: !!repositoryId,
    })),
    combine: (results): Repository | null => {
      for (const result of results) {
        const match = result.data?.repositories?.find((r: Repository) => r.id === repositoryId);
        if (match) return match;
      }
      return null;
    },
  });

  return found;
}
