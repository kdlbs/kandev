import { useQuery } from "@tanstack/react-query";
import type { Repository } from "@/lib/types/http";
import { workspaceQueryOptions } from "@/lib/query/query-options/workspace";

const EMPTY_REPOSITORIES: Repository[] = [];

/**
 * Returns repositories for the given workspace.
 *
 * Delegates to TanStack Query; dedup, caching, and stale-while-revalidate
 * are handled automatically — no inFlightRef or manual loading flags needed.
 *
 * Signature-compatible with the old Zustand-backed hook.
 */
export function useRepositories(workspaceId: string | null, enabled = true) {
  const { data, isLoading } = useQuery({
    ...workspaceQueryOptions.repos(workspaceId ?? ""),
    enabled: !!workspaceId && enabled,
  });

  return {
    repositories: data?.repositories ?? EMPTY_REPOSITORIES,
    isLoading,
  };
}
