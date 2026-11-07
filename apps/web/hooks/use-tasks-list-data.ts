import { useCallback, useEffect, useRef, useState } from "react";
import type { PaginationState } from "@tanstack/react-table";
import type { Task } from "@/lib/types/http";
import { listTasksByWorkspace } from "@/lib/api";
import { tasksListArchiveParams, type TasksListQuery } from "@/lib/tasks/tasks-list-query";
import { resolveTasksListApiSort } from "@/lib/tasks/tasks-list-options";
import { shouldSkipInitialTasksFetch } from "@/app/tasks/tasks-page-fetch-policy";

type Input = {
  workspaceId: string | null;
  query: TasksListQuery;
  sort: string;
  pagination: PaginationState;
  enabled: boolean;
  onPageChange: (index: number) => void;
  initialData?: { tasks: Task[]; total: number };
};

type Snapshot = {
  key: string;
  tasks: Task[];
  total: number;
  pending: boolean;
  error: Error | null;
};

export function useTasksListData(input: Input) {
  const { workspaceId, query, sort, pagination, enabled } = input;
  const key = JSON.stringify([
    workspaceId,
    query.archiveMode,
    query.searchQuery,
    query.workflowId,
    query.repositoryId,
    sort,
    pagination,
  ]);
  const { latest, generation, mounted, controller } = useTasksListRequestOwner(input, key);
  const skippedInitial = useRef(false);
  const bootKey = useRef(input.initialData ? key : null);
  const [snapshot, setSnapshot] = useState<Snapshot>(() => ({
    key: input.initialData ? key : "",
    tasks: input.initialData?.tasks ?? [],
    total: input.initialData?.total ?? 0,
    pending: false,
    error: null,
  }));

  const fetchTasks = useCallback(async (_silent?: boolean) => {
    const { input: current, key: requestKey } = latest.current;
    if (!current.workspaceId || !current.enabled) return;
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    const attempt = ++generation.current;
    const ownsRequest = () =>
      mounted.current &&
      generation.current === attempt &&
      latest.current.key === requestKey &&
      latest.current.input.enabled;
    setSnapshot({ key: requestKey, tasks: [], total: 0, pending: true, error: null });
    try {
      const data = await listTasksByWorkspace(
        current.workspaceId,
        {
          ...tasksListArchiveParams(current.query.archiveMode),
          query: current.query.searchQuery,
          workflowId: current.query.workflowId,
          repositoryId: current.query.repositoryId,
          sort: resolveTasksListApiSort(current.sort),
          page: current.pagination.pageIndex + 1,
          pageSize: current.pagination.pageSize,
        },
        { init: { signal: abort.signal } },
      );
      if (!ownsRequest()) return;
      const lastPage = Math.max(0, Math.ceil(data.total / current.pagination.pageSize) - 1);
      if (current.pagination.pageIndex > lastPage) {
        current.onPageChange(lastPage);
        return;
      }
      setSnapshot({
        key: requestKey,
        tasks: data.tasks,
        total: data.total,
        pending: false,
        error: null,
      });
    } catch (error) {
      if (!ownsRequest() || abort.signal.aborted) return;
      setSnapshot({
        key: requestKey,
        tasks: [],
        total: 0,
        pending: false,
        error: error instanceof Error ? error : new Error(String(error)),
      });
    }
  }, []);

  useEffect(() => {
    if (!enabled || !workspaceId) {
      controller.current?.abort();
      return;
    }
    if (
      shouldSkipInitialTasksFetch({
        hasInitialData: bootKey.current === key,
        alreadySkipped: skippedInitial.current,
        pageIndex: pagination.pageIndex,
        debouncedQuery: query.searchQuery,
        showArchived: query.archiveMode !== "active",
      })
    ) {
      skippedInitial.current = true;
      return;
    }
    void fetchTasks();
  }, [key, enabled, workspaceId, fetchTasks]);

  const current = enabled && snapshot.key === key;
  return {
    tasks: current ? snapshot.tasks : [],
    total: current ? snapshot.total : 0,
    isLoading: Boolean(workspaceId) && (!current || snapshot.pending),
    error: current ? snapshot.error : null,
    fetchTasks,
  };
}

function useTasksListRequestOwner(input: Input, key: string) {
  const latest = useRef({ input, key });
  latest.current = { input, key };
  const generation = useRef(0);
  const mounted = useRef(true);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      generation.current++;
      controller.current?.abort();
    };
  }, []);
  return { latest, generation, mounted, controller };
}
