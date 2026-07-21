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
 * computeNestCandidates returns the tasks that `taskId` may be nested under.
 *
 * A candidate is any task that is NOT:
 *  - the task itself,
 *  - a descendant of the task (nesting under a descendant would create a
 *    cycle), or
 *  - the task's current parent (selecting it would be a no-op).
 *
 * Order is preserved from the input list.
 */
export function computeNestCandidates<T extends NestCandidate>(tasks: T[], taskId: string): T[] {
  const task = tasks.find((t) => t.id === taskId);

  // Build the excluded set: the task plus its transitive descendants.
  const excluded = new Set<string>([taskId]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const t of tasks) {
      if (t.parentTaskId && excluded.has(t.parentTaskId) && !excluded.has(t.id)) {
        excluded.add(t.id);
        grew = true;
      }
    }
  }

  const currentParent = task?.parentTaskId ?? undefined;
  return tasks.filter((t) => !excluded.has(t.id) && t.id !== currentParent);
}
