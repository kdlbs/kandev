import { useCallback, useRef, useSyncExternalStore } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import type { Repository } from "@/lib/types/http";

export type RepositoriesByWorkspace = Record<string, Repository[]>;

const EMPTY_BY_WORKSPACE: RepositoriesByWorkspace = {};
const EMPTY_REPOSITORIES: Repository[] = [];

type RepositoryCacheSnapshot = {
  signature: string;
  repositoriesByWorkspace: RepositoriesByWorkspace;
};

export function useRepositoriesByWorkspace(): RepositoriesByWorkspace {
  const queryClient = useQueryClient();
  const snapshotRef = useRef<RepositoryCacheSnapshot>({
    signature: "",
    repositoriesByWorkspace: EMPTY_BY_WORKSPACE,
  });
  const getSnapshot = useCallback(() => {
    const snapshot = readRepositoriesByWorkspace(queryClient);
    if (snapshot.signature === snapshotRef.current.signature) {
      return snapshotRef.current.repositoriesByWorkspace;
    }
    snapshotRef.current = snapshot;
    return snapshot.repositoriesByWorkspace;
  }, [queryClient]);

  return useSyncExternalStore(
    (onStoreChange) => queryClient.getQueryCache().subscribe(onStoreChange),
    getSnapshot,
    () => EMPTY_BY_WORKSPACE,
  );
}

export function useCachedRepositories(workspaceId: string | null | undefined): Repository[] {
  const repositoriesByWorkspace = useRepositoriesByWorkspace();
  if (!workspaceId) return EMPTY_REPOSITORIES;
  return repositoriesByWorkspace[workspaceId] ?? EMPTY_REPOSITORIES;
}

export function useAllCachedRepositories(): Repository[] {
  return Object.values(useRepositoriesByWorkspace()).flat();
}

export function readRepositoriesByWorkspace(queryClient: QueryClient): RepositoryCacheSnapshot {
  const queries = queryClient
    .getQueryCache()
    .findAll()
    .filter((query) => {
      const key = query.queryKey;
      return (
        Array.isArray(key) &&
        key[0] === "workspaces" &&
        typeof key[1] === "string" &&
        key[2] === "repositories" &&
        Array.isArray(query.state.data)
      );
    })
    .sort((a, b) => a.state.dataUpdatedAt - b.state.dataUpdatedAt);

  const grouped = new Map<string, Map<string, Repository>>();
  for (const query of queries) {
    const key = query.queryKey;
    const workspaceId = key[1] as string;
    const workspaceRepos = grouped.get(workspaceId) ?? new Map<string, Repository>();
    for (const repository of query.state.data as Repository[]) {
      workspaceRepos.set(repository.id, {
        ...workspaceRepos.get(repository.id),
        ...repository,
      });
    }
    grouped.set(workspaceId, workspaceRepos);
  }

  const repositoriesByWorkspace = Object.fromEntries(
    [...grouped.entries()].map(([workspaceId, repositories]) => [
      workspaceId,
      [...repositories.values()],
    ]),
  );

  return {
    signature: queries
      .map(
        (query) => `${query.queryHash}:${query.state.dataUpdatedAt}:${query.state.dataUpdateCount}`,
      )
      .join("|"),
    repositoriesByWorkspace,
  };
}
