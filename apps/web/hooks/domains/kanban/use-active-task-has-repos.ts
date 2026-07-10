import { useAppStore } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices";
import { useTask } from "@/hooks/use-task";

type RepoBearingTask = Pick<KanbanState["tasks"][number], "repositories" | "repositoryId">;

export function taskHasRepositories(task: RepoBearingTask | null | undefined): boolean {
  return Boolean(task?.repositoryId || (task?.repositories && task.repositories.length > 0));
}

/**
 * Repo-status of the active task, distinguishing three states:
 *   - `null`   — unknown (no active task, or task hasn't loaded into the
 *                store yet); callers must not act on this.
 *   - `true`   — task has at least one repository.
 *   - `false`  — task is confirmed repo-less.
 *
 * Returning a boolean would conflate "loading" with "no repos", which would
 * cause auto-close effects (e.g. the Changes panel) to tear panels down on
 * first render before the task data arrives — `removePanel` is permanent
 * within a session, so the user couldn't recover them.
 */
export function useActiveTaskHasRepos(): boolean | null {
  const taskId = useAppStore((s) => s.tasks.activeTaskId);
  const task = useTask(taskId);
  if (!task) return null;
  return taskHasRepositories(task);
}
