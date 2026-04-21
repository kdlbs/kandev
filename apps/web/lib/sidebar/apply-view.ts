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

const SORT_COMPARATORS: Record<SortKey, (a: TaskSwitcherItem, b: TaskSwitcherItem) => number> = {
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

export function applySort(tasks: TaskSwitcherItem[], spec: SortSpec): TaskSwitcherItem[] {
  const cmp = SORT_COMPARATORS[spec.key];
  const sign = spec.direction === "desc" ? -1 : 1;
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

export function applyView(tasks: TaskSwitcherItem[], view: SidebarView): GroupedSidebarList {
  const filtered = applyFilters(tasks, view.filters);
  const sorted = applySort(filtered, view.sort);
  return applyGroup(sorted, view.group);
}

export function opIsNegative(op: FilterOp): boolean {
  return op === "is_not" || op === "not_in" || op === "not_matches";
}
