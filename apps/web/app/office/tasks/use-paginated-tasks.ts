import { useCallback, useMemo } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import type { ListTasksParams } from "@/lib/api/domains/office-extended-api";
import { flattenTasksPaginated, officeQueryOptions } from "@/lib/query/query-options/office";
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
function mapSortField(field: TaskSortField): ListTasksParams["sort"] | undefined {
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

function buildParams(
  filters: TaskFilterState,
  sortField: TaskSortField,
  sortDir: TaskSortDir,
  limit: number,
  includeSystem: boolean,
): ListTasksParams {
  const params: ListTasksParams = { limit };
  const status = canonicalStatusesToBackend(filters.statuses);
  if (status.length > 0) params.status = status;
  if (filters.priorities.length > 0) params.priority = filters.priorities;
  // Backend currently accepts a single assignee/project. With multi-select we
  // omit the server filter and let the page-local filter narrow what's
  // already loaded — so multi-value selections can miss matches beyond the
  // first page until the backend grows repeated `assignee[]` / `project[]`
  // params.
  if (filters.assigneeIds.length === 1) params.assignee = filters.assigneeIds[0];
  if (filters.projectIds.length === 1) params.project = filters.projectIds[0];
  const sort = mapSortField(sortField);
  if (sort) {
    params.sort = sort;
    params.order = sortDir;
  }
  if (includeSystem) params.include_system = true;
  return params;
}

export type UsePaginatedTasksResult = {
  tasks: OfficeTask[];
  isLoading: boolean;
  loadMore: () => void;
  hasMore: boolean;
  isLoadingMore: boolean;
  refetch: () => Promise<void>;
};

/**
 * Owns the lifecycle of the office tasks list: server-side filter / sort /
 * keyset pagination via the Stream-E `/workspaces/:wsId/tasks?...` endpoint.
 *
 * Backed by TanStack Query's `useInfiniteQuery` against
 * `officeQueryOptions.tasksPaginated`. WS-driven updates flow in via the
 * office bridge (`apps/web/lib/query/bridge/office.ts`), which invalidates
 * the paginated cache key on task lifecycle events so every page refetches.
 *
 * Accepts filters / sort as explicit params (rather than reading from
 * Zustand) so callers can keep UI state in local React state per the TQ
 * migration plan.
 */
export function usePaginatedTasks(
  workspaceId: string | null,
  includeSystem: boolean,
  filters: TaskFilterState,
  sortField: TaskSortField,
  sortDir: TaskSortDir,
): UsePaginatedTasksResult {
  const params = useMemo(
    () => buildParams(filters, sortField, sortDir, DEFAULT_PAGE_LIMIT, includeSystem),
    [filters, sortField, sortDir, includeSystem],
  );

  const query = useInfiniteQuery({
    ...officeQueryOptions.tasksPaginated(workspaceId ?? "", params),
    enabled: !!workspaceId,
  });

  const tasks = useMemo(() => flattenTasksPaginated(query.data), [query.data]);

  const loadMore = useCallback(() => {
    if (!query.hasNextPage || query.isFetchingNextPage) return;
    void query.fetchNextPage().catch((err) => {
      toast.error(err instanceof Error ? err.message : "Failed to load more tasks");
    });
  }, [query]);

  const refetch = useCallback(async () => {
    try {
      await query.refetch();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to refresh tasks");
    }
  }, [query]);

  return {
    tasks,
    isLoading: query.isPending && !!workspaceId,
    loadMore,
    hasMore: !!query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    refetch,
  };
}
