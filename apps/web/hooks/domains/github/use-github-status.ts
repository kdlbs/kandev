"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { githubStatusQueryOptions } from "@/lib/query/query-options/github";
import type { GitHubStatus } from "@/lib/types/github";

export function useGitHubStatus(initialStatus?: GitHubStatus | null) {
  const query = useQuery({
    ...githubStatusQueryOptions(),
    initialData: initialStatus ?? undefined,
  });
  const invalidateSystemHealth = useAppStore((state) => state.invalidateSystemHealth);

  const refresh = useCallback(() => {
    // Also invalidate system health so the header indicator refetches
    invalidateSystemHealth();
    void query.refetch();
  }, [invalidateSystemHealth, query]);

  return {
    status: query.data ?? null,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    refresh,
  };
}
