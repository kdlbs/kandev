import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type {
  OrchestrateIssue,
  OrchestrateIssueStatus,
  IssueSortField,
  IssueSortDir,
  IssueFilterState,
} from "@/lib/state/slices/orchestrate/types";

const FALLBACK_STATUS_ORDER: Record<OrchestrateIssueStatus, number> = {
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

function matchesFilters(issue: OrchestrateIssue, filters: IssueFilterState): boolean {
  if (filters.search && !issue.title.toLowerCase().includes(filters.search.toLowerCase())) {
    return false;
  }
  if (filters.statuses.length > 0 && !filters.statuses.includes(issue.status)) return false;
  if (filters.priorities.length > 0 && !filters.priorities.includes(issue.priority)) return false;
  if (
    filters.assigneeIds.length > 0 &&
    (!issue.assigneeAgentInstanceId || !filters.assigneeIds.includes(issue.assigneeAgentInstanceId))
  ) {
    return false;
  }
  if (
    filters.projectIds.length > 0 &&
    (!issue.projectId || !filters.projectIds.includes(issue.projectId))
  ) {
    return false;
  }
  return true;
}

type SortContext = {
  field: IssueSortField;
  dir: IssueSortDir;
  statusOrder: Record<string, number>;
  priorityOrder: Record<string, number>;
};

function compareIssues(a: OrchestrateIssue, b: OrchestrateIssue, ctx: SortContext): number {
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

export type FlatIssueNode = {
  issue: OrchestrateIssue;
  level: number;
  hasChildren: boolean;
};

export type UseIssuesTreeOptions = {
  issues: OrchestrateIssue[];
  filters: IssueFilterState;
  sortField: IssueSortField;
  sortDir: IssueSortDir;
  nestingEnabled: boolean;
  expandedIds: Set<string>;
};

export function useIssuesTree(opts: UseIssuesTreeOptions): FlatIssueNode[] {
  const { issues, filters, sortField, sortDir, nestingEnabled, expandedIds } = opts;
  const meta = useAppStore((s) => s.orchestrate.meta);

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
    const filtered = issues.filter((i) => matchesFilters(i, filters));
    const sortCtx: SortContext = {
      field: sortField,
      dir: sortDir,
      statusOrder: STATUS_ORDER,
      priorityOrder: PRIORITY_ORDER,
    };
    const sorted = [...filtered].sort((a, b) => compareIssues(a, b, sortCtx));

    if (!nestingEnabled) {
      return sorted.map((issue) => ({ issue, level: 0, hasChildren: false }));
    }

    const childrenMap = new Map<string | undefined, OrchestrateIssue[]>();
    for (const issue of sorted) {
      const key = issue.parentId ?? "__root__";
      const list = childrenMap.get(key);
      if (list) {
        list.push(issue);
      } else {
        childrenMap.set(key, [issue]);
      }
    }

    const result: FlatIssueNode[] = [];
    function walk(parentId: string | undefined, level: number) {
      const key = parentId ?? "__root__";
      const children = childrenMap.get(key) ?? [];
      for (const issue of children) {
        const kids = childrenMap.get(issue.id) ?? [];
        const hasChildren = kids.length > 0;
        result.push({ issue, level, hasChildren });
        if (hasChildren && expandedIds.has(issue.id)) {
          walk(issue.id, level + 1);
        }
      }
    }
    walk(undefined, 0);

    // Also include orphans (issues whose parent is not in filtered set)
    const renderedIds = new Set(result.map((n) => n.issue.id));
    for (const issue of sorted) {
      if (!renderedIds.has(issue.id)) {
        result.push({ issue, level: 0, hasChildren: false });
      }
    }

    return result;
  }, [
    issues,
    filters,
    sortField,
    sortDir,
    nestingEnabled,
    expandedIds,
    STATUS_ORDER,
    PRIORITY_ORDER,
  ]);
}
