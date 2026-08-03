import { isAdmittedTask } from "@/lib/kanban/wip-limit";

export type CreatedTask = {
  id?: string;
  createdAt?: string;
  position?: number;
  queuedForStepId?: string | null;
  wipAdmitted?: boolean | null;
};

function createdAtTime(task: CreatedTask): number {
  if (!task.createdAt) return Number.NEGATIVE_INFINITY;
  const time = Date.parse(task.createdAt);
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time;
}

function positionValue(task: CreatedTask): number {
  return Number.isFinite(task.position) ? Number(task.position) : 0;
}

export function compareTasksByCreatedDesc(a: CreatedTask, b: CreatedTask): number {
  const aTime = createdAtTime(a);
  const bTime = createdAtTime(b);
  if (bTime > aTime) return 1;
  if (bTime < aTime) return -1;
  return 0;
}

/**
 * Kanban column order: position ascending, then newest-first createdAt, then id.
 * All-zero positions therefore keep today's newest-first look until a reorder densifies them.
 */
export function compareTasksByBoardOrder(a: CreatedTask, b: CreatedTask): number {
  const byPosition = positionValue(a) - positionValue(b);
  if (byPosition !== 0) return byPosition;
  const byCreated = compareTasksByCreatedDesc(a, b);
  if (byCreated !== 0) return byCreated;
  return (a.id ?? "").localeCompare(b.id ?? "");
}

/** Admitted for board WIP / sortable list — same rule as WIP counting. */
export function isAdmittedKanbanTask(task: CreatedTask): boolean {
  return isAdmittedTask(task);
}

export function isQueuedOverflowKanbanTask(task: CreatedTask): boolean {
  return !isAdmittedTask(task);
}

export function partitionKanbanColumnTasks<T extends CreatedTask>(
  tasks: T[],
): {
  admitted: T[];
  queued: T[];
} {
  const admitted: T[] = [];
  const queued: T[] = [];
  for (const task of tasks) {
    if (isAdmittedKanbanTask(task)) admitted.push(task);
    else queued.push(task);
  }
  admitted.sort(compareTasksByBoardOrder);
  queued.sort(compareTasksByBoardOrder);
  return { admitted, queued };
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

/** Sort ids into the visible Kanban board order (position, then createdAt desc). */
export function sortIdsByBoardOrder(ids: string[], taskById: Map<string, CreatedTask>): string[] {
  return [...ids].sort((a, b) =>
    compareTasksByBoardOrder(
      { id: a, ...(taskById.get(a) ?? {}) },
      { id: b, ...(taskById.get(b) ?? {}) },
    ),
  );
}
