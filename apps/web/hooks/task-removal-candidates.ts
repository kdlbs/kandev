import type { KanbanState } from "@/lib/state/slices";
import type { Task } from "@/lib/types/http";
import { fetchTask } from "@/lib/api";
import { getRecentTasks } from "@/lib/recent-tasks";

/**
 * Orders next-task candidates by recent use, then board order, without trusting
 * either list as proof that a task still exists.
 */
export function orderedTaskCandidates(
  remainingTasks: KanbanState["tasks"],
  removedTaskId: string,
  excludedTaskIds?: ReadonlySet<string>,
  workspaceId?: string | null,
  isTaskPendingRemoval?: (taskId: string) => boolean,
): KanbanState["tasks"] {
  const candidates = remainingTasks.filter(
    (task) =>
      task.id !== removedTaskId &&
      !excludedTaskIds?.has(task.id) &&
      isTaskPendingRemoval?.(task.id) !== true &&
      (!workspaceId || task.workspaceId === workspaceId),
  );
  const remainingById = new Map(candidates.map((task) => [task.id, task]));
  const ordered: KanbanState["tasks"] = [];
  for (const recent of getRecentTasks()) {
    const task = remainingById.get(recent.taskId);
    if (!task) continue;
    ordered.push(task);
    remainingById.delete(task.id);
  }
  ordered.push(...candidates.filter((task) => remainingById.has(task.id)));
  return ordered;
}

export async function taskIsLive(taskId: string): Promise<boolean> {
  try {
    const task = await fetchTask(taskId, { cache: "no-store" });
    return !task.archived_at;
  } catch {
    return false;
  }
}

export async function taskHasSafeRemovalAncestry(
  taskId: string,
  excludedTaskIds: ReadonlySet<string>,
  workspaceId?: string | null,
): Promise<boolean> {
  const visited = new Set<string>();
  let currentTaskId: string | null = taskId;

  while (currentTaskId) {
    if (visited.has(currentTaskId)) return false;
    visited.add(currentTaskId);

    let task: Task;
    try {
      task = await fetchTask(currentTaskId, { cache: "no-store" });
    } catch {
      return false;
    }
    if (task.archived_at || (workspaceId && task.workspace_id !== workspaceId)) return false;

    const parentId = task.parent_id ?? null;
    if (!parentId) return true;
    if (excludedTaskIds.has(parentId)) return false;
    currentTaskId = parentId;
  }

  return false;
}

/**
 * Picks the first candidate that the task API still reports as unarchived.
 * Workflow snapshots and the active kanban are both cached projections and can
 * lag the same delete/archive event, so neither can independently validate
 * membership. `isTaskPendingRemoval` re-reads local accepted removals so a
 * cached successful liveness read cannot overrule newer pending intent.
 */
export async function selectNextTaskAfterRemoval(
  remainingTasks: KanbanState["tasks"],
  removedTaskId: string,
  isLive: (taskId: string) => Promise<boolean> = taskIsLive,
  options?: {
    excludedTaskIds?: ReadonlySet<string>;
    workspaceId?: string | null;
    validateTaskAncestry?: boolean;
    isTaskPendingRemoval?: (taskId: string) => boolean;
    /** Live ancestry check for removals observed while the attempt runs. */
    isTaskAncestrySafe?: (taskId: string) => Promise<boolean>;
  },
): Promise<KanbanState["tasks"][number] | null> {
  const {
    excludedTaskIds,
    workspaceId,
    validateTaskAncestry = false,
    isTaskPendingRemoval,
    isTaskAncestrySafe,
  } = options ?? {};
  const checksAncestry = isTaskAncestrySafe !== undefined || validateTaskAncestry;
  const ancestryExclusions = excludedTaskIds ?? new Set<string>();
  const candidates = orderedTaskCandidates(
    remainingTasks,
    removedTaskId,
    excludedTaskIds,
    workspaceId,
    isTaskPendingRemoval,
  );
  for (const task of candidates) {
    if (checksAncestry) {
      const ancestrySafe = isTaskAncestrySafe
        ? await isTaskAncestrySafe(task.id)
        : await taskHasSafeRemovalAncestry(task.id, ancestryExclusions, workspaceId);
      if (!ancestrySafe || isTaskPendingRemoval?.(task.id) === true) continue;
    }
    if (!(await isLive(task.id))) continue;
    if (isTaskPendingRemoval?.(task.id) === true) continue;
    return task;
  }
  return null;
}
