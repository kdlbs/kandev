import { classifyTask, type TaskBucket } from "@/components/task/task-classify";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { getExecutorLabel } from "@/lib/executor-icons";
import type {
  FilterClause,
  FilterDimension,
  FilterOp,
  FilterValue,
  GroupKey,
  SidebarView,
  SortKey,
  SortSpec,
} from "@/lib/state/slices/ui/sidebar-view-types";

export type SidebarGroup = {
  key: string;
  label: string;
  tasks: TaskSwitcherItem[];
};

export type GroupedSidebarList = {
  groups: SidebarGroup[];
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
};

export type SidebarTaskPrefs = {
  pinnedTaskIds: string[];
  orderedTaskIds: string[];
};

type DimensionExtractor = (task: TaskSwitcherItem) => FilterValue | undefined;

const STATE_BUCKET_ORDER: Record<TaskBucket, number> = {
  review: 0,
  in_progress: 1,
  backlog: 2,
};

function getStateBucket(task: TaskSwitcherItem): TaskBucket {
  return classifyTask(task.sessionState, task.state);
}

const dimensionExtractors: Record<FilterDimension, DimensionExtractor> = {
  archived: (t) => t.isArchived === true,
  state: (t) => getStateBucket(t),
  workflow: (t) => t.workflowId,
  workflowStep: (t) => t.workflowStepId,
  executorType: (t) => t.remoteExecutorType,
  repository: (t) => (t.repositories && t.repositories.length > 1 ? "__multi__" : t.repositoryPath),
  hasDiff: (t) => {
    const ds = t.diffStats;
    return !!ds && (ds.additions > 0 || ds.deletions > 0);
  },
  hasPR: (t) => !!t.prInfo,
  isPRReview: (t) => t.isPRReview === true,
  isIssueWatch: (t) => t.isIssueWatch === true,
  titleMatch: (t) => t.title ?? "",
};

function toStringArray(v: FilterValue): string[] {
  if (Array.isArray(v)) return v.map(String);
  return [String(v)];
}

function evaluateClause(task: TaskSwitcherItem, clause: FilterClause): boolean {
  const extract = dimensionExtractors[clause.dimension];
  const actual = extract(task);

  switch (clause.op) {
    case "is":
      return String(actual) === String(clause.value);
    case "is_not":
      return String(actual) !== String(clause.value);
    case "in":
      return toStringArray(clause.value).includes(String(actual));
    case "not_in":
      return !toStringArray(clause.value).includes(String(actual));
    case "matches": {
      const hay = String(actual ?? "").toLowerCase();
      const needle = String(clause.value).toLowerCase();
      return needle === "" || hay.includes(needle);
    }
    case "not_matches": {
      const hay = String(actual ?? "").toLowerCase();
      const needle = String(clause.value).toLowerCase();
      return needle !== "" && !hay.includes(needle);
    }
    default:
      return true;
  }
}

export function applyFilters(
  tasks: TaskSwitcherItem[],
  clauses: FilterClause[],
): TaskSwitcherItem[] {
  if (clauses.length === 0) return tasks;
  return tasks.filter((task) => clauses.every((clause) => evaluateClause(task, clause)));
}

type SortComparator = (a: TaskSwitcherItem, b: TaskSwitcherItem) => number;

const SORT_COMPARATORS: Record<Exclude<SortKey, "custom">, SortComparator> = {
  state: (a, b) => {
    const bucket = STATE_BUCKET_ORDER[getStateBucket(a)] - STATE_BUCKET_ORDER[getStateBucket(b)];
    if (bucket !== 0) return bucket;
    // Tiebreak: newest createdAt first (preserves historical sidebar ordering)
    return (b.createdAt ?? "").localeCompare(a.createdAt ?? "");
  },
  updatedAt: (a, b) => (a.updatedAt ?? "").localeCompare(b.updatedAt ?? ""),
  createdAt: (a, b) => (a.createdAt ?? "").localeCompare(b.createdAt ?? ""),
  title: (a, b) => (a.title ?? "").localeCompare(b.title ?? ""),
};

function customComparator(orderedTaskIds: string[]): SortComparator {
  const order = new Map<string, number>();
  for (let i = 0; i < orderedTaskIds.length; i++) order.set(orderedTaskIds[i], i);
  return (a, b) => {
    const ai = order.get(a.id);
    const bi = order.get(b.id);
    if (ai !== undefined && bi !== undefined) return ai - bi;
    if (ai !== undefined) return -1;
    if (bi !== undefined) return 1;
    // Fallback for tasks the user hasn't placed yet: newest createdAt first.
    return (b.createdAt ?? "").localeCompare(a.createdAt ?? "");
  };
}

export function applySort(
  tasks: TaskSwitcherItem[],
  spec: SortSpec,
  orderedTaskIds: string[] = [],
): TaskSwitcherItem[] {
  const cmp: SortComparator =
    spec.key === "custom" ? customComparator(orderedTaskIds) : SORT_COMPARATORS[spec.key];
  // "Custom" is a manual order — reversing it on direction=desc would flip
  // the user's drag, which has no intuitive meaning. The picker hides the
  // direction toggle for custom; this guard keeps the data layer consistent
  // even if a stored view ends up with desc direction.
  const sign = spec.key !== "custom" && spec.direction === "desc" ? -1 : 1;
  const withIndex = tasks.map((t, i) => ({ t, i }));
  withIndex.sort((a, b) => {
    const primary = cmp(a.t, b.t) * sign;
    if (primary !== 0) return primary;
    return a.i - b.i;
  });
  return withIndex.map((x) => x.t);
}

const UNASSIGNED_LABEL = "Unassigned";
const MULTI_REPO_LABEL = "Multi-repo";

type GroupExtractor = (task: TaskSwitcherItem) => { key: string; label: string };

const groupExtractors: Record<Exclude<GroupKey, "none">, GroupExtractor> = {
  repository: (t) => {
    if (t.repositories && t.repositories.length > 1) {
      return { key: "__multi__", label: MULTI_REPO_LABEL };
    }
    if (t.repositoryPath) return { key: t.repositoryPath, label: t.repositoryPath };
    return { key: "__unassigned__", label: UNASSIGNED_LABEL };
  },
  workflow: (t) => {
    if (t.workflowId) return { key: t.workflowId, label: t.workflowName ?? t.workflowId };
    return { key: "__unassigned__", label: UNASSIGNED_LABEL };
  },
  workflowStep: (t) => {
    if (t.workflowStepId) {
      return { key: t.workflowStepId, label: t.workflowStepTitle ?? t.workflowStepId };
    }
    return { key: "__unassigned__", label: UNASSIGNED_LABEL };
  },
  executorType: (t) => {
    if (t.remoteExecutorType) {
      return { key: t.remoteExecutorType, label: getExecutorLabel(t.remoteExecutorType) };
    }
    return { key: "__unassigned__", label: UNASSIGNED_LABEL };
  },
  state: (t) => {
    const bucket = getStateBucket(t);
    return { key: bucket, label: bucket };
  },
};

function separateSubtasks(tasks: TaskSwitcherItem[]): {
  rootTasks: TaskSwitcherItem[];
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
} {
  const allIds = new Set(tasks.map((t) => t.id));
  const subMap = new Map<string, TaskSwitcherItem[]>();
  const rootTasks: TaskSwitcherItem[] = [];
  for (const t of tasks) {
    if (t.parentTaskId && allIds.has(t.parentTaskId)) {
      const arr = subMap.get(t.parentTaskId) ?? [];
      arr.push(t);
      subMap.set(t.parentTaskId, arr);
    } else {
      rootTasks.push(t);
    }
  }
  return { rootTasks, subTasksByParentId: subMap };
}

export function applyGroup(tasks: TaskSwitcherItem[], groupKey: GroupKey): GroupedSidebarList {
  const { rootTasks, subTasksByParentId } = separateSubtasks(tasks);

  if (groupKey === "none") {
    return {
      groups: [{ key: "__all__", label: "All", tasks: rootTasks }],
      subTasksByParentId,
    };
  }

  const extract = groupExtractors[groupKey];
  const buckets = new Map<string, SidebarGroup>();
  for (const task of rootTasks) {
    const { key, label } = extract(task);
    let group = buckets.get(key);
    if (!group) {
      group = { key, label, tasks: [] };
      buckets.set(key, group);
    }
    group.tasks.push(task);
  }

  const groups = [...buckets.values()];
  if (groupKey === "repository") {
    mergeSingleRepoUnassigned(groups);
    sortRepoGroups(groups);
  }
  return { groups, subTasksByParentId };
}

function mergeSingleRepoUnassigned(groups: SidebarGroup[]): void {
  const repoGroups = groups.filter((g) => g.key !== "__multi__" && g.key !== "__unassigned__");
  if (repoGroups.length !== 1) return;
  const unassignedIdx = groups.findIndex((g) => g.key === "__unassigned__");
  if (unassignedIdx === -1) return;
  const unassigned = groups[unassignedIdx];
  repoGroups[0].tasks.push(...unassigned.tasks);
  groups.splice(unassignedIdx, 1);
}

function sortRepoGroups(groups: SidebarGroup[]): void {
  groups.sort((a, b) => {
    if (a.key === "__multi__") return -1;
    if (b.key === "__multi__") return 1;
    if (a.key === "__unassigned__") return 1;
    if (b.key === "__unassigned__") return -1;
    return a.label.localeCompare(b.label);
  });
}

/**
 * Float pinned tasks to the top within a group, preserving the order the user
 * pinned them in. The active sort still determines the order of unpinned
 * tasks (and acts as a tiebreaker among pinned tasks not in pinIndex).
 */
function floatPinnedToTop(
  tasks: TaskSwitcherItem[],
  pinnedSet: Set<string>,
  pinIndex: Map<string, number>,
): TaskSwitcherItem[] {
  if (pinnedSet.size === 0) return tasks;
  const withSortIndex = tasks.map((t, i) => ({ t, sortIdx: i }));
  withSortIndex.sort((a, b) => {
    const ap = pinnedSet.has(a.t.id);
    const bp = pinnedSet.has(b.t.id);
    if (ap !== bp) return ap ? -1 : 1;
    if (ap && bp) {
      const ai = pinIndex.get(a.t.id) ?? 0;
      const bi = pinIndex.get(b.t.id) ?? 0;
      if (ai !== bi) return ai - bi;
    }
    return a.sortIdx - b.sortIdx;
  });
  return withSortIndex.map((x) => x.t);
}

/**
 * Splice a group's reordered task IDs back into the global manual-order list,
 * preserving the position of *other* groups. Without an anchor, repeatedly
 * appending the dragged group to the end shuffles section headers (the next
 * `applyGroup` pass keys section order off first encounter, so the dragged
 * group always ends up last).
 *
 * Anchor: index of the first existing group task in `current`. If the group
 * is being placed for the first time (`firstIdx === -1`), append at the end —
 * subsequent drags will pin its position via the anchor.
 */
export function mergeGroupOrder(current: string[], groupTaskIds: string[]): string[] {
  const groupSet = new Set(groupTaskIds);
  let firstIdx = -1;
  for (let i = 0; i < current.length; i++) {
    if (groupSet.has(current[i])) {
      firstIdx = i;
      break;
    }
  }
  const remaining = current.filter((id) => !groupSet.has(id));
  if (firstIdx === -1) return [...remaining, ...groupTaskIds];
  // Items before `firstIdx` in `current` are all non-group (firstIdx is the
  // first group occurrence), so translation to `remaining` is identity.
  return [...remaining.slice(0, firstIdx), ...groupTaskIds, ...remaining.slice(firstIdx)];
}

function buildIndex(ids: string[]): Map<string, number> {
  const m = new Map<string, number>();
  for (let i = 0; i < ids.length; i++) m.set(ids[i], i);
  return m;
}

export function applyView(
  tasks: TaskSwitcherItem[],
  view: SidebarView,
  prefs?: SidebarTaskPrefs,
): GroupedSidebarList {
  const filtered = applyFilters(tasks, view.filters);
  const sorted = applySort(filtered, view.sort, prefs?.orderedTaskIds);
  const grouped = applyGroup(sorted, view.group);
  if (!prefs || prefs.pinnedTaskIds.length === 0) return grouped;
  const pinnedSet = new Set(prefs.pinnedTaskIds);
  const pinIndex = buildIndex(prefs.pinnedTaskIds);
  for (const group of grouped.groups) {
    group.tasks = floatPinnedToTop(group.tasks, pinnedSet, pinIndex);
  }
  return grouped;
}

export function opIsNegative(op: FilterOp): boolean {
  return op === "is_not" || op === "not_in" || op === "not_matches";
}
