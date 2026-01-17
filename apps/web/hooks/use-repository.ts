import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';

export function useRepository(repositoryId: string | null) {
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);

  return useMemo(() => {
    if (!repositoryId) return null;
    const repositories = Object.values(repositoriesByWorkspace).flat();
    return repositories.find((repo) => repo.id === repositoryId) ?? null;
  }, [repositoriesByWorkspace, repositoryId]);
}
