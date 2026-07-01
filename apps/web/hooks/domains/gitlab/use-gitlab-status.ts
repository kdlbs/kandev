"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { gitlabStatusQueryOptions } from "@/lib/query/query-options/gitlab";
import type { GitLabStatus } from "@/lib/types/gitlab";

/**
 * useGitLabStatus reads the shared GitLab connection status query. Fetches on
 * mount; explicit retries are caller-driven through `refresh`.
 */
export function useGitLabStatus(initialStatus?: GitLabStatus | null) {
  const query = useQuery({
    ...gitlabStatusQueryOptions(),
    initialData: initialStatus ?? undefined,
  });
  const refetch = query.refetch;

  const refresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  return {
    status: query.data ?? null,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    refresh,
  };
}
