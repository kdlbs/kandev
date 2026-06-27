import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import type { Repository } from "@/lib/types/http";
import { workspaceRepositoriesQueryOptions } from "@/lib/query/query-options";

const EMPTY_REPOSITORIES: Repository[] = [];

/**
 * Loads a workspace's repositories from TanStack Query. Pass `forceRefresh` to
 * pull a fresh list once per workspace on mount while preserving cached data.
 */
export function useRepositories(workspaceId: string | null, enabled = true, forceRefresh = false) {
  const query = useQuery({
    ...workspaceRepositoriesQueryOptions(workspaceId ?? ""),
    enabled: enabled && Boolean(workspaceId),
  });
  const repositories = query.data ?? EMPTY_REPOSITORIES;
  const forcedRef = useRef<string | null>(null);

  useEffect(() => {
    if (!enabled || !forceRefresh || !workspaceId) return;
    if (forcedRef.current === workspaceId) return;
    forcedRef.current = workspaceId;
    void query
      .refetch()
      .then((result) => {
        if (result.error && forcedRef.current === workspaceId) {
          forcedRef.current = null;
        }
      })
      .catch(() => {
        if (forcedRef.current === workspaceId) {
          forcedRef.current = null;
        }
      });
  }, [enabled, forceRefresh, query, workspaceId]);

  return { repositories, isLoading: query.isFetching && repositories.length === 0 };
}
