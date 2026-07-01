import type { QueryClient } from "@tanstack/react-query";
import type { OfficeTask } from "@/lib/state/slices/office/types";

type OfficeTasksPage = {
  tasks?: unknown;
};

type OfficeTasksInfiniteCache = {
  pages?: unknown;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isOfficeTasksQueryKey(key: readonly unknown[], workspaceId: string): boolean {
  return (
    key[0] === "office" && key[1] === "workspaces" && key[2] === workspaceId && key[3] === "tasks"
  );
}

function readTaskFromPage(page: OfficeTasksPage, taskId: string): OfficeTask | null {
  if (!Array.isArray(page.tasks)) return null;
  for (const task of page.tasks) {
    if (isRecord(task) && task.id === taskId) return task as OfficeTask;
  }
  return null;
}

export function readOfficeTaskFromCachedPages(
  queryClient: QueryClient,
  workspaceId: string | null,
  taskId: string,
): OfficeTask | null {
  if (!workspaceId) return null;
  for (const query of queryClient.getQueryCache().findAll()) {
    if (!isOfficeTasksQueryKey(query.queryKey, workspaceId)) continue;
    const current = queryClient.getQueryData<OfficeTasksInfiniteCache>(query.queryKey);
    if (!Array.isArray(current?.pages)) continue;
    for (const page of current.pages) {
      if (!isRecord(page)) continue;
      const task = readTaskFromPage(page, taskId);
      if (task) return task;
    }
  }
  return null;
}
