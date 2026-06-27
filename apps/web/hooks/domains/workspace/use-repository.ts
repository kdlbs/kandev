import { useMemo } from "react";
import { useAllCachedRepositories } from "./use-repository-cache";

export function useRepository(repositoryId: string | null) {
  const repositories = useAllCachedRepositories();

  return useMemo(() => {
    if (!repositoryId) return null;
    return repositories.find((repo) => repo.id === repositoryId) ?? null;
  }, [repositories, repositoryId]);
}
