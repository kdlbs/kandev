"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";

/**
 * Resolves the workspace's primary single-repo name from the active task and
 * repositories slice. Returns undefined when:
 * - the task hasn't loaded yet (Bug 5 loading-order concern: `tasks` and
 *   `reposByWorkspace` hydrate independently via SSR + WS, so the task can be
 *   missing on first render)
 * - the task has multiple repos (the empty-name fallback would mislabel any
 *   untagged row as the primary repo)
 * - no matching repo entry was found in any workspace
 *
 * Extracted so the resolver returned by `useRepoDisplayName` can be a stable
 * closure over a single primitive and React Compiler can preserve memoization.
 */
type TaskLike = {
  id: string;
  repositoryId?: string | null;
  repositories?: unknown[];
};
type RepoEntry = { id: string; name: string };

function resolvePrimaryRepoName(
  taskId: string | null,
  tasks: TaskLike[],
  reposByWorkspace: Record<string, RepoEntry[]>,
): string | undefined {
  const task = taskId ? tasks.find((t) => t.id === taskId) : undefined;
  if (task === undefined) return undefined;
  const primaryRepoId = task.repositoryId ?? null;
  const taskHasMultipleRepos = (task.repositories?.length ?? 0) > 1;
  if (taskHasMultipleRepos || !primaryRepoId) return undefined;
  for (const list of Object.values(reposByWorkspace)) {
    const found = list.find((r) => r.id === primaryRepoId);
    if (found) return found.name;
  }
  return undefined;
}

/**
 * Resolves a repository_name (as reported by agentctl in git status) to a
 * human-readable label for the UI. Non-empty inputs pass through unchanged;
 * empty inputs fall back to the workspace's primary repo name when safely
 * resolvable, otherwise undefined (callers render a neutral "Repository").
 */
export function useRepoDisplayName(sessionId: string | null | undefined) {
  const session = useAppStore((state) => (sessionId ? state.taskSessions.items[sessionId] : null));
  const taskId = session?.task_id ?? null;
  const tasks = useAppStore((state) => state.kanban.tasks);
  const reposByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  // Resolve to a single primitive (or undefined) so the returned closure is
  // memoized cleanly without React Compiler bailing on a branched return.
  const primaryName = useMemo(
    () =>
      resolvePrimaryRepoName(
        taskId,
        tasks as unknown as TaskLike[],
        reposByWorkspace as unknown as Record<string, RepoEntry[]>,
      ),
    [taskId, tasks, reposByWorkspace],
  );
  return useMemo(
    () => (repositoryName: string) => repositoryName || primaryName || undefined,
    [primaryName],
  );
}
