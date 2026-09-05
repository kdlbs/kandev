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
  const [error, setError] = useState<Error | null>(null);
  const currentWorkspaceRef = useRef(workspaceId);
  currentWorkspaceRef.current = workspaceId;

  const discoverRepositories = useCallback(async () => {
    if (!workspaceId) {
      setDiscoveredRepositories([]);
      setError(null);
      return;
    }
    const requestedWorkspaceId = workspaceId;
    setRepositoriesDiscovering(true);
    setError(null);
    try {
      const result = await discoverRepositoriesAction(requestedWorkspaceId);
      if (currentWorkspaceRef.current === requestedWorkspaceId) {
        setDiscoveredRepositories(result.repositories);
        setError(null);
      }
    } catch (cause) {
      if (currentWorkspaceRef.current === requestedWorkspaceId) {
        setDiscoveredRepositories([]);
        setError(cause instanceof Error ? cause : new Error(String(cause)));
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
    error,
    refreshRepositoryOptions,
  };
}
