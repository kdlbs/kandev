import type { TaskPriority } from "@/lib/types/http";

type CreatedTask = {
  createdAt?: string;
};

function createdAtTime(task: CreatedTask): number {
  if (!task.createdAt) return Number.NEGATIVE_INFINITY;
  const time = Date.parse(task.createdAt);
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time;
}

export function compareTasksByCreatedDesc(a: CreatedTask, b: CreatedTask): number {
  const aTime = createdAtTime(a);
  const bTime = createdAtTime(b);
  if (bTime > aTime) return 1;
  if (bTime < aTime) return -1;
  return 0;
}

/**
 * Sort `ids` into the board's visible created-desc order using `taskById` for
 * lookups. Ids without a known task keep their relative order. Used before a
 * kanban bulk move so a backward range selection doesn't land scrambled when
 * sequential positions are assigned.
 */
export function sortIdsByCreatedDesc(ids: string[], taskById: Map<string, CreatedTask>): string[] {
  // Missing ids fall back to `{}`, which `compareTasksByCreatedDesc` treats as
  // the oldest (sorts last) — keeping the comparator transitive rather than
  // returning 0 whenever either side is unknown.
  return [...ids].sort((a, b) =>
    compareTasksByCreatedDesc(taskById.get(a) ?? {}, taskById.get(b) ?? {}),
  );
}

/**
 * A band's or step's total ordering key
 * (REQ-TASKS-KANBAN-TASK-REORDERING-001.1): `position` ascending, then
 * priority rank, then `queuedAt` ascending (an absent `queuedAt` reads as
 * `createdAt`, per .36), then `createdAt` ascending, then `id` ascending.
 * `lib/kanban/wip-queue.ts`'s overflow-queue comparator delegates to this one
 * so the two cannot drift apart.
 */
export type StepOrderTask = {
  id: string;
  position?: number | null;
  priority?: TaskPriority | null;
  queuedAt?: string | null;
  createdAt?: string | null;
};

function priorityRank(priority: StepOrderTask["priority"]): number {
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

function orderTimestamp(value: string | null | undefined): number {
  if (!value) return Number.POSITIVE_INFINITY;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : Number.POSITIVE_INFINITY;
}

function effectiveQueuedAt(task: StepOrderTask): number {
  if (task.queuedAt) {
    const parsed = Date.parse(task.queuedAt);
    if (Number.isFinite(parsed)) return parsed;
  }
  return orderTimestamp(task.createdAt);
}

function compareOrderNumbers(left: number, right: number): number {
  if (left < right) return -1;
  if (left > right) return 1;
  return 0;
}

export function compareStepOrder(left: StepOrderTask, right: StepOrderTask): number {
  const position = (left.position ?? 0) - (right.position ?? 0);
  if (position !== 0) return position;

  const priority = priorityRank(left.priority) - priorityRank(right.priority);
  if (priority !== 0) return priority;

  const queuedAt = compareOrderNumbers(effectiveQueuedAt(left), effectiveQueuedAt(right));
  if (queuedAt !== 0) return queuedAt;

  const createdAt = compareOrderNumbers(
    orderTimestamp(left.createdAt),
    orderTimestamp(right.createdAt),
  );
  if (createdAt !== 0) return createdAt;

  return left.id.localeCompare(right.id);
}
