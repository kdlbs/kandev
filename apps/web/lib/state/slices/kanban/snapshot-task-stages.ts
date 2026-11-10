import type { TaskOverview, TaskOverviewState } from "../task-overview-types";
import { isStaleTaskOverviewPatch, recordTaskOverviewChange } from "../task-overview-merge";

export function journalSnapshotTaskStages(
  overview: TaskOverviewState,
  incoming: TaskOverview[],
  previous: TaskOverview[] | undefined,
): TaskOverviewState {
  const before = new Map(previous?.map((task) => [task.id, task]));
  let next = overview;
  for (const task of incoming) {
    if (
      task === before.get(task.id) ||
      !Object.hasOwn(task, "workflowStepId") ||
      isStaleTaskOverviewPatch(overview.byId[task.id], task)
    )
      continue;
    // A replacement snapshot row asserts its stage even when the value is equal.
    next = recordTaskOverviewChange(next, task.id, {
      id: task.id,
      workflowStepId: task.workflowStepId,
    });
  }
  return next;
}
