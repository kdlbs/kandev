import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import {
  fetchTask,
  fetchWorkflowSnapshot,
  getSubtaskCount,
  listTasksByWorkspace,
  listWorkflows,
} from "@/lib/api/domains/kanban-api";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { qk, type TaskListFilters } from "../keys";
import { withSignal } from "./utils";

const DEFAULT_TASK_PAGE_SIZE = 50;

export function workflowsQueryOptions(workspaceId: string, params?: { includeHidden?: boolean }) {
  return queryOptions({
    queryKey: qk.workflows.all(workspaceId, params),
    queryFn: async ({ signal }) => {
      const response = await listWorkflows(workspaceId, { ...withSignal(signal), ...params });
      return response.workflows;
    },
    enabled: Boolean(workspaceId),
  });
}

export function workflowSnapshotQueryOptions(workflowId: string) {
  return queryOptions({
    queryKey: qk.workflows.snapshot(workflowId),
    queryFn: ({ signal }) => fetchWorkflowSnapshot(workflowId, withSignal(signal)),
    enabled: Boolean(workflowId),
  });
}

export function workflowStepsQueryOptions(workflowId: string) {
  return queryOptions({
    queryKey: qk.workflows.steps(workflowId),
    queryFn: async ({ signal }) => {
      const response = await listWorkflowSteps(workflowId, withSignal(signal));
      return [...response.steps].sort((a, b) => a.position - b.position);
    },
    enabled: Boolean(workflowId),
  });
}

export function taskQueryOptions(taskId: string) {
  return queryOptions({
    queryKey: qk.tasks.detail(taskId),
    queryFn: ({ signal }) => fetchTask(taskId, withSignal(signal)),
    enabled: Boolean(taskId),
  });
}

export function workspaceTasksQueryOptions(workspaceId: string, filters: TaskListFilters = {}) {
  return queryOptions({
    queryKey: qk.tasks.page(workspaceId, filters),
    queryFn: ({ signal }) =>
      listTasksByWorkspace(
        workspaceId,
        {
          ...filters,
          sort: filters.sort ?? undefined,
          page: filters.page ?? 1,
          pageSize: filters.pageSize ?? DEFAULT_TASK_PAGE_SIZE,
        },
        withSignal(signal),
      ),
    enabled: Boolean(workspaceId),
  });
}

export function workspaceTasksInfiniteQueryOptions(
  workspaceId: string,
  filters: TaskListFilters = {},
) {
  return infiniteQueryOptions({
    queryKey: qk.tasks.infinite(workspaceId, filters),
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) =>
      listTasksByWorkspace(
        workspaceId,
        {
          ...filters,
          sort: filters.sort ?? undefined,
          page: Number(pageParam),
          pageSize: filters.pageSize ?? DEFAULT_TASK_PAGE_SIZE,
        },
        withSignal(signal),
      ),
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((total, page) => total + page.tasks.length, 0);
      return loaded < lastPage.total ? allPages.length + 1 : undefined;
    },
    enabled: Boolean(workspaceId),
  });
}

export function subtaskCountQueryOptions(taskId: string) {
  return queryOptions({
    queryKey: qk.tasks.subtaskCount(taskId),
    queryFn: ({ signal }) => getSubtaskCount(taskId, withSignal(signal)),
    enabled: Boolean(taskId),
  });
}
