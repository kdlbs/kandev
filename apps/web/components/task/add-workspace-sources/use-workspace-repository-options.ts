"use client";

import { useCallback } from "react";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useRepositoryDiscovery } from "@/hooks/domains/workspace/use-repository-discovery";

export function useWorkspaceRepositoryOptions(workspaceId: string | null, open: boolean) {
  const {
    repositories,
    isLoading: repositoriesLoading,
    refresh: refreshRepositories,
  } = useRepositories(workspaceId, open);
  const discovery = useRepositoryDiscovery(workspaceId, open);

  const refreshRepositoryOptions = useCallback(() => {
    void Promise.all([refreshRepositories(), discovery.refresh()]);
  }, [discovery.refresh, refreshRepositories]);

  return {
    repositories,
    discoveredRepositories: discovery.repositories,
    repositoriesRefreshing: repositoriesLoading || discovery.isLoading || discovery.isRefreshing,
    refreshRepositoryOptions,
  };
}
