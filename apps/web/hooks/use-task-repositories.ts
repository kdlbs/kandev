import { useAppStore } from '@/components/state-provider';
import { useTask } from '@/hooks/use-task';
import type { Repository } from '@/lib/types/http';

export function useTaskRepositories(taskId: string | null) {
  const task = useTask(taskId);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);

  if (!task?.repositoryId) return [];
  const repositories = Object.values(repositoriesByWorkspace).flat() as Repository[];
  return repositories.filter((repo: Repository) => repo.id === task.repositoryId);
}
