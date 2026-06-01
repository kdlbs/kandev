import { useTask } from "@/hooks/use-task";
import { useAllRepositories } from "@/hooks/domains/workspace/use-all-repositories";
import type { Repository } from "@/lib/types/http";

export function useTaskRepositories(taskId: string | null) {
  const task = useTask(taskId);
  // Observe cached repo lists across workspaces (no fetch).
  const { repositories } = useAllRepositories(false);

  if (!task?.repositoryId) return [];
  return repositories.filter((repo: Repository) => repo.id === task.repositoryId);
}
