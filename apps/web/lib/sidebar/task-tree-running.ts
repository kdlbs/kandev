import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";

function taskIsRunning(task: TaskSwitcherItem): boolean {
  return typeof task.hasRunningSession === "boolean"
    ? task.hasRunningSession
    : task.sessionState === "RUNNING";
}

/** Aggregates task-wide RUNNING state from included descendants. */
export function resolveTaskTreeRunning(
  tasks: TaskSwitcherItem[],
  subTasksByParentId: Map<string, TaskSwitcherItem[]>,
): Map<string, boolean> {
  const taskIds = new Set(tasks.map((task) => task.id));
  const runningById = new Map(tasks.map((task) => [task.id, taskIsRunning(task)]));
  const parentsById = new Map<string, string[]>();
  for (const task of tasks) parentsById.set(task.id, []);
  for (const [parentId, children] of subTasksByParentId) {
    if (!taskIds.has(parentId)) continue;
    for (const child of children) {
      if (!taskIds.has(child.id)) continue;
      const parents = parentsById.get(child.id)!;
      if (!parents.includes(parentId)) parents.push(parentId);
    }
  }

  const pending = tasks.filter(taskIsRunning).map((task) => task.id);
  for (let index = 0; index < pending.length; index += 1) {
    for (const parentId of parentsById.get(pending[index]) ?? []) {
      if (runningById.get(parentId)) continue;
      runningById.set(parentId, true);
      pending.push(parentId);
    }
  }
  return runningById;
}
