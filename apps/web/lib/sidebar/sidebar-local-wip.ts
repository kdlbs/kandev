import type { LocalSidebarTask } from "./sidebar-local-projection";
import { sqliteBinary, sqliteTaskTime } from "./sidebar-local-order";

const priority: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3, none: 4 };

export function localSidebarQueues(tasks: LocalSidebarTask[]) {
  const steps = new Map<string, LocalSidebarTask[]>();
  for (const task of tasks) {
    const overview = task.overview;
    if (
      task.isArchived ||
      overview.wipAdmitted ||
      !overview.queuedForStepId ||
      overview.queuedForStepId !== task.workflowStepId
    )
      continue;
    const queue = steps.get(overview.queuedForStepId) ?? [];
    queue.push(task);
    steps.set(overview.queuedForStepId, queue);
  }
  const result = new Map<string, { position: number; total: number }>();
  for (const queue of steps.values()) {
    queue.sort(
      ({ overview: a }, { overview: b }) =>
        a.position - b.position ||
        (priority[(a.priority ?? "none").toLowerCase()] ?? 4) -
          (priority[(b.priority ?? "none").toLowerCase()] ?? 4) ||
        sqliteBinary(
          sqliteTaskTime(a.queuedAt ?? a.createdAt),
          sqliteTaskTime(b.queuedAt ?? b.createdAt),
        ) ||
        sqliteBinary(sqliteTaskTime(a.createdAt), sqliteTaskTime(b.createdAt)) ||
        sqliteBinary(a.id, b.id),
    );
    queue.forEach((task, index) =>
      result.set(task.id, { position: index + 1, total: queue.length }),
    );
  }
  return result;
}
