import { useAppStore } from "@/components/state-provider";
import { useTask } from "@/hooks/use-task";

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
  return Boolean(task.repositories && task.repositories.length > 0);
}
