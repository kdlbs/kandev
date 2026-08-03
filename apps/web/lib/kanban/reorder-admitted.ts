import { arrayMove } from "@dnd-kit/sortable";
import {
  compareTasksByBoardOrder,
  isAdmittedKanbanTask,
  type CreatedTask,
} from "@/lib/kanban/task-order";

export type ReorderableTask = CreatedTask & {
  id: string;
  workflowStepId: string;
};

export type PositionPatch = {
  taskId: string;
  workflowStepId: string;
  position: number;
};

export type SameStepReorderResult<T extends ReorderableTask> = {
  /** Full task list with updated positions for admitted cards in the step. */
  tasks: T[];
  /** Persist calls needed after densify (same step, changed position only). */
  patches: PositionPatch[];
};

/**
 * Reorder admitted cards in `stepId` by moving `activeId` to the index of
 * `overId` (or to the end when `overId` is null / the column). Queued overflow
 * cards are left in place and excluded from densify.
 */
export function reorderAdmittedInStep<T extends ReorderableTask>(
  tasks: T[],
  stepId: string,
  activeId: string,
  overId: string | null,
): SameStepReorderResult<T> | null {
  const stepTasks = tasks.filter((task) => task.workflowStepId === stepId);
  const admitted = stepTasks.filter(isAdmittedKanbanTask).sort(compareTasksByBoardOrder);
  const fromIndex = admitted.findIndex((task) => task.id === activeId);
  if (fromIndex < 0) return null;

  let toIndex = admitted.length - 1;
  if (overId && overId !== stepId) {
    const overIndex = admitted.findIndex((task) => task.id === overId);
    // Dropping on a queued overflow card (or any non-admitted id) means "end of
    // the admitted list", not a no-op — queued cards render below admitted.
    toIndex = overIndex >= 0 ? overIndex : admitted.length - 1;
  }
  if (fromIndex === toIndex) return null;

  const nextAdmitted = arrayMove(admitted, fromIndex, toIndex);
  const positionById = new Map(nextAdmitted.map((task, index) => [task.id, index]));
  const patches: PositionPatch[] = [];
  for (const [taskId, position] of positionById) {
    const previous = admitted.find((task) => task.id === taskId);
    if (!previous || (previous.position ?? 0) === position) continue;
    patches.push({ taskId, workflowStepId: stepId, position });
  }
  if (patches.length === 0) return null;

  const nextTasks = tasks.map((task) => {
    const position = positionById.get(task.id);
    if (position === undefined) return task;
    return { ...task, position };
  });

  return { tasks: nextTasks, patches };
}

/** Resolve a droppable/sortable over-id to a workflow step id. */
export function resolveKanbanDropStepId(
  overId: string,
  tasks: ReadonlyArray<{ id: string; workflowStepId: string }>,
  stepIds: ReadonlySet<string>,
): string | null {
  if (stepIds.has(overId)) return overId;
  const overTask = tasks.find((task) => task.id === overId);
  return overTask?.workflowStepId ?? null;
}
