"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import { githubStatusQueryOptions } from "@/lib/query/query-options/github";
import type { GitHubStatus } from "@/lib/types/github";

export function useGitHubStatus(initialStatus?: GitHubStatus | null) {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...githubStatusQueryOptions(),
    initialData: initialStatus ?? undefined,
  });

  const refresh = useCallback(() => {
    // Also invalidate system health so the header indicator refetches
    void queryClient.invalidateQueries({ queryKey: qk.settings.systemHealth() });
    void query.refetch();
  }, [query, queryClient]);

  return {
    status: query.data ?? null,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    refresh,
  };
}
