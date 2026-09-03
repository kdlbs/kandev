import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { TaskMR } from "@/lib/types/gitlab";
import type { TaskPR } from "@/lib/types/github";
import { taskPRInfoFromSummary } from "@/components/task/task-pr-info";

type Task = KanbanState["tasks"][number];

/** GitHub PR + GitLab MR numbers linked to a task, de-duplicated and ascending. */
export function changeRequestNumbers(
  task: Task,
  mrsForTask: TaskMR[],
  prsForTask: TaskPR[] = [],
): number[] {
  const numbers = new Set<number>();
  const prNumber = taskPRInfoFromSummary(task.statusSummary)?.number;
  if (prNumber) numbers.add(prNumber);
  for (const pr of prsForTask) numbers.add(pr.pr_number);
  for (const mr of mrsForTask) numbers.add(mr.mr_iid);
  return [...numbers].sort((a, b) => a - b);
}

/** cmdk search key: title, id, and both '#N' and bare 'N' for each change-request number. */
export function dependencyOptionValue(task: Task, numbers: number[]): string {
  const parts = [task.title, task.id, ...numbers.map((n) => `#${n} ${n}`)];
  return parts.join(" ");
}

function timestampMs(task: Task): number | undefined {
  const updated = task.updatedAt ? Date.parse(task.updatedAt) : NaN;
  if (Number.isFinite(updated)) return updated;
  return createdAtMs(task);
}

function createdAtMs(task: Task): number | undefined {
  const created = task.createdAt ? Date.parse(task.createdAt) : NaN;
  return Number.isFinite(created) ? created : undefined;
}

/** Most-recently-updated first, using createdAt and then title as tie-breakers. */
export function compareDependencyCandidates(a: Task, b: Task): number {
  const aTime = timestampMs(a);
  const bTime = timestampMs(b);
  if (aTime !== undefined && bTime !== undefined && aTime !== bTime) return bTime - aTime;
  if (aTime !== undefined && bTime === undefined) return -1;
  if (aTime === undefined && bTime !== undefined) return 1;
  const aCreated = createdAtMs(a);
  const bCreated = createdAtMs(b);
  if (aCreated !== undefined && bCreated !== undefined && aCreated !== bCreated) {
    return bCreated - aCreated;
  }
  return a.title.localeCompare(b.title);
}
