import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskMR } from "@/lib/types/gitlab";
import { taskPRInfoFromSummary } from "@/components/task/task-pr-info";

type Task = KanbanState["tasks"][number];

/** GitHub PR + GitLab MR numbers linked to a task, de-duplicated and ascending. */
export function changeRequestNumbers(task: Task, mrsForTask: TaskMR[]): number[] {
  const numbers = new Set<number>();
  const prNumber = taskPRInfoFromSummary(task.statusSummary)?.number;
  if (prNumber) numbers.add(prNumber);
  for (const mr of mrsForTask) numbers.add(mr.mr_iid);
  return [...numbers].sort((a, b) => a - b);
}

/** cmdk search key: title, id, and both '#N' and bare 'N' for each change-request number. */
export function dependencyOptionValue(task: Task, numbers: number[]): string {
  const parts = [task.title, task.id, ...numbers.map((n) => `#${n} ${n}`)];
  return parts.join(" ");
}

function timestampMs(task: Task): number | undefined {
  const value = task.updatedAt ?? task.createdAt;
  return value ? new Date(value).getTime() : undefined;
}

/** Most-recently-updated first, falling back to createdAt then title. */
export function compareDependencyCandidates(a: Task, b: Task): number {
  const aTime = timestampMs(a);
  const bTime = timestampMs(b);
  if (aTime !== undefined && bTime !== undefined && aTime !== bTime) return bTime - aTime;
  if (aTime !== undefined && bTime === undefined) return -1;
  if (aTime === undefined && bTime !== undefined) return 1;
  return a.title.localeCompare(b.title);
}
