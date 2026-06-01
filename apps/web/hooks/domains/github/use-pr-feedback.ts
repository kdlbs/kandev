"use client";

import { useQuery } from "@tanstack/react-query";
import { githubQueryOptions } from "@/lib/query/query-options/github";

/**
 * Fetch live PR feedback (reviews, comments, checks) from GitHub.
 * Uses TanStack Query for caching and dedup; callers call `refresh()` imperatively.
 */
export function usePRFeedback(owner: string | null, repo: string | null, prNumber: number | null) {
  const {
    data: feedback,
    isLoading,
    error,
    refetch,
  } = useQuery(githubQueryOptions.prFeedback(owner, repo, prNumber));

  function refresh() {
    void refetch();
  }

  let errorMessage: string | null = null;
  if (error instanceof Error) errorMessage = error.message;
  else if (error) errorMessage = String(error);

  return {
    feedback: feedback ?? null,
    loading: isLoading,
    error: errorMessage,
    refresh,
  };
}
