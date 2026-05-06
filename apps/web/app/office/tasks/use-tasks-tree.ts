import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type {
  OfficeTask,
  OfficeTaskStatus,
  TaskSortField,
  TaskSortDir,
  TaskFilterState,
} from "@/lib/state/slices/office/types";

const FALLBACK_STATUS_ORDER: Record<OfficeTaskStatus, number> = {
  backlog: 0,
  todo: 1,
  in_progress: 2,
  in_review: 3,
  blocked: 4,
  done: 5,
  cancelled: 6,
};

const FALLBACK_PRIORITY_ORDER: Record<string, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
  none: 4,
};

function matchesFilters(task: OfficeTask, filters: TaskFilterState): boolean {
  if (filters.search && !task.title.toLowerCase().includes(filters.search.toLowerCase())) {
    return false;
  }
  if (filters.statuses.length > 0 && !filters.statuses.includes(task.status)) return false;
  if (filters.priorities.length > 0 && !filters.priorities.includes(task.priority)) return false;
  if (
    filters.assigneeIds.length > 0 &&
    (!task.assigneeAgentProfileId || !filters.assigneeIds.includes(task.assigneeAgentProfileId))
  ) {
    return false;
  }
  if (
    filters.projectIds.length > 0 &&
    (!task.projectId || !filters.projectIds.includes(task.projectId))
  ) {
    return false;
  }
  return true;
}

type SortContext = {
  field: TaskSortField;
  dir: TaskSortDir;
  statusOrder: Record<string, number>;
  priorityOrder: Record<string, number>;
};

function compareIssues(a: OfficeTask, b: OfficeTask, ctx: SortContext): number {
  let cmp = 0;
  switch (ctx.field) {
    case "status":
      cmp = (ctx.statusOrder[a.status] ?? 99) - (ctx.statusOrder[b.status] ?? 99);
      break;
    case "priority":
      cmp = (ctx.priorityOrder[a.priority] ?? 4) - (ctx.priorityOrder[b.priority] ?? 4);
      break;
    case "title":
      cmp = a.title.localeCompare(b.title);
      break;
    case "created":
      cmp = a.createdAt.localeCompare(b.createdAt);
      break;
    case "updated":
    default:
      cmp = a.updatedAt.localeCompare(b.updatedAt);
      break;
  }
  return ctx.dir === "asc" ? cmp : -cmp;
}

export type FlatTaskNode = {
  task: OfficeTask;
  level: number;
  hasChildren: boolean;
};

export type UseIssuesTreeOptions = {
  tasks: OfficeTask[];
  filters: TaskFilterState;
  sortField: TaskSortField;
  sortDir: TaskSortDir;
  nestingEnabled: boolean;
  expandedIds: Set<string>;
};

export function useIssuesTree(opts: UseIssuesTreeOptions): FlatTaskNode[] {
  const { tasks, filters, sortField, sortDir, nestingEnabled, expandedIds } = opts;
  const meta = useAppStore((s) => s.office.meta);

  const STATUS_ORDER = useMemo(() => {
    if (!meta) return FALLBACK_STATUS_ORDER;
    const map: Record<string, number> = {};
    for (const s of meta.statuses) map[s.id] = s.order;
    return map;
  }, [meta]);

  const PRIORITY_ORDER = useMemo(() => {
    if (!meta) return FALLBACK_PRIORITY_ORDER;
    const map: Record<string, number> = {};
    for (const p of meta.priorities) map[p.id] = p.order;
    return map;
  }, [meta]);

  return useMemo(() => {
    const filtered = tasks.filter((i) => matchesFilters(i, filters));
    const sortCtx: SortContext = {
      field: sortField,
      dir: sortDir,
      statusOrder: STATUS_ORDER,
      priorityOrder: PRIORITY_ORDER,
    };
    const sorted = [...filtered].sort((a, b) => compareIssues(a, b, sortCtx));

    if (!nestingEnabled) {
      return sorted.map((task) => ({ task, level: 0, hasChildren: false }));
    }

    const childrenMap = new Map<string | undefined, OfficeTask[]>();
    for (const task of sorted) {
      const key = task.parentId ?? "__root__";
      const list = childrenMap.get(key);
      if (list) {
        list.push(task);
      } else {
        childrenMap.set(key, [task]);
      }
    }

    const result: FlatTaskNode[] = [];
    function walk(parentId: string | undefined, level: number) {
      const key = parentId ?? "__root__";
      const children = childrenMap.get(key) ?? [];
      for (const task of children) {
        const kids = childrenMap.get(task.id) ?? [];
        const hasChildren = kids.length > 0;
        result.push({ task, level, hasChildren });
        if (hasChildren && expandedIds.has(task.id)) {
          walk(task.id, level + 1);
        }
      }
    }
    walk(undefined, 0);

    // Also include orphans (tasks whose parent is not in filtered set)
    const renderedIds = new Set(result.map((n) => n.task.id));
    for (const task of sorted) {
      if (!renderedIds.has(task.id)) {
        result.push({ task, level: 0, hasChildren: false });
      }
    }

    return result;
  }, [
    tasks,
    filters,
    sortField,
    sortDir,
    nestingEnabled,
    expandedIds,
    STATUS_ORDER,
    PRIORITY_ORDER,
  ]);
}
