import { useCallback, useEffect, useRef, useState } from "react";
import type { PaginationState } from "@tanstack/react-table";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { normalizePathname } from "@/lib/links";
import {
  resolveTasksListQuery,
  type TasksListArchiveMode,
  type TasksListScope,
} from "@/lib/tasks/tasks-list-query";

type Input = TasksListScope & {
  workspaceId: string | null;
  persistScope: (scope: Partial<TasksListScope>) => void;
};

export function useTasksListQueryState({ workspaceId, persistScope, ...defaults }: Input) {
  const params = useSearchParams();
  const router = useRouter();
  const parsed = resolveTasksListQuery(params, defaults);
  const previous = useRef({ workspaceId, archiveMode: parsed.archiveMode });
  const changedWorkspace = previous.current.workspaceId !== workspaceId;
  const query = changedWorkspace
    ? {
        ...parsed,
        archiveMode: previous.current.archiveMode,
        searchQuery: "",
        workflowId: null,
        repositoryId: null,
      }
    : parsed;
  const filterKey = JSON.stringify([workspaceId, query]);
  const [page, setPage] = useState({ key: filterKey, pageIndex: 0, pageSize: 25 });
  const pagination = {
    pageIndex: page.key === filterKey ? page.pageIndex : 0,
    pageSize: page.pageSize,
  };

  const write = useCallback(
    (patch: Record<string, string | null>, replace = false) => {
      const next = new URLSearchParams(window.location.search);
      for (const [key, value] of Object.entries(patch)) {
        if (value === null) next.delete(key);
        else next.set(key, value);
      }
      const href = `/tasks?${next.toString()}`;
      if (replace) router.replace(href, { scroll: false });
      else router.push(href, { scroll: false });
    },
    [router],
  );

  useEffect(() => {
    if (
      changedWorkspace &&
      workspaceId &&
      normalizePathname(window.location.pathname) === "/tasks"
    ) {
      write(
        {
          workspace: workspaceId,
          archive: query.archiveMode,
          q: null,
          workflow: "all",
          repository: "all",
        },
        true,
      );
    }
    previous.current = { workspaceId, archiveMode: query.archiveMode };
  }, [changedWorkspace, workspaceId, query.archiveMode, write]);

  const setPagination = useCallback(
    (next: PaginationState | ((prev: PaginationState) => PaginationState)) => {
      setPage((current) => {
        const base = {
          pageIndex: current.key === filterKey ? current.pageIndex : 0,
          pageSize: current.pageSize,
        };
        const value = typeof next === "function" ? next(base) : next;
        return {
          ...value,
          pageIndex: value.pageSize === base.pageSize ? value.pageIndex : 0,
          key: filterKey,
        };
      });
    },
    [filterKey],
  );

  return {
    ...query,
    pagination,
    setPagination,
    setArchiveMode: (mode: TasksListArchiveMode) => write({ archive: mode }),
    setSearchQuery: (value: string) => write({ q: value || null }, true),
    setWorkflowId: (value: string | null) => {
      write({ workflow: value ?? "all" });
      persistScope({ workflowId: value });
    },
    setRepositoryId: (value: string | null) => {
      write({ repository: value ?? "all" });
      persistScope({ repositoryId: value });
    },
    clearFilters: () => {
      write({ q: null, workflow: "all", repository: "all" });
      persistScope({ workflowId: null, repositoryId: null });
    },
  };
}
