import { useQuery } from "@tanstack/react-query";
import type { RepositoryScript } from "@/lib/types/http";
import { workspaceQueryOptions } from "@/lib/query/query-options/workspace";

const EMPTY_SCRIPTS: RepositoryScript[] = [];

/**
 * Returns repository scripts for the given repository.
 *
 * The old inFlightRef + manual loading-state pattern is replaced by
 * TanStack Query. The `isLoaded` return value maps to `!isLoading && !!data`.
 *
 * Signature-compatible with the old Zustand-backed hook.
 */
export function useRepositoryScripts(repositoryId: string | null, enabled = true) {
  const { data, isLoading, isFetched } = useQuery({
    ...workspaceQueryOptions.scripts(repositoryId),
    enabled: !!repositoryId && enabled,
  });

  return {
    scripts: data?.scripts ?? EMPTY_SCRIPTS,
    isLoading,
    isLoaded: isFetched,
  };
}
