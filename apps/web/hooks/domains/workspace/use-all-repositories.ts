"use client";

import { useQuery, useQueries } from "@tanstack/react-query";
import type { Repository } from "@/lib/types/http";
import { workspaceQueryOptions } from "@/lib/query/query-options/workspace";

const EMPTY_REPOSITORIES: Repository[] = [];

/**
 * Reads repositories for every known workspace from the TanStack Query cache.
 *
 * Replaces the old `state.repositories.itemsByWorkspaceId` Zustand map. We
 * drive one `repos(wsId)` query per workspace (most already cached from the
 * task-create dialog / SSR seed) and expose both the flat list and a
 * by-workspace map so call sites can pick whichever they used before.
 *
 * `enabled` lets callers observe the cache without forcing a fetch for every
 * workspace when they only need whatever is already populated.
 */
export function useAllRepositories(enabled = true): {
  repositories: Repository[];
  byWorkspaceId: Record<string, Repository[]>;
} {
  const { data: wsData } = useQuery({ ...workspaceQueryOptions.all(), enabled });
  const workspaceIds = wsData?.workspaces.map((w) => w.id) ?? [];

  return useQueries({
    queries: workspaceIds.map((wsId) => ({
      ...workspaceQueryOptions.repos(wsId),
      enabled,
    })),
    combine: (results) => {
      const byWorkspaceId: Record<string, Repository[]> = {};
      const repositories: Repository[] = [];
      results.forEach((result, idx) => {
        const repos = result.data?.repositories ?? EMPTY_REPOSITORIES;
        const wsId = workspaceIds[idx];
        if (wsId) byWorkspaceId[wsId] = repos;
        for (const repo of repos) repositories.push(repo);
      });
      return { repositories, byWorkspaceId };
    },
  });
}
