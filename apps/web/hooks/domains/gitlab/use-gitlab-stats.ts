"use client";

import { useQuery } from "@tanstack/react-query";
import { gitlabStatsQueryOptions } from "@/lib/query/query-options/gitlab";

/**
 * useGitLabStats subscribes to the open-MRs / awaiting-review / open-issues
 * counts surfaced on the /gitlab page header.
 */
export function useGitLabStats() {
  const query = useQuery(gitlabStatsQueryOptions());

  return { stats: query.data ?? null, loading: query.isFetching && !query.isSuccess };
}
