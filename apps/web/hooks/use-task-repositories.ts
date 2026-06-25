import { useAllCachedRepositories } from "@/hooks/domains/workspace/use-repository-cache";
import { useTask } from "@/hooks/use-task";

export function useTaskRepositories(taskId: string | null) {
  const task = useTask(taskId);
  const repositories = useAllCachedRepositories();

  if (!task?.repositoryId) return [];
  return repositories.filter((repo) => repo.id === task.repositoryId);
}
