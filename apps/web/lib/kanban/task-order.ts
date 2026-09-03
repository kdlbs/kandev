import { KANBAN_PRIORITY_TOKENS } from "@/lib/kanban/task-priority";
import type { KanbanSort } from "@/lib/kanban/kanban-sort";
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

/** Unranked (absent or out-of-vocabulary) sorts after all four tokens. */
function priorityRank(priority: TaskPriority | undefined): number {
  const index = priority ? (KANBAN_PRIORITY_TOKENS as readonly string[]).indexOf(priority) : -1;
  return index === -1 ? KANBAN_PRIORITY_TOKENS.length : index;
}

function compareIdsAsc(a: string, b: string): number {
  if (a === b) return 0;
  return a < b ? -1 : 1;
}

function comparePositionAsc(a: { position?: number }, b: { position?: number }): number {
  return (a.position ?? 0) - (b.position ?? 0);
}

type PriorityRankedTask = { id: string; priority?: TaskPriority };

/**
 * `priority_desc` total order for the kanban and mobile column views:
 * priority rank, then `createdAt` descending, then task `id` ascending. The
 * `id` key is required — neither preceding key is unique — so no two cards
 * are ever left in an order this sequence does not determine.
 */
export function compareTasksByPriorityThenCreatedDesc(
  a: PriorityRankedTask & CreatedTask,
  b: PriorityRankedTask & CreatedTask,
): number {
  const rankDiff = priorityRank(a.priority) - priorityRank(b.priority);
  if (rankDiff !== 0) return rankDiff;
  const createdDiff = compareTasksByCreatedDesc(a, b);
  if (createdDiff !== 0) return createdDiff;
  return compareIdsAsc(a.id, b.id);
}

/**
 * Selects the within-step comparator for the kanban and mobile column views,
 * shared so both views apply the sort token identically.
 */
export function pickKanbanColumnComparator(
  sortToken: KanbanSort,
): (a: PriorityRankedTask & CreatedTask, b: PriorityRankedTask & CreatedTask) => number {
  return sortToken === "priority_desc"
    ? compareTasksByPriorityThenCreatedDesc
    : compareTasksByCreatedDesc;
}

/**
 * `priority_desc` total order for the pipeline view's within-step tiebreak:
 * priority rank, then `position` ascending, then task `id` ascending. The
 * workflow-step index is applied by the caller as the outermost key.
 */
export function compareTasksByPriorityThenPositionAsc(
  a: PriorityRankedTask & { position?: number },
  b: PriorityRankedTask & { position?: number },
): number {
  const rankDiff = priorityRank(a.priority) - priorityRank(b.priority);
  if (rankDiff !== 0) return rankDiff;
  const positionDiff = comparePositionAsc(a, b);
  if (positionDiff !== 0) return positionDiff;
  return compareIdsAsc(a.id, b.id);
}

/**
 * Orders tasks for the pipeline view by workflow-step index (the step's
 * index in `displaySteps`, the order the pipeline view already renders),
 * then by the sort token's within-step comparator. Step index is always the
 * outermost key, so `priority_desc` reorders cards within a step and never
 * regroups them across steps. Under `created_desc` this is byte-identical to
 * the view's pre-existing inline sort (step index, then `position`
 * ascending, no `id` tiebreak).
 */
export function sortTasksForPipelineView<
  T extends PriorityRankedTask & { workflowStepId: string; position?: number },
>(tasks: T[], displaySteps: { id: string }[], sortToken: KanbanSort): T[] {
  const stepIndex = new Map(displaySteps.map((step, index) => [step.id, index]));
  const indexOf = (task: T) => stepIndex.get(task.workflowStepId) ?? displaySteps.length;
  const withinStep =
    sortToken === "priority_desc" ? compareTasksByPriorityThenPositionAsc : comparePositionAsc;
  return [...tasks].sort((a, b) => {
    const stepDiff = indexOf(a) - indexOf(b);
    if (stepDiff !== 0) return stepDiff;
    return withinStep(a, b);
  });
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

export type DisplayOrderTask = PriorityRankedTask &
  CreatedTask & { position?: number; workflowStepId?: string };

/**
 * Sorts `ids` into the board's currently-displayed order, derived from the
 * active board sort token and the active view rather than a fixed
 * created-descending order. The pipeline view groups by
 * `stepIndexOf` (its currently displayed step order) as the outermost key;
 * the kanban/mobile views do not group by step. Under `created_desc` this
 * delegates to `sortIdsByCreatedDesc`/`comparePositionAsc` so today's order is
 * unchanged byte-for-byte.
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
  // `compareTasksByPriorityThenPositionAsc` already ends in its own id
  // tiebreak; `comparePositionAsc` intentionally has none, matching
  // `created_desc`'s "native order untouched" contract.
  const withinStep =
    sortToken === "priority_desc" ? compareTasksByPriorityThenPositionAsc : comparePositionAsc;
  return [...ids].sort((a, b) => {
    const taskA = taskById.get(a) ?? { id: a };
    const taskB = taskById.get(b) ?? { id: b };
    const stepDiff = indexOf(taskA.workflowStepId) - indexOf(taskB.workflowStepId);
    if (stepDiff !== 0) return stepDiff;
    return withinStep(taskA, taskB);
  });
}
