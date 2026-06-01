"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { getWebSocketClient } from "@/lib/ws/connection";
import { qk } from "@/lib/query/keys";
import type { PRCommitInfo } from "@/lib/types/github";

async function fetchPRCommitsList(
  owner: string,
  repo: string,
  prNumber: number,
): Promise<PRCommitInfo[]> {
  const client = getWebSocketClient();
  if (!client) return [];
  const response = await client.request<{ commits?: PRCommitInfo[] }>("github.pr_commits.get", {
    owner,
    repo,
    number: prNumber,
  });
  return response?.commits ?? [];
}

/**
 * Fetches the commits in a pull request via WebSocket.
 * Returns commit metadata from the GitHub API.
 */
export function usePRCommits(
  owner: string | null,
  repo: string | null,
  prNumber: number | null,
  refreshKey?: string | null,
) {
  const hasParams = !!(owner && repo && prNumber);
  // Incorporate refreshKey so callers can force a refetch.
  const cacheKey = hasParams
    ? [...qk.github.prCommits(owner!, repo!, prNumber!), refreshKey ?? ""]
    : (["github", "pr-commits", null] as const);

  const {
    data: commits = [],
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: cacheKey,
    queryFn: () => fetchPRCommitsList(owner!, repo!, prNumber!),
    enabled: hasParams,
    staleTime: 30_000,
    refetchOnWindowFocus: false,
  });

  const refresh = useCallback(() => {
    if (!hasParams) return;
    void refetch();
  }, [hasParams, refetch]);

  if (!hasParams) {
    return { commits: [] as PRCommitInfo[], loading: false, error: null, refresh };
  }

  let errorMessage: string | null = null;
  if (error instanceof Error) errorMessage = error.message;
  else if (error) errorMessage = String(error);

  return {
    commits,
    loading: isLoading,
    error: errorMessage,
    refresh,
  };
}
