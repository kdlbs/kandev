"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { discoverRepositoriesAction } from "@/app/actions/workspaces";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import type { LocalRepository } from "@/lib/types/http";

export function useWorkspaceRepositoryOptions(workspaceId: string | null, open: boolean) {
  const {
    repositories,
    isLoading: repositoriesLoading,
    refresh: refreshRepositories,
  } = useRepositories(workspaceId, open);
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [repositoriesDiscovering, setRepositoriesDiscovering] = useState(false);
  const currentWorkspaceRef = useRef(workspaceId);
  currentWorkspaceRef.current = workspaceId;

  const discoverRepositories = useCallback(async () => {
    if (!workspaceId) {
      setDiscoveredRepositories([]);
      return;
    }
    const requestedWorkspaceId = workspaceId;
    setRepositoriesDiscovering(true);
    try {
      const result = await discoverRepositoriesAction(requestedWorkspaceId);
      if (currentWorkspaceRef.current === requestedWorkspaceId) {
        setDiscoveredRepositories(result.repositories);
      }
    } catch {
      if (currentWorkspaceRef.current === requestedWorkspaceId) {
        setDiscoveredRepositories([]);
      }
    } finally {
      if (currentWorkspaceRef.current === requestedWorkspaceId) {
        setRepositoriesDiscovering(false);
      }
    }
  }, [workspaceId]);

  useEffect(() => {
    if (open) void discoverRepositories();
  }, [discoverRepositories, open]);

  const refreshRepositoryOptions = useCallback(() => {
    void Promise.all([refreshRepositories(), discoverRepositories()]);
  }, [discoverRepositories, refreshRepositories]);

  return {
    repositories,
    discoveredRepositories,
    repositoriesRefreshing: repositoriesLoading || repositoriesDiscovering,
    refreshRepositoryOptions,
  };
}
