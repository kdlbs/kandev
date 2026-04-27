import { useMemo } from "react";
import type {
  OrchestrateIssue,
  OrchestrateIssueStatus,
  IssueSortField,
  IssueSortDir,
  IssueFilterState,
} from "@/lib/state/slices/orchestrate/types";

const STATUS_ORDER: Record<OrchestrateIssueStatus, number> = {
  backlog: 0,
  todo: 1,
  in_progress: 2,
  in_review: 3,
  blocked: 4,
  done: 5,
  cancelled: 6,
};

const PRIORITY_ORDER: Record<string, number> = {
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

function compareIssues(
  a: OrchestrateIssue,
  b: OrchestrateIssue,
  field: IssueSortField,
  dir: IssueSortDir,
): number {
  let cmp = 0;
  switch (field) {
    case "status":
      cmp = STATUS_ORDER[a.status] - STATUS_ORDER[b.status];
      break;
    case "priority":
      cmp = (PRIORITY_ORDER[a.priority] ?? 4) - (PRIORITY_ORDER[b.priority] ?? 4);
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
  return dir === "asc" ? cmp : -cmp;
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
  return useMemo(() => {
    const filtered = issues.filter((i) => matchesFilters(i, filters));
    const sorted = [...filtered].sort((a, b) => compareIssues(a, b, sortField, sortDir));

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
  }, [issues, filters, sortField, sortDir, nestingEnabled, expandedIds]);
}
