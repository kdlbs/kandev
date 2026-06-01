"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { getWebSocketClient } from "@/lib/ws/connection";
import { qk } from "@/lib/query/keys";
import type { PRDiffFile } from "@/lib/types/github";

async function fetchPRDiffFiles(
  owner: string,
  repo: string,
  prNumber: number,
): Promise<PRDiffFile[]> {
  const client = getWebSocketClient();
  if (!client) return [];
  const response = await client.request<{ files?: PRDiffFile[] }>("github.pr_files.get", {
    owner,
    repo,
    number: prNumber,
  });
  return response?.files ?? [];
}

/**
 * Fetches the files changed in a pull request via WebSocket.
 * Returns structured diff data from the GitHub API with full unified diff patches.
 */
export function usePRDiff(
  owner: string | null,
  repo: string | null,
  prNumber: number | null,
  refreshKey?: string | null,
) {
  const hasParams = !!(owner && repo && prNumber);
  // Incorporate refreshKey into the cache key so callers can force a refetch
  // when last_synced_at advances.
  const cacheKey = hasParams
    ? qk.github.prFiles(owner!, repo!, prNumber!, refreshKey)
    : (["github", "pr-files", null] as const);

  const {
    data: files = [],
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: cacheKey,
    queryFn: () => fetchPRDiffFiles(owner!, repo!, prNumber!),
    enabled: hasParams,
    staleTime: 30_000,
    refetchOnWindowFocus: false,
  });

  const refresh = useCallback(() => {
    if (!hasParams) return;
    void refetch();
  }, [hasParams, refetch]);

  if (!hasParams) {
    return { files: [] as PRDiffFile[], loading: false, error: null, refresh };
  }

  let errorMessage: string | null = null;
  if (error instanceof Error) errorMessage = error.message;
  else if (error) errorMessage = String(error);

  return {
    files,
    loading: isLoading,
    error: errorMessage,
    refresh,
  };
}
