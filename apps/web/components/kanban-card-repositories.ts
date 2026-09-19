"use client";

import { useAppStore } from "@/components/state-provider";
import type { RepositoryChip, Task } from "@/components/kanban-card";
import { repositorySlug } from "@/lib/repository-slug";
import { formatUserHomePath } from "@/lib/utils";
import { repositoryId as toRepositoryId } from "@/lib/types/http";
import type { Repository } from "@/lib/types/http";

const EMPTY_REPOSITORIES: Repository[] = [];

export function useActiveWorkspaceRepositories() {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  return useAppStore((state) =>
    activeWorkspaceId
      ? (state.repositories.itemsByWorkspaceId[activeWorkspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
}

/** Resolves linked repositories to card chips, primary first. */
export function resolveTaskRepositoryChips(
  task: Task,
  repositories: Repository[],
): RepositoryChip[] {
  const byId = new Map(repositories.map((repo) => [repo.id, repo]));
  const seen = new Set<string>();
  const chips: RepositoryChip[] = [];
  const push = (id: string | undefined) => {
    if (!id || seen.has(id)) return;
    const repo = byId.get(toRepositoryId(id));
    if (!repo) return;
    seen.add(id);
    const label = repositorySlug(repo);
    if (!label) return;
    chips.push({
      label,
      ...(repo.local_path ? { path: formatUserHomePath(repo.local_path) } : {}),
    });
  };
  push(task.repositoryId);
  const ordered = [...(task.repositories ?? [])].sort((a, b) => a.position - b.position);
  for (const link of ordered) push(link.repository_id);
  return chips;
}
