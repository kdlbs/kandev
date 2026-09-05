import { TASK_PRIORITY_TOKENS, isTaskPriority } from "@/lib/tasks/task-priority";
import type { TaskPriority } from "@/lib/types/http";

/**
 * Resolves a value read back from storage (or the WS/HTTP wire) to a priority
 * filter selection. A non-list value, `null` included, and any member outside
 * the four priority tokens resolve to empty/dropped rather than reaching the
 * membership test — mirrors the server's read-side rules. The result is
 * deduped and returned in rank order so re-renders are stable.
 */
export function parseKanbanPriorityFilterTokens(value: unknown): TaskPriority[] {
  if (!Array.isArray(value)) return [];
  const selected = new Set<TaskPriority>();
  for (const item of value) {
    if (isTaskPriority(item)) selected.add(item);
  }
  return TASK_PRIORITY_TOKENS.filter((token) => selected.has(token));
}

/** Adds or removes `token`, returning the result in rank order. */
export function toggleKanbanPriorityFilterToken(
  selection: TaskPriority[],
  token: TaskPriority,
): TaskPriority[] {
  const next = new Set(selection);
  if (next.has(token)) next.delete(token);
  else next.add(token);
  return TASK_PRIORITY_TOKENS.filter((t) => next.has(t));
}

/**
 * The empty selection admits every task, including an unranked one. A
 * non-empty selection admits only a task whose priority is a member —
 * an unranked task holds none of the selected tokens, so it is excluded.
 */
export function taskMatchesPriorityFilter(
  priority: TaskPriority | undefined,
  selection: TaskPriority[],
): boolean {
  if (selection.length === 0) return true;
  if (!priority) return false;
  return selection.includes(priority);
}
