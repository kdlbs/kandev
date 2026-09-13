import { TASK_PRIORITY_TOKENS } from "@/lib/tasks/task-priority";
import type { KanbanSort } from "@/lib/kanban/kanban-sort";
import type { TaskPriority } from "@/lib/types/http";

type CreatedTask = {
  createdAt?: string | null;
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

/** Unranked (absent or out-of-vocabulary) sorts after all four tokens. */
function priorityRank(priority: TaskPriority | null | undefined): number {
  const index = priority ? (TASK_PRIORITY_TOKENS as readonly string[]).indexOf(priority) : -1;
  return index === -1 ? TASK_PRIORITY_TOKENS.length : index;
}

function compareIdsAsc(a: string, b: string): number {
  if (a === b) return 0;
  return a < b ? -1 : 1;
}

function comparePositionAsc(
  a: { position?: number | null },
  b: { position?: number | null },
): number {
  return (a.position ?? 0) - (b.position ?? 0);
}

type PriorityRankedTask = { id: string; priority?: TaskPriority | null };

/**
 * `priority_desc` order for the legacy kanban column surface. Tasks that carry
 * reorder metadata use the current native step order for ties; small callers
 * that only provide `createdAt` retain the original created-desc tie-break.
 */
export function compareTasksByPriorityThenCreatedDesc(
  a: PriorityRankedTask & CreatedTask,
  b: PriorityRankedTask & CreatedTask,
): number {
  const rankDiff = priorityRank(a.priority) - priorityRank(b.priority);
  if (rankDiff !== 0) return rankDiff;

  if ("position" in a || "position" in b || "queuedAt" in a || "queuedAt" in b) {
    return compareNativeStepOrderWithoutPriority(a as StepOrderTask, b as StepOrderTask);
  }

  const createdDiff = compareTasksByCreatedDesc(a, b);
  if (createdDiff !== 0) return createdDiff;
  return compareIdsAsc(a.id, b.id);
}

/**
 * Selects the explicit priority/created comparator used by the priority
 * display setting. Responsive surfaces use the native reorder comparator for
 * `created_desc` and this comparator for `priority_desc`.
 */
export function pickKanbanColumnComparator(
  sortToken: KanbanSort,
): (a: PriorityRankedTask & CreatedTask, b: PriorityRankedTask & CreatedTask) => number {
  return sortToken === "priority_desc"
    ? compareTasksByPriorityThenCreatedDesc
    : compareTasksByCreatedDesc;
}

/**
 * `priority_desc` order for the pipeline view: priority rank, then position,
 * then the remaining native step-order keys. The workflow-step index is the
 * outermost key and is applied by the caller.
 */
export function compareTasksByPriorityThenPositionAsc(
  a: PriorityRankedTask & { position?: number | null },
  b: PriorityRankedTask & { position?: number | null },
): number {
  const rankDiff = priorityRank(a.priority) - priorityRank(b.priority);
  if (rankDiff !== 0) return rankDiff;
  const positionDiff = comparePositionAsc(a, b);
  if (positionDiff !== 0) return positionDiff;
  return compareIdsAsc(a.id, b.id);
}

/**
 * A band's or step's total ordering key
 * (REQ-TASKS-KANBAN-TASK-REORDERING-001.1): `position` ascending, then
 * priority rank, then `queuedAt` ascending (an absent `queuedAt` reads as
 * `createdAt`), then `createdAt` ascending, then `id` ascending.
 */
export type StepOrderTask = {
  id: string;
  position?: number | null;
  priority?: TaskPriority | null;
  queuedAt?: string | null;
  createdAt?: string | null;
};

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

function compareNativeStepOrderWithoutPriority(left: StepOrderTask, right: StepOrderTask): number {
  const position = comparePositionAsc(left, right);
  if (position !== 0) return position;

  const queuedAt = compareOrderNumbers(effectiveQueuedAt(left), effectiveQueuedAt(right));
  if (queuedAt !== 0) return queuedAt;

  const createdAt = compareOrderNumbers(
    orderTimestamp(left.createdAt),
    orderTimestamp(right.createdAt),
  );
  if (createdAt !== 0) return createdAt;

  return left.id.localeCompare(right.id);
}

function comparePipelineCreatedOrder(
  left: { id: string; position?: number | null; createdAt?: string | null },
  right: { id: string; position?: number | null; createdAt?: string | null },
): number {
  if ("position" in left || "position" in right) {
    return comparePositionAsc(left, right);
  }
  return compareTasksByCreatedDesc(left, right);
}

export function compareStepOrder(left: StepOrderTask, right: StepOrderTask): number {
  const position = comparePositionAsc(left, right);
  if (position !== 0) return position;

  const priority = priorityRank(left.priority) - priorityRank(right.priority);
  if (priority !== 0) return priority;

  return compareNativeStepOrderWithoutPriority(left, right);
}

function comparePriorityThenNativeStepOrder(left: StepOrderTask, right: StepOrderTask): number {
  const priority = priorityRank(left.priority) - priorityRank(right.priority);
  if (priority !== 0) return priority;
  return compareNativeStepOrderWithoutPriority(left, right);
}

/**
 * Orders tasks for the pipeline view by workflow-step index, then by the
 * selected view order. Unknown steps use a finite equal sentinel, so the
 * within-step comparator remains reachable instead of returning NaN from
 * `Infinity - Infinity`.
 */
export function sortTasksForPipelineView<
  T extends PriorityRankedTask & { workflowStepId: string; position?: number | null },
>(tasks: T[], displaySteps: { id: string }[], sortToken: KanbanSort): T[] {
  const stepIndex = new Map(displaySteps.map((step, index) => [step.id, index]));
  const indexOf = (task: T) => stepIndex.get(task.workflowStepId) ?? displaySteps.length;
  return [...tasks].sort((a, b) => {
    const stepDiff = indexOf(a) - indexOf(b);
    if (stepDiff !== 0) return stepDiff;
    return sortToken === "priority_desc"
      ? comparePriorityThenNativeStepOrder(a, b)
      : comparePipelineCreatedOrder(a, b);
  });
}

/** Sort ids into the board's visible created-desc order using task lookups. */
export function sortIdsByCreatedDesc(ids: string[], taskById: Map<string, CreatedTask>): string[] {
  return [...ids].sort((a, b) =>
    compareTasksByCreatedDesc(taskById.get(a) ?? {}, taskById.get(b) ?? {}),
  );
}

export type DisplayOrderTask = PriorityRankedTask &
  CreatedTask & { position?: number | null; workflowStepId?: string };

/**
 * Sort selected ids into the board's current display order. The pipeline
 * step index is supplied by the caller because it must use the effective
 * workflow selection and hidden-step projection.
 */
export function sortIdsByDisplayOrder(
  ids: string[],
  taskById: Map<string, DisplayOrderTask>,
  options: {
    sortToken: KanbanSort;
    isPipelineView: boolean;
    stepIndexOf?: (stepId: string | undefined) => number;
  },
): string[] {
  const { sortToken, isPipelineView, stepIndexOf } = options;
  if (!isPipelineView) {
    if (sortToken !== "priority_desc") return sortIdsByCreatedDesc(ids, taskById);
    return [...ids].sort((a, b) =>
      compareTasksByPriorityThenCreatedDesc(
        taskById.get(a) ?? { id: a },
        taskById.get(b) ?? { id: b },
      ),
    );
  }

  const indexOf = (stepId: string | undefined) => stepIndexOf?.(stepId) ?? 0;
  return [...ids].sort((a, b) => {
    const taskA = taskById.get(a) ?? { id: a };
    const taskB = taskById.get(b) ?? { id: b };
    const stepA = indexOf(taskA.workflowStepId);
    const stepB = indexOf(taskB.workflowStepId);
    if (stepA !== stepB) return stepA - stepB;
    return sortToken === "priority_desc"
      ? comparePriorityThenNativeStepOrder(taskA, taskB)
      : comparePipelineCreatedOrder(taskA, taskB);
  });
}
