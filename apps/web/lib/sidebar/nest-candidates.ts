/**
 * A task shape that can be nested. Only the fields needed to reason about
 * parent/child relationships are required.
 */
export type NestCandidate = {
  id: string;
  title: string;
  parentTaskId?: string | null;
  isFromOffice?: boolean;
};

/**
 * computeNestCandidates returns the tasks that `taskId` may be nested under,
 * enforcing the one-level Kanban limit while allowing arbitrary-depth Office
 * hierarchies.
 *
 * Every hierarchy excludes the task, its current parent, and descendants.
 * Kanban additionally requires a childless subject and root candidate. Office
 * uses the backend's endpoint rule: either endpoint being Office lifts those
 * two depth restrictions.
 *
 * Order is preserved from the input list.
 */
export function computeNestCandidates<T extends NestCandidate>(
  tasks: T[],
  taskId: string,
  hierarchyTasks: readonly NestCandidate[] = tasks,
): T[] {
  const tasksById = new Map(hierarchyTasks.map((candidate) => [candidate.id, candidate]));
  const task = tasksById.get(taskId) ?? tasks.find((candidate) => candidate.id === taskId);
  if (!task) return [];

  const taskHasChildren = hierarchyTasks.some((candidate) => candidate.parentTaskId === taskId);
  const currentParent = task.parentTaskId ?? undefined;

  return tasks.filter((candidate) => {
    const hierarchyCandidate = tasksById.get(candidate.id) ?? candidate;
    if (hierarchyCandidate.id === taskId || hierarchyCandidate.id === currentParent) return false;
    if (hasAncestor(hierarchyCandidate, taskId, tasksById)) return false;
    if (task.isFromOffice || hierarchyCandidate.isFromOffice) return true;
    return !taskHasChildren && !hierarchyCandidate.parentTaskId;
  });
}

function hasAncestor(
  candidate: NestCandidate,
  ancestorId: string,
  tasksById: ReadonlyMap<string, NestCandidate>,
): boolean {
  const visited = new Set<string>();
  let parentId = candidate.parentTaskId;
  while (parentId) {
    if (parentId === ancestorId) return true;
    if (visited.has(parentId)) return true;
    visited.add(parentId);
    parentId = tasksById.get(parentId)?.parentTaskId;
  }
  return false;
}
