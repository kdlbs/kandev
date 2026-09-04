import { compareStepOrder, type StepOrderTask } from "./task-order";

export type WipQueueTask = StepOrderTask & {
  workflowStepId: string;
  queuedForStepId?: string | null;
  wipAdmitted?: boolean | null;
};

export type WipQueueEntry<T extends WipQueueTask = WipQueueTask> = {
  task: T;
  position: number;
  total: number;
};

export type WipQueueStatus = {
  position: number;
  total: number;
  destinationTitle: string;
};

// Delegates to task-order.ts's AC.1 total ordering key so the overflow queue
// and every other board/promotion comparator (REQ-TASKS-KANBAN-TASK-REORDERING-001.36)
// cannot drift apart.
export function compareWipQueueTasks(left: WipQueueTask, right: WipQueueTask): number {
  return compareStepOrder(left, right);
}

function isDestinationQueued(task: WipQueueTask, destinationStepId: string): boolean {
  return (
    task.workflowStepId === destinationStepId &&
    task.queuedForStepId === destinationStepId &&
    task.wipAdmitted !== true
  );
}

export function getDestinationQueue<T extends WipQueueTask>(
  tasks: T[],
  destinationStepId: string,
): WipQueueEntry<T>[] {
  const queued = tasks.filter((task) => isDestinationQueued(task, destinationStepId));
  queued.sort(compareWipQueueTasks);
  return queued.map((task, index) => ({ task, position: index + 1, total: queued.length }));
}

export function partitionWipTasks<T extends WipQueueTask>(
  tasks: T[],
  destinationStepId: string,
): { admitted: T[]; queued: T[] } {
  const queuedEntries = getDestinationQueue(tasks, destinationStepId);
  const queuedIds = new Set(queuedEntries.map(({ task }) => task.id));
  return {
    admitted: tasks.filter((task) => !queuedIds.has(task.id)),
    queued: queuedEntries.map(({ task }) => task),
  };
}

export function getWipQueueStatus<T extends WipQueueTask>(
  task: T,
  tasks: T[],
  destinationStepId: string,
  destinationTitle: string,
): WipQueueStatus | undefined {
  const entry = getDestinationQueue(tasks, destinationStepId).find(
    ({ task: candidate }) => candidate.id === task.id,
  );
  if (!entry) return undefined;
  return { position: entry.position, total: entry.total, destinationTitle };
}
