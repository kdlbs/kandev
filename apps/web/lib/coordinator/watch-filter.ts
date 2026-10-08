import type { AttentionStall, AttentionTask } from "@/lib/coordinator/attention";

/** The effective watch set of a coordinator: every board, or the listed ones. */
export type WatchSet = { scope: "all" | "selected"; workflowIds: readonly string[] };

/** True when the set is `all`, or the task's workflow is in the set. A task with no workflow is never watched. */
export function isTaskWatched(task: AttentionTask, watchSet: WatchSet): boolean {
  if (!task.workflowId) return false;
  if (watchSet.scope === "all") return true;
  return watchSet.workflowIds.includes(task.workflowId);
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
