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
    inFlightRef.current = true;
    setRepositoryBranchesLoading(repositoryId, true);
    // Capture the repositoryId at effect start for the closure
    const fetchRepoId = repositoryId;
    listRepositoryBranches(fetchRepoId, { cache: 'no-store' })
      .then((response) => {
        // Safe to always store: data is keyed by repositoryId in the store
        setRepositoryBranches(fetchRepoId, response.branches);
      })
      .catch(() => {
        setRepositoryBranches(fetchRepoId, []);
      })
      .finally(() => {
        inFlightRef.current = false;
        setRepositoryBranchesLoading(fetchRepoId, false);
      });
  }, [enabled, isLoaded, repositoryId, setRepositoryBranches, setRepositoryBranchesLoading]);

  return { branches, isLoading };
}
