/**
 * A task shape that can be nested. Only the fields needed to reason about
 * parent/child relationships are required.
 */
export type NestCandidate = {
  id: string;
  title: string;
  parentTaskId?: string | null;
};

/**
 * computeNestCandidates returns the tasks that `taskId` may be nested under,
 * enforcing the one-level kanban subtask limit.
 *
 * When the task itself already has children, nesting it under any parent would
 * push those children to depth 2, so no candidate is offered. Otherwise a
 * candidate is any task that is NOT:
 *  - the task itself,
 *  - the task's current parent (selecting it would be a no-op), or
 *  - already a subtask — nesting under a subtask would create a grandchild
 *    (this also structurally excludes the task's own descendants, so no cycle
 *    can be introduced).
 *
 * Order is preserved from the input list.
 */
export function computeNestCandidates<T extends NestCandidate>(tasks: T[], taskId: string): T[] {
  const task = tasks.find((t) => t.id === taskId);

  // A task with children cannot be nested without exceeding the one-level limit.
  if (tasks.some((t) => t.parentTaskId === taskId)) return [];

  const currentParent = task?.parentTaskId ?? undefined;
  return tasks.filter((t) => t.id !== taskId && t.id !== currentParent && !t.parentTaskId);
}
