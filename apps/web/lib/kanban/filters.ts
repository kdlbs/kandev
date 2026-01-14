import type { Repository } from '@/lib/types/http';
import type { KanbanState } from '@/lib/state/store';

export type KanbanTask = KanbanState['tasks'][number];

// Minimal task type for filtering - only needs id and repositoryUrl
export type FilterableTask = {
  id: string;
  repositoryUrl?: string;
};

export function mapSelectedRepositoryPaths(
  repositories: Repository[],
  selectedIds: string[]
): Set<string> {
  if (selectedIds.length === 0) {
    return new Set();
  }
  const repoById = new Map(repositories.map((repo) => [repo.id, repo]));
  const paths = new Set<string>();
  selectedIds.forEach((id) => {
    const repo = repoById.get(id);
    if (repo?.local_path) {
      paths.add(repo.local_path);
    }
  });
  return paths;
}

export function filterTasksByRepositories<T extends FilterableTask>(
  tasks: T[],
  selectedRepositoryPaths: Set<string>
): T[] {
  if (selectedRepositoryPaths.size === 0) {
    return tasks;
  }
  return tasks.filter((task) => task.repositoryUrl && selectedRepositoryPaths.has(task.repositoryUrl));
}
