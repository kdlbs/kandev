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
