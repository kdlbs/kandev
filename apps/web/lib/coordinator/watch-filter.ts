import type { AttentionStall, AttentionTask } from "@/lib/coordinator/attention";

/**
 * The resolved Projects scope of a coordinator under `selected`: the
 * repositories that count as in scope (sets already expanded), or `null` when
 * that read failed, which watches nothing. Absent means every project.
 */
export type WatchProjects = {
  scope: "selected";
  repositoryIds: readonly string[] | null;
  includeNoRepository: boolean;
};

/** The effective watch set of a coordinator: every board, or the listed ones, narrowed by Projects. */
export type WatchSet = {
  scope: "all" | "selected";
  workflowIds: readonly string[];
  projects?: WatchProjects;
};

/** Whether a repository list falls in the Projects scope; an unknown list on either side is outside. */
export function repositoriesInProjects(
  taskRepos: readonly string[] | null | undefined,
  projects: WatchProjects | undefined,
): boolean {
  if (!projects) return true;
  const inScope = projects.repositoryIds;
  if (inScope === null || taskRepos === null || taskRepos === undefined) return false;
  if (taskRepos.length === 0) return projects.includeNoRepository;
  return taskRepos.some((id) => inScope.includes(id));
}

/**
 * True when the task is in the boards scope and the Projects scope. A task with
 * no workflow is never watched; a task whose repositories are unknown is
 * outside a selected Projects scope.
 */
export function isTaskWatched(task: AttentionTask, watchSet: WatchSet): boolean {
  if (!task.workflowId) return false;
  if (watchSet.scope !== "all" && !watchSet.workflowIds.includes(task.workflowId)) return false;
  return repositoriesInProjects(task.repositoryIds, watchSet.projects);
}

/** Keeps the watched tasks and the stalls whose task is among them; a stall of an absent task is dropped. */
export function filterWatched(
  input: { tasks: AttentionTask[]; stalls: AttentionStall[] },
  watchSet: WatchSet,
): { tasks: AttentionTask[]; stalls: AttentionStall[] } {
  const tasks = input.tasks.filter((task) => isTaskWatched(task, watchSet));
  const ids = new Set(tasks.map((task) => task.id));
  return { tasks, stalls: input.stalls.filter((stall) => ids.has(stall.task_id)) };
}
