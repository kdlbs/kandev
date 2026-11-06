import type { ThreadTaskScope, ThreadView } from "@/lib/state/slices/ui/thread-view-types";
import { ATTENTION_SORT } from "@/lib/threads/thread-view-query";

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

/**
 * The draft fields to write when the task-scope mode changes. Selection order
 * has nothing to rank in the all scope, so leaving it replaces the sort in the
 * same edit.
 */
export function threadTaskScopeModeUpdate(
  current: Pick<ThreadView, "taskScope" | "sort">,
  mode: ThreadTaskScope["mode"],
): Partial<Pick<ThreadView, "taskScope" | "sort">> {
  if (mode === "selected") {
    const taskIds = current.taskScope.mode === "selected" ? [...current.taskScope.taskIds] : [];
    return { taskScope: { mode: "selected", taskIds } };
  }
  const taskScope: ThreadTaskScope = { mode: "all", taskIds: [] };
  return current.sort.key === "selection" ? { taskScope, sort: ATTENTION_SORT } : { taskScope };
}
