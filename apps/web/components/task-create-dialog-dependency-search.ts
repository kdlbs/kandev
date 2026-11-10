import type { KanbanState } from "@/lib/state/slices/kanban/types";
export { changeRequestNumbers } from "@/lib/kanban/task-search-index";

type Task = KanbanState["tasks"][number];

/** cmdk search key: title, id, and both '#N' and bare 'N' for each change-request number. */
export function dependencyOptionValue(task: Pick<Task, "id" | "title">, numbers: number[]): string {
  const parts = [task.title, task.id, ...numbers.map((n) => `#${n} ${n}`)];
  return parts.join(" ");
}

function timestampMs(task: Pick<Task, "updatedAt" | "createdAt">): number | undefined {
  const updated = task.updatedAt ? Date.parse(task.updatedAt) : NaN;
  if (Number.isFinite(updated)) return updated;
  return createdAtMs(task);
}

function createdAtMs(task: Pick<Task, "createdAt">): number | undefined {
  const created = task.createdAt ? Date.parse(task.createdAt) : NaN;
  return Number.isFinite(created) ? created : undefined;
}

/** Most-recently-updated first, using createdAt and then title as tie-breakers. */
export function compareDependencyCandidates(
  a: Pick<Task, "title" | "updatedAt" | "createdAt">,
  b: Pick<Task, "title" | "updatedAt" | "createdAt">,
): number {
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

type RankedCandidate = Pick<Task, "id" | "title" | "updatedAt" | "createdAt"> &
  Partial<Pick<Task, "workflowStepId">>;

export type CreateDialogCandidateRanking = {
  contextStepId?: string;
  recentTaskIds?: ReadonlySet<string>;
  searchText?: string;
  titleMatchFirst?: boolean;
};

function compareCandidateContext(
  a: RankedCandidate,
  b: RankedCandidate,
  ranking: CreateDialogCandidateRanking,
): number {
  const aInContext = Boolean(ranking.contextStepId && a.workflowStepId === ranking.contextStepId);
  const bInContext = Boolean(ranking.contextStepId && b.workflowStepId === ranking.contextStepId);
  if (aInContext !== bInContext) return aInContext ? -1 : 1;
  const aRecent = ranking.recentTaskIds?.has(a.id) ?? false;
  const bRecent = ranking.recentTaskIds?.has(b.id) ?? false;
  if (aRecent === bRecent) return 0;
  return aRecent ? -1 : 1;
}

function compareCandidateTitleMatch(
  a: RankedCandidate,
  b: RankedCandidate,
  ranking: CreateDialogCandidateRanking,
): number {
  const search = ranking.searchText?.trim().toLocaleLowerCase();
  if (!ranking.titleMatchFirst || !search) return 0;
  const aMatch = a.title.toLocaleLowerCase().includes(search);
  const bMatch = b.title.toLocaleLowerCase().includes(search);
  if (aMatch === bMatch) return 0;
  return aMatch ? -1 : 1;
}

/** Adds create-dialog context signals before the existing dependency ordering. */
export function rankCreateDialogCandidates(
  a: RankedCandidate,
  b: RankedCandidate,
  ranking: CreateDialogCandidateRanking = {},
): number {
  return (
    compareCandidateContext(a, b, ranking) ||
    compareCandidateTitleMatch(a, b, ranking) ||
    compareDependencyCandidates(a, b)
  );
}
