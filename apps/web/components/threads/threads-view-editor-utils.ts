export const MIN_THREAD_VIEW_COLUMNS = 1;
export const MAX_THREAD_VIEW_COLUMNS = 30;
export const MAX_THREAD_VIEW_TASK_IDS = 200;

/** Returns null for the unbounded value and undefined for an invalid edit. */
export function parseThreadMaxColumns(input: string, badInput = false): number | null | undefined {
  if (badInput) return undefined;
  if (input === "") return null;
  if (!/^\d+$/.test(input)) return undefined;

  const value = Number(input);
  if (
    !Number.isFinite(value) ||
    !Number.isInteger(value) ||
    value < MIN_THREAD_VIEW_COLUMNS ||
    value > MAX_THREAD_VIEW_COLUMNS
  ) {
    return undefined;
  }
  return value;
}

export function updateThreadTaskSelection(
  selectedTaskIds: string[],
  taskId: string,
  checked: boolean,
): string[] {
  if (!checked) return selectedTaskIds.filter((selectedTaskId) => selectedTaskId !== taskId);
  if (selectedTaskIds.includes(taskId) || selectedTaskIds.length >= MAX_THREAD_VIEW_TASK_IDS) {
    return selectedTaskIds;
  }
  return [...selectedTaskIds, taskId];
}

export function updateVisibleThreadTaskSelection(
  selectedTaskIds: string[],
  visibleTaskIds: string[],
  checked: boolean,
): string[] {
  if (!checked) {
    const visible = new Set(visibleTaskIds);
    return selectedTaskIds.filter((taskId) => !visible.has(taskId));
  }
  const next = [...selectedTaskIds];
  for (const taskId of visibleTaskIds) {
    if (next.includes(taskId)) continue;
    if (next.length >= MAX_THREAD_VIEW_TASK_IDS) break;
    next.push(taskId);
  }
  return next;
}
