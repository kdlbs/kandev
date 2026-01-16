import { useEffect, useRef } from 'react';
import { useAppStore } from '@/components/state-provider';
import type { Repository } from '@/lib/types/http';
import { listRepositories } from '@/lib/http';

const EMPTY_REPOSITORIES: Repository[] = [];

export function useRepositories(workspaceId: string | null, enabled = true) {
  const repositories = useAppStore((state) =>
    workspaceId
      ? state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES
      : EMPTY_REPOSITORIES
  );
  const isLoading = useAppStore((state) =>
    workspaceId ? state.repositories.loadingByWorkspaceId[workspaceId] ?? false : false
  );
  const isLoaded = useAppStore((state) =>
    workspaceId ? state.repositories.loadedByWorkspaceId[workspaceId] ?? false : false
  );
  const setRepositories = useAppStore((state) => state.setRepositories);
  const setRepositoriesLoading = useAppStore((state) => state.setRepositoriesLoading);
  const inFlightRef = useRef(false);

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    if (isLoaded && isLoading) {
      setRepositoriesLoading(workspaceId, false);
    }
  }, [enabled, isLoaded, isLoading, setRepositoriesLoading, workspaceId]);

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    if (isLoaded || inFlightRef.current) return;
    let cancelled = false;
    inFlightRef.current = true;
    setRepositoriesLoading(workspaceId, true);
    listRepositories(workspaceId, { cache: 'no-store' })
      .then((response) => {
        if (cancelled) return;
        setRepositories(workspaceId, response.repositories);
      })
      .catch(() => {
        if (cancelled) return;
        setRepositories(workspaceId, []);
      })
      .finally(() => {
        inFlightRef.current = false;
        if (cancelled) return;
        setRepositoriesLoading(workspaceId, false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, isLoaded, setRepositories, setRepositoriesLoading, workspaceId]);

  return { repositories, isLoading };
}
