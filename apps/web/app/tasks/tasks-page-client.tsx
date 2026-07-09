"use client";

import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import type { PaginationState } from "@tanstack/react-table";
import { archiveTask, deleteTask, listTasksByWorkspace, updateUserSettings } from "@/lib/api";
import { KanbanHeader } from "@/components/kanban/kanban-header";
import { MobileSearchBar } from "@/components/kanban/mobile-search-bar";
import type { Task, Workspace, Workflow, Repository } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useDebounce } from "@/hooks/use-debounce";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { linkToTask } from "@/lib/links";
import { shouldSkipInitialTasksFetch } from "./tasks-page-fetch-policy";
import { TasksListView } from "./tasks-list-view";
import {
  parseTasksListGroup,
  parseTasksListSort,
  sortTasksForList,
  type TasksListGroup,
  type TasksListSort,
} from "@/lib/tasks/tasks-list-options";

interface TasksPageClientProps {
  workspaces: Workspace[];
  initialWorkspaceId?: string;
  initialWorkflows: Workflow[];
  initialRepositories: Repository[];
  initialTasks: Task[];
  initialTotal: number;
  initialDataLoaded?: boolean;
  initialSort: TasksListSort;
  initialGroup: TasksListGroup;
}

type UseTaskOperationsParams = {
  activeWorkspaceId: string | null;
  activeWorkflowId: string | null;
  selectedRepositoryId: string | null;
  pagination: PaginationState;
  debouncedQuery: string;
  showArchived: boolean;
  tasksListSort: TasksListSort;
  setTasks: (tasks: Task[]) => void;
  setTotal: (total: number) => void;
};

function useLatestWorkspaceRequest(activeWorkspaceId: string | null) {
  const latestFetchRef = useRef({ seq: 0, workspaceId: activeWorkspaceId });

  useEffect(() => {
    latestFetchRef.current.workspaceId = activeWorkspaceId;
  }, [activeWorkspaceId]);

  const beginRequest = useCallback((workspaceId: string) => {
    const seq = latestFetchRef.current.seq + 1;
    latestFetchRef.current = { seq, workspaceId };
    return seq;
  }, []);

  const isCurrentRequest = useCallback(
    (seq: number, workspaceId: string) =>
      latestFetchRef.current.seq === seq && latestFetchRef.current.workspaceId === workspaceId,
    [],
  );

  return { beginRequest, isCurrentRequest };
}

function useTaskOperations({
  activeWorkspaceId,
  activeWorkflowId,
  selectedRepositoryId,
  pagination,
  debouncedQuery,
  showArchived,
  tasksListSort,
  setTasks,
  setTotal,
}: UseTaskOperationsParams) {
  const { toast } = useToast();
  const [isLoading, setIsLoading] = useState(false);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);
  const { beginRequest, isCurrentRequest } = useLatestWorkspaceRequest(activeWorkspaceId);

  const fetchTasks = useCallback(async () => {
    if (!activeWorkspaceId) return;
    const requestSeq = beginRequest(activeWorkspaceId);
    const shouldCommit = () => isCurrentRequest(requestSeq, activeWorkspaceId);
    setIsLoading(true);
    try {
      const result = await listTasksByWorkspace(activeWorkspaceId, {
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        query: debouncedQuery,
        includeArchived: showArchived,
        workflowId: activeWorkflowId,
        repositoryId: selectedRepositoryId,
        sort: tasksListSort,
      });
      if (!shouldCommit()) return;
      setTasks(result.tasks);
      setTotal(result.total);
    } catch (err) {
      if (!shouldCommit()) return;
      toast({
        title: "Failed to load tasks",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "error",
      });
    } finally {
      if (shouldCommit()) setIsLoading(false);
    }
  }, [
    activeWorkspaceId,
    activeWorkflowId,
    selectedRepositoryId,
    pagination.pageIndex,
    pagination.pageSize,
    debouncedQuery,
    showArchived,
    tasksListSort,
    beginRequest,
    isCurrentRequest,
    toast,
    setTasks,
    setTotal,
  ]);

  const handleArchive = useCallback(
    async (taskId: string, opts?: { cascade?: boolean }) => {
      try {
        await archiveTask(taskId, opts);
        toast({ title: "Task archived", description: "The task has been archived successfully." });
        fetchTasks();
      } catch (err) {
        toast({
          title: "Failed to archive task",
          description: err instanceof Error ? err.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [fetchTasks, toast],
  );

  const handleDelete = useCallback(
    async (taskId: string, opts?: { cascade?: boolean }) => {
      setDeletingTaskId(taskId);
      try {
        await deleteTask(taskId, opts);
        fetchTasks();
      } catch (err) {
        toast({
          title: "Failed to delete task",
          description: err instanceof Error ? err.message : "Unknown error",
          variant: "error",
        });
      } finally {
        setDeletingTaskId(null);
      }
    },
    [fetchTasks, toast],
  );

  return { isLoading, deletingTaskId, fetchTasks, handleArchive, handleDelete };
}

function useTasksPageViewState({
  initialWorkflows,
  initialRepositories,
  initialTasks,
  initialTotal,
  initialSort,
  initialGroup,
  storeRepositories,
}: {
  initialWorkflows: Workflow[];
  initialRepositories: Repository[];
  initialTasks: Task[];
  initialTotal: number;
  initialSort: TasksListSort;
  initialGroup: TasksListGroup;
  storeRepositories: Repository[];
}) {
  const [workflows, setWorkflows] = useState(initialWorkflows);
  const repositories = storeRepositories.length > 0 ? storeRepositories : initialRepositories;
  const [tasks, setTasks] = useState(initialTasks);
  const [total, setTotal] = useState(initialTotal);
  const [searchQuery, setSearchQuery] = useState("");
  const [tasksListSort, setTasksListSort] = useState<TasksListSort>(initialSort);
  const [tasksListGroup, setTasksListGroup] = useState<TasksListGroup>(initialGroup);
  const [showArchived, setShowArchived] = useState(false);
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 25 });

  useEffect(() => {
    setWorkflows(initialWorkflows);
  }, [initialWorkflows]);

  return {
    workflows,
    repositories,
    tasks,
    setTasks,
    total,
    setTotal,
    searchQuery,
    setSearchQuery,
    tasksListSort,
    setTasksListSort,
    tasksListGroup,
    setTasksListGroup,
    showArchived,
    setShowArchived,
    pagination,
    setPagination,
  };
}

function useTasksPageEffects({
  debouncedQuery,
  setPagination,
  activeWorkspaceId,
  fetchTasks,
  pagination,
  showArchived,
  activeWorkflowId,
  selectedRepositoryId,
  initialDataLoaded = false,
}: {
  debouncedQuery: string;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  activeWorkspaceId: string | null;
  fetchTasks: () => void;
  pagination: PaginationState;
  showArchived: boolean;
  activeWorkflowId: string | null;
  selectedRepositoryId: string | null;
  initialDataLoaded?: boolean;
}) {
  const skippedInitialFetchRef = useRef(false);

  useEffect(() => {
    void Promise.resolve().then(() => setPagination((prev) => ({ ...prev, pageIndex: 0 })));
  }, [debouncedQuery, activeWorkflowId, selectedRepositoryId, setPagination]);

  useEffect(() => {
    if (
      shouldSkipInitialTasksFetch({
        hasInitialData: initialDataLoaded,
        alreadySkipped: skippedInitialFetchRef.current,
        pageIndex: pagination.pageIndex,
        debouncedQuery,
        showArchived,
      })
    ) {
      skippedInitialFetchRef.current = true;
      return;
    }
    if (activeWorkspaceId) fetchTasks();
  }, [
    activeWorkspaceId,
    pagination.pageIndex,
    pagination.pageSize,
    debouncedQuery,
    showArchived,
    fetchTasks,
    initialDataLoaded,
  ]);
}

function useTasksPageComputed({
  total,
  pagination,
  router,
}: {
  total: number;
  pagination: PaginationState;
  router: ReturnType<typeof useRouter>;
}) {
  const pageCount = useMemo(
    () => Math.ceil(total / pagination.pageSize),
    [total, pagination.pageSize],
  );
  const handleRowClick = useCallback(
    (task: Task) => {
      router.push(linkToTask(task.id));
    },
    [router],
  );

  return { pageCount, handleRowClick };
}

function useTasksPageSetup(props: TasksPageClientProps) {
  const router = useRouter();
  const {
    activeWorkspaceId,
    activeWorkflowId,
    repositories: storeRepositories,
    selectedRepositoryId,
  } = useKanbanDisplaySettings();
  const viewState = useTasksPageViewState({
    initialWorkflows: props.initialWorkflows,
    initialRepositories: props.initialRepositories,
    initialTasks: props.initialTasks,
    initialTotal: props.initialTotal,
    initialSort: props.initialSort,
    initialGroup: props.initialGroup,
    storeRepositories,
  });
  const debouncedQuery = useDebounce(viewState.searchQuery, 300);
  const ops = useTaskOperations({
    activeWorkspaceId,
    activeWorkflowId,
    selectedRepositoryId,
    pagination: viewState.pagination,
    debouncedQuery,
    showArchived: viewState.showArchived,
    tasksListSort: viewState.tasksListSort,
    setTasks: viewState.setTasks,
    setTotal: viewState.setTotal,
  });
  useTasksPageEffects({
    debouncedQuery,
    setPagination: viewState.setPagination,
    activeWorkspaceId,
    fetchTasks: ops.fetchTasks,
    pagination: viewState.pagination,
    showArchived: viewState.showArchived,
    activeWorkflowId,
    selectedRepositoryId,
    initialDataLoaded: props.initialDataLoaded,
  });
  const computed = useTasksPageComputed({
    total: viewState.total,
    pagination: viewState.pagination,
    router,
  });
  return { ...viewState, ...ops, ...computed, activeWorkspaceId, debouncedQuery };
}

function useTasksListPreferenceSync({
  tasksListSort,
  setTasksListSort,
  tasksListGroup,
  setTasksListGroup,
  setTasks,
  setPagination,
}: {
  tasksListSort: TasksListSort;
  setTasksListSort: (sort: TasksListSort) => void;
  tasksListGroup: TasksListGroup;
  setTasksListGroup: (group: TasksListGroup) => void;
  setTasks: (tasks: Task[] | ((prev: Task[]) => Task[])) => void;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
}) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const store = useAppStoreApi();

  const persistPreferences = useCallback(
    (sort: TasksListSort, group: TasksListGroup) => {
      const current = store.getState().userSettings;
      const setUserSettings = store.getState().setUserSettings;
      if (current.tasksListSort === sort && current.tasksListGroup === group) {
        return;
      }
      setUserSettings({
        ...current,
        tasksListSort: sort,
        tasksListGroup: group,
        loaded: true,
      });
      updateUserSettings(
        {
          tasks_list_sort: sort,
          tasks_list_group: group,
        },
        { cache: "no-store" },
      ).catch(() => {});
    },
    [store],
  );

  useEffect(() => {
    const hasSortParam = searchParams.has("sort");
    const hasGroupParam = searchParams.has("group");
    if (!hasSortParam && !hasGroupParam) return;

    const nextSort = hasSortParam ? parseTasksListSort(searchParams.get("sort")) : tasksListSort;
    const nextGroup = hasGroupParam
      ? parseTasksListGroup(searchParams.get("group"))
      : tasksListGroup;
    if (nextSort !== tasksListSort) {
      setTasksListSort(nextSort);
      setTasks((prev) => sortTasksForList(prev, nextSort));
      setPagination((prev) => ({ ...prev, pageIndex: 0 }));
    }
    if (nextGroup !== tasksListGroup) {
      setTasksListGroup(nextGroup);
    }
    persistPreferences(nextSort, nextGroup);
  }, [
    persistPreferences,
    searchParams,
    setPagination,
    setTasks,
    setTasksListGroup,
    setTasksListSort,
    tasksListGroup,
    tasksListSort,
  ]);

  const writeUrl = useCallback(
    (sort: TasksListSort, group: TasksListGroup) => {
      const params = new URLSearchParams(window.location.search);
      params.set("sort", sort);
      params.set("group", group);
      const query = params.toString();
      router.replace(`${window.location.pathname}${query ? `?${query}` : ""}`, { scroll: false });
    },
    [router],
  );

  const handleSortChange = useCallback(
    (sort: TasksListSort) => {
      setTasksListSort(sort);
      setTasks((prev) => sortTasksForList(prev, sort));
      setPagination((prev) => ({ ...prev, pageIndex: 0 }));
      writeUrl(sort, tasksListGroup);
      persistPreferences(sort, tasksListGroup);
    },
    [persistPreferences, setPagination, setTasks, setTasksListSort, tasksListGroup, writeUrl],
  );

  const handleGroupChange = useCallback(
    (group: TasksListGroup) => {
      setTasksListGroup(group);
      writeUrl(tasksListSort, group);
      persistPreferences(tasksListSort, group);
    },
    [persistPreferences, setTasksListGroup, tasksListSort, writeUrl],
  );

  return { handleSortChange, handleGroupChange };
}

export function TasksPageClient(props: TasksPageClientProps) {
  const s = useTasksPageSetup(props);
  const setMobileSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
  const isMobileSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const { isMobile } = useResponsiveBreakpoint();
  const { handleSortChange, handleGroupChange } = useTasksListPreferenceSync({
    tasksListSort: s.tasksListSort,
    setTasksListSort: s.setTasksListSort,
    tasksListGroup: s.tasksListGroup,
    setTasksListGroup: s.setTasksListGroup,
    setTasks: s.setTasks,
    setPagination: s.setPagination,
  });

  useEffect(() => {
    setMobileSearchOpen(false);
    return () => setMobileSearchOpen(false);
  }, [setMobileSearchOpen]);

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <KanbanHeader
        workspaceId={s.activeWorkspaceId ?? undefined}
        currentPage="tasks"
        hideTitle
        searchQuery={s.searchQuery}
        onSearchChange={s.setSearchQuery}
        isSearchLoading={s.isLoading && !!s.debouncedQuery}
      />
      {isMobile && isMobileSearchOpen && (
        <MobileSearchBar searchQuery={s.searchQuery} onSearchChange={s.setSearchQuery} />
      )}
      <TasksListView
        showArchived={s.showArchived}
        setShowArchived={s.setShowArchived}
        tasksListSort={s.tasksListSort}
        onTasksListSortChange={handleSortChange}
        tasksListGroup={s.tasksListGroup}
        onTasksListGroupChange={handleGroupChange}
        tasks={s.tasks}
        workflows={s.workflows}
        repositories={s.repositories}
        total={s.total}
        pageCount={s.pageCount}
        pagination={s.pagination}
        setPagination={s.setPagination}
        isLoading={s.isLoading}
        handleRowClick={s.handleRowClick}
        deletingTaskId={s.deletingTaskId}
        handleArchive={s.handleArchive}
        handleDelete={s.handleDelete}
      />
    </div>
  );
}
