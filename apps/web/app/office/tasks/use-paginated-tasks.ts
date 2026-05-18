import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { listTasks, type ListTasksParams } from "@/lib/api/domains/office-extended-api";
import type { TaskFilterState, TaskSortDir, TaskSortField } from "@/lib/state/slices/office/types";
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
  const appendTasks = useAppStore((s) => s.appendTasks);
  const setTasksLoading = useAppStore((s) => s.setTasksLoading);
  const filters = useAppStore((s) => s.office.tasks.filters);
  const sortField = useAppStore((s) => s.office.tasks.sortField);
  const sortDir = useAppStore((s) => s.office.tasks.sortDir);

  // Cursor + the params snapshot that produced it, kept atomically so a
  // stale cursor from a previous filter set can't be used for loadMore.
  const [page, setPage] = useState<{
    cursor?: string;
    id?: string;
    key: string;
  }>({ key: "" });
  const [isLoadingMore, setIsLoadingMore] = useState(false);

  // Derive the live params + key from filter/sort state so render and
  // event handlers see a consistent snapshot without ref reads.
  const params = useMemo(
    () => buildParams(filters, sortField, sortDir, DEFAULT_PAGE_LIMIT, includeSystem),
    [filters, sortField, sortDir, includeSystem],
  );
  const paramsKey = useMemo(
    () => JSON.stringify(params) + ":" + (workspaceId ?? ""),
    [params, workspaceId],
  );
  // Mirror the latest snapshot into a ref for refetch() callers (WS
  // events) so they don't pull from a stale closure.
  const paramsRef = useRef<ListTasksParams>(params);
  useEffect(() => {
    paramsRef.current = params;
  }, [params]);

  // Initial fetch + refetch on workspace / filter / sort change. Cursor
  // reset is rolled into the same setPage call as the fetch result to
  // avoid a separate setState pass inside the effect body. `params` and
  // `paramsKey` derive from the dependencies, so they need not be listed.
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    setTasksLoading(true);
    const key = paramsKey;
    listTasks(workspaceId, params)
      .then((res) => {
        if (cancelled) return;
        setTasks(res.tasks ?? []);
        setPage({ cursor: res.next_cursor || undefined, id: res.next_id || undefined, key });
      })
      .catch((err) => {
        if (!cancelled) toast.error(err instanceof Error ? err.message : "Failed to load tasks");
      })
      .finally(() => {
        if (!cancelled) setTasksLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, params, paramsKey, setTasks, setTasksLoading]);

  const loadMore = useCallback(() => {
    // Only paginate when the cursor matches the current params snapshot.
    if (!workspaceId || !page.cursor || isLoadingMore) return;
    if (page.key !== paramsKey) return;
    setIsLoadingMore(true);
    const next: ListTasksParams = { ...params, cursor: page.cursor, cursor_id: page.id };
    listTasks(workspaceId, next)
      .then((res) => {
        appendTasks(res.tasks ?? []);
        setPage({
          cursor: res.next_cursor || undefined,
          id: res.next_id || undefined,
          key: page.key,
        });
      })
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : "Failed to load more tasks");
      })
      .finally(() => setIsLoadingMore(false));
  }, [workspaceId, page, paramsKey, params, isLoadingMore, appendTasks]);

  const refetch = useCallback(async () => {
    if (!workspaceId) return;
    // Use the latest params snapshot via ref so a stale closure from a
    // long-lived WS subscription doesn't post old filters.
    const liveParams = paramsRef.current;
    const liveKey = JSON.stringify(liveParams) + ":" + workspaceId;
    try {
      const res = await listTasks(workspaceId, liveParams);
      setTasks(res.tasks ?? []);
      setPage({
        cursor: res.next_cursor || undefined,
        id: res.next_id || undefined,
        key: liveKey,
      });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to refresh tasks");
    }
  }, [workspaceId, setTasks]);

  return {
    loadMore,
    hasMore: !!page.cursor && page.key === paramsKey,
    isLoadingMore,
    refetch,
  };
}
