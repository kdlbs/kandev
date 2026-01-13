import type { Repository } from '@/lib/types/http';
import type { KanbanState } from '@/lib/state/store';

export type KanbanTask = KanbanState['tasks'][number];

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

export function filterTasksByRepositories(
  tasks: KanbanTask[],
  selectedRepositoryPaths: Set<string>
): KanbanTask[] {
  if (selectedRepositoryPaths.size === 0) {
    return tasks;
  }
  return tasks.filter((task) => task.repositoryUrl && selectedRepositoryPaths.has(task.repositoryUrl));
}
