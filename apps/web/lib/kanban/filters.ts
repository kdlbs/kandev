import type { Repository } from '@/lib/types/http';
import type { KanbanState } from '@/lib/state/store';

export type KanbanTask = KanbanState['tasks'][number];

// Minimal task type for filtering - only needs id and repositoryId
export type FilterableTask = {
  id: string;
  repositoryId?: string;
};

export function mapSelectedRepositoryIds(
  repositories: Repository[],
  selectedIds: string[]
): Set<string> {
  if (selectedIds.length === 0) {
    return new Set();
  }
  const repoIds = new Set<string>();
  repositories.forEach((repo) => {
    if (selectedIds.includes(repo.id)) {
      repoIds.add(repo.id);
    }
  });
  return repoIds;
}

export function filterTasksByRepositories<T extends FilterableTask>(
  tasks: T[],
  selectedRepositoryIds: Set<string>
): T[] {
  if (selectedRepositoryIds.size === 0) {
    return tasks;
  }
  return tasks.filter((task) => task.repositoryId && selectedRepositoryIds.has(task.repositoryId));
}
