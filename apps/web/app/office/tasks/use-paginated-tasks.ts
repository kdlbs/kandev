import { useCallback, useEffect, useMemo, useRef } from "react";
import { useInfiniteQuery, type InfiniteData } from "@tanstack/react-query";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { officeTasksInfiniteQueryOptions } from "@/lib/query/query-options/office";
import type { OfficeTaskFilters } from "@/lib/query/keys";
import type {
  OfficeTask,
  TaskFilterState,
  TaskSortDir,
  TaskSortField,
} from "@/lib/state/slices/office/types";
import { canonicalStatusesToBackend } from "./normalize-status";

const DEFAULT_PAGE_LIMIT = 200;

// Server-side sort allow-list (mirror of taskListSortColumns on the
// backend). Frontend "title" / "status" sorts have no SQL equivalent and
// are handled by the local re-sort in useIssuesTree.
function mapSortField(field: TaskSortField): OfficeTaskFilters["sort"] {
  switch (field) {
    case "updated":
      return "updated_at";
    case "created":
      return "created_at";
    case "priority":
      return "priority";
    default:
      return undefined;
  }
}

function buildQueryFilters(
  filters: TaskFilterState,
  sortField: TaskSortField,
  sortDir: TaskSortDir,
  limit: number,
  includeSystem: boolean,
): OfficeTaskFilters {
  const params: OfficeTaskFilters = { limit };
  const status = canonicalStatusesToBackend(filters.statuses);
  if (status.length > 0) params.status = status;
  if (filters.priorities.length > 0) params.priority = filters.priorities;
  // Keep the full UI selection in the query key. The query option factory
  // only sends single assignee/project values to the backend because the
  // endpoint does not yet accept repeated values.
  if (filters.assigneeIds.length > 0) params.assignee = filters.assigneeIds;
  if (filters.projectIds.length > 0) params.project = filters.projectIds;
  const sort = mapSortField(sortField);
  if (sort) {
    params.sort = sort;
    params.order = sortDir;
  } else {
    params.sort = null;
    params.order = null;
  }
  if (includeSystem) params.includeSystem = true;
  return params;
}

type OfficeTasksInfiniteData = InfiniteData<{ tasks?: OfficeTask[] }>;

function flattenPages(data: OfficeTasksInfiniteData | undefined): OfficeTask[] {
  const tasks: OfficeTask[] = [];
  const seen = new Set<string>();
  for (const page of data?.pages ?? []) {
    for (const task of page.tasks ?? []) {
      if (seen.has(task.id)) continue;
      seen.add(task.id);
      tasks.push(task);
    }
  }
  return tasks;
}

export type UsePaginatedTasksResult = {
  loadMore: () => void;
  hasMore: boolean;
  isLoadingMore: boolean;
  refetch: () => Promise<void>;
};

/**
 * Owns the lifecycle of the office tasks list: server-side filter / sort /
 * keyset pagination via the Stream-E `/workspaces/:wsId/tasks?...` endpoint.
 *
 * Resets the cursor and replaces the list whenever the workspace, filters
 * or sort change. Exposes loadMore() to fetch the next page (appending to
 * the store) and refetch() for WS-driven invalidations.
 */
export function usePaginatedTasks(
  workspaceId: string | null,
  includeSystem: boolean,
): UsePaginatedTasksResult {
  const setTasks = useAppStore((s) => s.setTasks);
  const setTasksLoading = useAppStore((s) => s.setTasksLoading);
  const filters = useAppStore((s) => s.office.tasks.filters);
  const sortField = useAppStore((s) => s.office.tasks.sortField);
  const sortDir = useAppStore((s) => s.office.tasks.sortDir);

  const queryFilters = useMemo(
    () => buildQueryFilters(filters, sortField, sortDir, DEFAULT_PAGE_LIMIT, includeSystem),
    [filters, sortField, sortDir, includeSystem],
  );

  const query = useInfiniteQuery(officeTasksInfiniteQueryOptions(workspaceId ?? "", queryFilters));

  const lastErrorRef = useRef<unknown>(null);
  useEffect(() => {
    if (!query.error || lastErrorRef.current === query.error) return;
    lastErrorRef.current = query.error;
    toast.error(query.error instanceof Error ? query.error.message : "Failed to load tasks");
  }, [query.error]);

  const queryTasks = useMemo(() => flattenPages(query.data), [query.data]);

  useEffect(() => {
    if (!query.data) return;
    setTasks(queryTasks);
  }, [query.data, queryTasks, setTasks]);

  useEffect(() => {
    setTasksLoading(Boolean(workspaceId) && query.isPending);
  }, [query.isPending, setTasksLoading, workspaceId]);

  const loadMore = useCallback(() => {
    if (!workspaceId || !query.hasNextPage || query.isFetchingNextPage) return;
    void query.fetchNextPage();
  }, [query, workspaceId]);

  const refetch = useCallback(async () => {
    if (!workspaceId) return;
    await query.refetch();
  }, [query, workspaceId]);

  return {
    loadMore,
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    refetch,
  };
}
