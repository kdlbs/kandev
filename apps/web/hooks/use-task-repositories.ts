import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import { useTask } from '@/hooks/use-task';

export function useTaskRepositories(taskId: string | null) {
  const task = useTask(taskId);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);

  return useMemo(() => {
    if (!task?.repositoryId) return [];
    const repositories = Object.values(repositoriesByWorkspace).flat();
    return repositories.filter((repo) => repo.id === task.repositoryId);
  }, [repositoriesByWorkspace, task?.repositoryId]);
}
