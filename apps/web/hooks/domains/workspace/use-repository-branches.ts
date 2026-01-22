import { useEffect, useRef } from 'react';
import { useAppStore } from '@/components/state-provider';
import { listRepositoryBranches } from '@/lib/api';
import type { Branch } from '@/lib/types/http';

const EMPTY_BRANCHES: Branch[] = [];

export function useRepositoryBranches(repositoryId: string | null, enabled = true) {
  const branches = useAppStore((state) =>
    repositoryId
      ? state.repositoryBranches.itemsByRepositoryId[repositoryId] ?? EMPTY_BRANCHES
      : EMPTY_BRANCHES
  );
  const isLoaded = useAppStore((state) =>
    repositoryId ? state.repositoryBranches.loadedByRepositoryId[repositoryId] ?? false : false
  );
  const isLoading = useAppStore((state) =>
    repositoryId ? state.repositoryBranches.loadingByRepositoryId[repositoryId] ?? false : false
  );
  const setRepositoryBranches = useAppStore((state) => state.setRepositoryBranches);
  const setRepositoryBranchesLoading = useAppStore((state) => state.setRepositoryBranchesLoading);
  const inFlightRef = useRef(false);

  useEffect(() => {
    if (!enabled || !repositoryId) return;
    if (isLoaded || inFlightRef.current) return;
    let cancelled = false;
    inFlightRef.current = true;
    setRepositoryBranchesLoading(repositoryId, true);
    listRepositoryBranches(repositoryId, { cache: 'no-store' })
      .then((response) => {
        if (cancelled) return;
        setRepositoryBranches(repositoryId, response.branches);
      })
      .catch(() => {
        if (cancelled) return;
        setRepositoryBranches(repositoryId, []);
      })
      .finally(() => {
        inFlightRef.current = false;
        if (cancelled) return;
        setRepositoryBranchesLoading(repositoryId, false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, isLoaded, repositoryId, setRepositoryBranches, setRepositoryBranchesLoading]);

  return { branches, isLoading };
}
