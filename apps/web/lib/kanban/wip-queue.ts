import type { TaskPriority } from "@/lib/types/http";

export type WipQueueTask = {
  id: string;
  workflowStepId: string;
  queuedForStepId?: string | null;
  wipAdmitted?: boolean | null;
  position?: number | null;
  priority?: TaskPriority | null;
  queuedAt?: string | null;
  createdAt?: string | null;
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

function priorityRank(priority: WipQueueTask["priority"]): number {
  switch (priority) {
    case "critical":
      return 0;
    case "high":
      return 1;
    case "medium":
      return 2;
    case "low":
      return 3;
    default:
      return 4;
  }
}

function timestamp(value: string | null | undefined): number {
  if (!value) return Number.POSITIVE_INFINITY;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : Number.POSITIVE_INFINITY;
}

function compareNumbers(left: number, right: number): number {
  if (left < right) return -1;
  if (left > right) return 1;
  return 0;
}

export function compareWipQueueTasks(left: WipQueueTask, right: WipQueueTask): number {
  const position = (left.position ?? 0) - (right.position ?? 0);
  if (position !== 0) return position;

  const priority = priorityRank(left.priority) - priorityRank(right.priority);
  if (priority !== 0) return priority;

  const queuedAt = compareNumbers(timestamp(left.queuedAt), timestamp(right.queuedAt));
  if (queuedAt !== 0) return queuedAt;

  const createdAt = compareNumbers(timestamp(left.createdAt), timestamp(right.createdAt));
  if (createdAt !== 0) return createdAt;

  return left.id.localeCompare(right.id);
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
