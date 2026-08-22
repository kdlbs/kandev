"use client";

import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import type { PaginationState } from "@tanstack/react-table";
import {
  archiveTask,
  deleteTask,
  listTasksByWorkspace,
  unarchiveTask,
  updateUserSettings,
} from "@/lib/api";
import type { Task, Workspace, Workflow, Repository } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
// Module-level `t`: every use below is inside a callback or a plain helper
// (`errorDescription`), so it resolves at invocation rather than at import —
// the case `apps/web/CLAUDE.md` sanctions. None of it renders as JSX.
import { t } from "@/lib/i18n";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useDebounce } from "@/hooks/use-debounce";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskListingView } from "@/hooks/use-task-listing-view";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { useWorkflowSnapshot } from "@/hooks/use-workflow-snapshot";
import { useWorkspacePRs } from "@/hooks/domains/github/use-task-pr";
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useTaskListFacets } from "@/hooks/use-task-list-facets";
import { useTaskListFacetSelection } from "@/hooks/use-task-list-facet-selection";
import { linkToTask } from "@/lib/links";
import { unarchiveToastPayload } from "@/lib/tasks/unarchive-feedback";
import { shouldSkipInitialTasksFetch } from "./tasks-page-fetch-policy";
import { TasksPageContent } from "./tasks-page-content";
import { type MobileTaskStep } from "./mobile-tasks-create-dialog";
import { MobileTasksActions } from "./mobile-tasks-actions";
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

const EMPTY_WORKFLOW_STEPS: MobileTaskStep[] = [];

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
  const { beginRequest, isCurrentRequest } = useLatestWorkspaceRequest(activeWorkspaceId);

  const fetchTasks = useCallback(
    async (silent = false) => {
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
        if (!silent) {
          toast({
            title: t("tasks:failedToLoadTasks"),
            description: errorDescription(err),
            variant: "error",
          });
        } else {
          console.error("[TasksPage] Silent foreground refresh failed:", err);
        }
      } finally {
        if (shouldCommit()) setIsLoading(false);
      }
    },
    [
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
    ],
  );

  const mutations = useTaskMutations(fetchTasks);
  return { isLoading, fetchTasks, ...mutations };
}

function errorDescription(err: unknown): string {
  return err instanceof Error ? err.message : t("common:unknownError");
}

function useTaskMutations(fetchTasks: () => void) {
  const { toast } = useToast();
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);

  const handleArchive = useCallback(
    async (taskId: string, opts?: { cascade?: boolean }) => {
      try {
        await archiveTask(taskId, opts);
        toast({
          title: t("tasks:taskArchived"),
          description: t("tasks:taskArchivedDescription"),
        });
        fetchTasks();
      } catch (err) {
        toast({
          title: t("tasks:failedToArchiveTask"),
          description: errorDescription(err),
          variant: "error",
        });
      }
    },
    [fetchTasks, toast],
  );

  const handleUnarchive = useCallback(
    async (taskId: string) => {
      try {
        const result = await unarchiveTask(taskId);
        toast(unarchiveToastPayload(result));
        fetchTasks();
      } catch (err) {
        toast({
          title: t("tasks:failedToUnarchiveTask"),
          description: errorDescription(err),
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
          title: t("tasks:failedToDeleteTask"),
          description: errorDescription(err),
          variant: "error",
        });
      } finally {
        setDeletingTaskId(null);
      }
    },
    [fetchTasks, toast],
  );

  return { deletingTaskId, handleArchive, handleUnarchive, handleDelete };
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
  return { ...viewState, ...ops, ...computed, activeWorkspaceId, activeWorkflowId, debouncedQuery };
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
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const { setView } = useTaskListingView();
  const setMobileSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
  const isMobileSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const { isMobile } = useResponsiveBreakpoint();
  const showTaskDetails = useAppStore((state) => state.userSettings.tasksListShowDetails ?? false);
  const { facets, values: facetValues } = useTaskListFacets(s.tasks, s.activeWorkspaceId);
  const facetOptions = useMemo(
    () => facets.map((facet) => ({ value: facet.key, label: facet.label })),
    [facets],
  );
  const activeSteps = useAppStore((state) =>
    state.kanban.workflowId === s.activeWorkflowId ? state.kanban.steps : EMPTY_WORKFLOW_STEPS,
  );
  useWorkflowSnapshot(s.activeWorkflowId);
  // Task-title previews render PR/MR glyphs independently of the list-details
  // preference, so both workspace caches must be hydrated here.
  useWorkspacePRs(s.activeWorkspaceId);
  useWorkspaceMRs(s.activeWorkspaceId);
  useForegroundRefresh(() => s.fetchTasks(true), Boolean(s.activeWorkspaceId), s.activeWorkspaceId);
  const { handleSortChange, handleGroupChange } = useTasksListPreferenceSync({
    tasksListSort: s.tasksListSort,
    setTasksListSort: s.setTasksListSort,
    tasksListGroup: s.tasksListGroup,
    setTasksListGroup: s.setTasksListGroup,
    setTasks: s.setTasks,
    setPagination: s.setPagination,
  });
  const { displayedTasks, sort, group, selectSort, selectGroup } = useTaskListFacetSelection({
    facetKeys: facetOptions.map((facet) => facet.value),
    coreSort: s.tasksListSort,
    coreGroup: s.tasksListGroup,
    tasks: s.tasks,
    facetValues,
    onCoreSortChange: handleSortChange,
    onCoreGroupChange: handleGroupChange,
  });

  useEffect(() => {
    setMobileSearchOpen(false);
    return () => setMobileSearchOpen(false);
  }, [setMobileSearchOpen]);

  useEffect(() => {
    setView("list");
  }, [setView]);

  return (
    <TasksPageContent
      header={{
        workspaceId: s.activeWorkspaceId ?? undefined,
        currentPage: "tasks",
        searchQuery: s.searchQuery,
        onSearchChange: s.setSearchQuery,
        isSearchLoading: s.isLoading && !!s.debouncedQuery,
        tasksListOptions: {
          showArchived: s.showArchived,
          onShowArchivedChange: s.setShowArchived,
          sort,
          onSortChange: selectSort,
          group,
          onGroupChange: selectGroup,
          facetOptions,
        },
      }}
      isMobile={isMobile}
      isMobileSearchOpen={isMobileSearchOpen}
      tasks={displayedTasks}
      workflows={s.workflows}
      repositories={s.repositories}
      facetOptions={facetOptions}
      facetValues={facetValues}
      total={s.total}
      pageCount={s.pageCount}
      pagination={s.pagination}
      setPagination={s.setPagination}
      isLoading={s.isLoading}
      showArchived={s.showArchived}
      showTaskDetails={showTaskDetails}
      sort={sort}
      group={group}
      onSortChange={selectSort}
      onGroupChange={selectGroup}
      onShowArchivedChange={s.setShowArchived}
      onRowClick={s.handleRowClick}
      deletingTaskId={s.deletingTaskId}
      onArchive={s.handleArchive}
      onUnarchive={s.handleUnarchive}
      onDelete={s.handleDelete}
      onRefresh={() => s.fetchTasks()}
      mobileActions={mobileTaskActions({
        isMobile,
        workspaceId: s.activeWorkspaceId,
        workflowId: s.activeWorkflowId,
        steps: activeSteps,
        open: isCreateOpen,
        onOpenChange: setIsCreateOpen,
        onCreated: () => s.fetchTasks(true),
      })}
    />
  );
}

function mobileTaskActions({
  isMobile,
  workspaceId,
  workflowId,
  steps,
  open,
  onOpenChange,
  onCreated,
}: {
  isMobile: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  steps: MobileTaskStep[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => Promise<void>;
}) {
  if (!isMobile || !workspaceId) return null;
  return (
    <MobileTasksActions
      workspaceId={workspaceId}
      workflowId={workflowId}
      steps={steps}
      open={open}
      onOpenChange={onOpenChange}
      onCreated={onCreated}
    />
  );
}
