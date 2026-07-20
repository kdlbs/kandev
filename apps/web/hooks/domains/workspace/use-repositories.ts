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
  const { data, isFetching, refetch } = query;
  const repositories = data ?? EMPTY_REPOSITORIES;
  const forcedRef = useRef<string | null>(null);

  useEffect(() => {
    if (!enabled || !forceRefresh || !workspaceId) return;
    if (forcedRef.current === workspaceId) return;
    forcedRef.current = workspaceId;
    void refetch();
  }, [enabled, forceRefresh, refetch, workspaceId]);

  return { repositories, isLoading: isFetching && repositories.length === 0 };
}
