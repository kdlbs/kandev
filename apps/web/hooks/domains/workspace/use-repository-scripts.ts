import { useQuery } from "@tanstack/react-query";
import type { RepositoryScript } from "@/lib/types/http";
import { repositoryScriptsQueryOptions } from "@/lib/query/query-options";

const EMPTY_SCRIPTS: RepositoryScript[] = [];

export function useRepositoryScripts(repositoryId: string | null, enabled = true) {
  const query = useQuery({
    ...repositoryScriptsQueryOptions(repositoryId ?? ""),
    enabled: enabled && Boolean(repositoryId),
  });
  const scripts = query.data ?? EMPTY_SCRIPTS;

  return {
    scripts,
    isLoading: query.isFetching && scripts.length === 0,
    isLoaded: query.isFetched || Boolean(query.data),
  };
}
