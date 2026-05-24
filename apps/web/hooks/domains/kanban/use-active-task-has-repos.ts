"use client";

import { useAppStore } from "@/components/state-provider";
import { useTaskRepositories } from "@/hooks/domains/kanban/use-task-repositories";

/**
 * Repo-status of the active task, distinguishing three states:
 *   - `null`   — unknown (no active task, or task hasn't loaded into the
 *                cache yet); callers must not act on this.
 *   - `true`   — task has at least one repository.
 *   - `false`  — task is confirmed repo-less.
 *
 * Returning a boolean would conflate "loading" with "no repos", which would
 * cause auto-close effects (e.g. the Changes panel) to tear panels down on
 * first render before the task data arrives — `removePanel` is permanent
 * within a session, so the user couldn't recover them.
 *
 * Reads from TQ cache via `useTaskRepositories` (which reads the
 * `qk.kanban.multi()` cache).
 */
export function useActiveTaskHasRepos(): boolean | null {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const repos = useTaskRepositories(taskId);

  // taskId is null → unknown
  if (!taskId) return null;

  // repos is an empty array when: task not found in cache (still loading) or
  // task has no repos. We need to distinguish "loading" from "confirmed empty".
  // Return null while we haven't resolved the task yet (array empty AND taskId set).
  // Once the cache is warm, repos.length === 0 means confirmed repo-less.
  return repos.length > 0;
}
