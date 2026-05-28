"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { githubQueryOptions } from "@/lib/query/query-options/github";
import { qk } from "@/lib/query/keys";

export function useGitHubStatus() {
  const qc = useQueryClient();
  const { data: status, isLoading, isFetching, isSuccess } = useQuery(githubQueryOptions.status());

  function refresh() {
    // Invalidate both github status and system health so the header indicator refetches.
    void qc.invalidateQueries({ queryKey: qk.github.status() });
    void qc.invalidateQueries({ queryKey: qk.settings.systemHealth() });
  }

  return {
    status: status ?? null,
    loaded: isSuccess,
    loading: isLoading || isFetching,
    refresh,
  };
}
