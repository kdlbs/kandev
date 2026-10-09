import type { Repository } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/store";

export type KanbanTask = KanbanState["tasks"][number];

// Minimal task membership shared by repository filtering and search.
export type FilterableTask = {
  id: string;
  repositoryId?: string;
  repositories?: ReadonlyArray<{ repository_id: string }>;
};

export function getTaskRepositoryIds(task: FilterableTask): string[] {
  return (
    task.repositories?.map((repository) => repository.repository_id) ??
    (task.repositoryId ? [task.repositoryId] : [])
  );
}

export type RepositorySearchLookup = ReadonlyMap<string, Pick<Repository, "name" | "local_path">>;

export function taskMatchesRepositorySearch(
  task: FilterableTask,
  query: string,
  repositoriesById?: RepositorySearchLookup,
): boolean {
  return getTaskRepositoryIds(task).some((id) => {
    const repository = repositoriesById?.get(id);
    return Boolean(
      repository?.name?.toLowerCase().includes(query) ||
      repository?.local_path?.toLowerCase().includes(query),
    );
  });
}

export function mapSelectedRepositoryIds(
  repositories: Repository[],
  selectedIds: string[],
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
  selectedRepositoryIds: Set<string>,
): T[] {
  if (selectedRepositoryIds.size === 0) {
    return tasks;
  }
  return tasks.filter((task) =>
    getTaskRepositoryIds(task).some((repositoryId) => selectedRepositoryIds.has(repositoryId)),
  );
}
