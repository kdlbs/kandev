import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { KanbanState, KanbanMultiState } from "@/lib/state/slices/kanban/types";

export type TaskSubtask = Pick<
  KanbanState["tasks"][number],
  "id" | "title" | "state" | "position" | "createdAt"
>;

// Stable empty reference so the hook's output stays referentially equal
// across renders when a task has no subtasks — an unstable array from a
// selector causes an infinite re-render loop (see hooks/domains/gitlab/use-task-mr.ts).
const EMPTY_SUBTASKS: TaskSubtask[] = [];

function compareSubtasks(a: TaskSubtask, b: TaskSubtask): number {
  const positionDelta = (a.position ?? 0) - (b.position ?? 0);
  if (positionDelta !== 0) return positionDelta;
  return (a.createdAt ?? "").localeCompare(b.createdAt ?? "");
}

function computeSubtasks(
  tasks: KanbanState["tasks"],
  snapshots: KanbanMultiState["snapshots"],
  taskId: string,
): TaskSubtask[] {
  const seen = new Map<string, TaskSubtask>();
  for (const task of tasks) {
    if (task.parentTaskId === taskId) seen.set(task.id, task);
  }
  for (const snapshot of Object.values(snapshots)) {
    for (const task of snapshot.tasks) {
      if (task.parentTaskId === taskId && !seen.has(task.id)) seen.set(task.id, task);
    }
  }
  if (seen.size === 0) return EMPTY_SUBTASKS;
  return Array.from(seen.values()).sort(compareSubtasks);
}

/**
 * Direct children of `taskId`, sourced from the active board (state.kanban)
 * plus every loaded cross-workflow snapshot (state.kanbanMulti), so a task
 * whose subtasks live on a different workflow's board still shows them.
 * Mirrors separateSubtasks (lib/sidebar/apply-view.ts) rather than inventing
 * a second grouping rule. De-duplicated by id, ordered by position then
 * createdAt.
 *
 * `tasks` and `snapshots` are read as their own selectors (rather than
 * filtering inside one selector) so the derived array is memoized against
 * their stable store references — filtering inline would return a fresh
 * array on every store update and re-render the caller unnecessarily.
 */
export function useTaskSubtasks(taskId: string | null): TaskSubtask[] {
  const tasks = useAppStore((state) => state.kanban.tasks);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  return useMemo(
    () => (taskId ? computeSubtasks(tasks, snapshots, taskId) : EMPTY_SUBTASKS),
    [tasks, snapshots, taskId],
  );
}
