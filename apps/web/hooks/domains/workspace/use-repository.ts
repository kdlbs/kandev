import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import type { Repository } from '@/lib/types/http';

export function useRepository(repositoryId: string | null) {
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);

  return useMemo(() => {
    if (!repositoryId) return null;
    const repositories = Object.values(repositoriesByWorkspace).flat() as Repository[];
    return repositories.find((repo: Repository) => repo.id === repositoryId) ?? null;
  }, [repositoriesByWorkspace, repositoryId]);
}
