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
 *
 * Multi-branch tasks: agentctl tags per-repo tracker events with the
 * subdir name (e.g. `kandev-branch-2` for a sibling worktree), which is
 * fine on disk but ugly in the UI. When the subdir matches `<repoName>-<slug>`
 * of a repo known to this workspace, the resolver formats it as
 * `<repoName> · <slug>` to match how PR CHANGES labels the same groups
 * — consistent visual language across both sections.
 */
export function useRepoDisplayName(sessionId: string | null | undefined) {
  const session = useAppStore((state) => (sessionId ? state.taskSessions.items[sessionId] : null));
  const taskId = session?.task_id ?? null;
  const tasks = useAppStore((state) => state.kanban.tasks);
  const reposByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const primaryName = useMemo(
    () =>
      resolvePrimaryRepoName(
        taskId,
        tasks as unknown as TaskLike[],
        reposByWorkspace as unknown as Record<string, RepoEntry[]>,
      ),
    [taskId, tasks, reposByWorkspace],
  );
  // Flattened, sorted list of known repo names — long names first so
  // `kandev-foo` matches `kandev-foo` before it matches `kandev`.
  const knownRepoNames = useMemo(() => {
    const set = new Set<string>();
    for (const list of Object.values(reposByWorkspace as Record<string, RepoEntry[]>)) {
      for (const r of list) {
        if (r.name) set.add(r.name);
      }
    }
    return Array.from(set).sort((a, b) => b.length - a.length);
  }, [reposByWorkspace]);
  return useMemo(
    () => (repositoryName: string) => formatRepoLabel(repositoryName, primaryName, knownRepoNames),
    [primaryName, knownRepoNames],
  );
}

/**
 * formatRepoLabel produces the UI label for a repository_name reported by
 * agentctl. Multi-branch subdir tags (`<repo>-<slug>`) get unified with the
 * PR CHANGES `<repo> · <slug>` formatting so commits and PR groups render
 * with the same visual structure.
 */
function formatRepoLabel(
  repositoryName: string,
  primaryName: string | undefined,
  knownRepoNames: string[],
): string | undefined {
  if (!repositoryName) return primaryName || undefined;
  // Exact match first so a bare hyphenated repo (e.g. `kandev-cli`) isn't
  // split by a shorter known prefix (`kandev-`) into `kandev · cli`.
  if (knownRepoNames.includes(repositoryName)) return repositoryName;
  for (const known of knownRepoNames) {
    const prefix = known + "-";
    if (repositoryName.length > prefix.length && repositoryName.startsWith(prefix)) {
      return `${known} · ${repositoryName.slice(prefix.length)}`;
    }
  }
  return repositoryName;
}
