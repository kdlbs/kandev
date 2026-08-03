export function normalizeWipLimit(limit: number | null | undefined): number {
  if (!Number.isFinite(limit)) return 0;
  return Math.max(0, Math.trunc(limit ?? 0));
}

export function formatWipCount(taskCount: number, limit: number | null | undefined): string {
  const normalizedLimit = normalizeWipLimit(limit);
  return normalizedLimit > 0 ? `${taskCount}/${normalizedLimit}` : String(taskCount);
}

export function isOverWipLimit(taskCount: number, limit: number | null | undefined): boolean {
  const normalizedLimit = normalizeWipLimit(limit);
  return normalizedLimit > 0 && taskCount > normalizedLimit;
}

export type WipAdmissionTask = {
  queuedForStepId?: string | null;
  wipAdmitted?: boolean | null;
};

/** Admitted for WIP counts / sortable board lists: flagged admitted, or not queued. */
export function isAdmittedTask(task: WipAdmissionTask): boolean {
  return task.wipAdmitted === true || !task.queuedForStepId;
}

export function countAdmittedTasks<T extends WipAdmissionTask>(tasks: T[]): number {
  return tasks.filter(isAdmittedTask).length;
}
