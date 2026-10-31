"use client";

import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import type { PaginationState } from "@tanstack/react-table";
import { deleteTask, unarchiveTask, updateUserSettings } from "@/lib/api";
import type { DeleteTaskParams } from "@/lib/api/domains/kanban-api";
import type { Task, Workspace, Repository } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
// Module-level `t` is only used at invocation time in callbacks and helpers.
import { t } from "@/lib/i18n";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useDebounce } from "@/hooks/use-debounce";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskListingView } from "@/hooks/use-task-listing-view";
import { useWorkflowSnapshot } from "@/hooks/use-workflow-snapshot";
import { useWorkspacePRs } from "@/hooks/domains/github/use-task-pr";
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useTaskListFacets } from "@/hooks/use-task-list-facets";
import { useTaskListFacetSelection } from "@/hooks/use-task-list-facet-selection";
import { useTasksListStepRefresh } from "@/hooks/use-task-list-workflow-steps";
import { useTaskActions } from "@/hooks/use-task-actions";
import { linkToTask } from "@/lib/links";
import { unarchiveToastPayload } from "@/lib/tasks/unarchive-feedback";
import { isTaskDeleteDirtyWorktreeError } from "@/lib/api/task-delete-errors";
import { useTasksListQueryState } from "@/hooks/use-tasks-list-query-state";
import { useTasksListData } from "@/hooks/use-tasks-list-data";
import { useUserDisplaySettings } from "@/hooks/use-user-display-settings";
import {
  canUseTasksListBootData,
  tasksListFilterStatus,
  type TasksListScope,
  type TasksListMetadataReady,
} from "@/lib/tasks/tasks-list-query";
import { TasksPageContent } from "./tasks-page-content";
import { type MobileTaskStep } from "./mobile-tasks-create-dialog";
import { MobileTasksActions } from "./mobile-tasks-actions";
import {
  parseTasksListGroup,
  parseTasksListSort,
  type TasksListGroupPreference,
  type TasksListSortPreference,
} from "@/lib/tasks/tasks-list-options";

interface TasksPageClientProps {
  workspaces: Workspace[];
  initialWorkspaceId?: string;
  initialRepositories: Repository[];
  initialTasks: Task[];
  initialTotal: number;
  initialDataLoaded?: boolean;
  initialMetadataLoaded?: TasksListMetadataReady;
  initialSort: TasksListSortPreference;
  initialGroup: TasksListGroupPreference;
}

const EMPTY_WORKFLOW_STEPS: MobileTaskStep[] = [];

function errorDescription(err: unknown): string {
  return err instanceof Error ? err.message : t("common:unknownError");
}

function useTaskMutations(fetchTasks: () => void) {
  const { toast } = useToast();
  const { archiveTaskById } = useTaskActions();
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);

  const handleArchive = useCallback(
    async (taskId: string, opts?: { cascade?: boolean }) => {
      try {
        await archiveTaskById(taskId, opts);
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
    [archiveTaskById, fetchTasks, toast],
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
    async (taskId: string, opts?: DeleteTaskParams) => {
      setDeletingTaskId(taskId);
      try {
        const result = await deleteTask(taskId, opts);
        if (result?.workspace_cleanup === "retain") {
          toast({
            title: t("task:deleteKeptFilesTitle"),
            description: t(
              result.runtime_stop_pending
                ? "task:deleteKeptFilesPending"
                : "task:deleteKeptFilesComplete",
            ),
          });
        } else if (result?.workspace_cleanup_pending) {
          toast({
            title: t("task:deleteCleanupPendingTitle"),
            description: t("task:deleteCleanupPendingDescription"),
          });
        }
        fetchTasks();
      } catch (err) {
        toast({
          title: isTaskDeleteDirtyWorktreeError(err)
            ? t("task:deleteDirtyWorktreeTitle")
            : t("tasks:failedToDeleteTask"),
          description: isTaskDeleteDirtyWorktreeError(err)
            ? t("task:deleteDirtyWorktreeDescription")
            : errorDescription(err),
          variant: "error",
        });
      } finally {
        setDeletingTaskId(null);
      }
    },
    [fetchTasks, t, toast],
  );

  return { deletingTaskId, handleArchive, handleUnarchive, handleDelete };
}

function useTasksListMetadata(
  workspaceId: string | null,
  initialWorkspaceId: string | undefined,
  repositories: Repository[],
  query: TasksListScope,
  metadataLoaded: TasksListMetadataReady | undefined,
) {
  const workflows = useAppStore((state) => state.workflows.items);
  const context = useAppStore((state) => state.workspaceContextRead);
  const workspaceMatches = Boolean(workspaceId && initialWorkspaceId === workspaceId);
  const currentContext = context.workspaceId === workspaceId;
  return tasksListFilterStatus({
    query,
    workflowIds: workflows.filter((w) => w.workspaceId === workspaceId).map((w) => w.id),
    repositoryIds: repositories.map((r) => r.id),
    workflowReady: workspaceMatches && Boolean(metadataLoaded?.workflows),
    repositoryReady: workspaceMatches && Boolean(metadataLoaded?.repositories),
    workflowFailed: currentContext && Boolean(context.errors.workflows),
    repositoryFailed: currentContext && Boolean(context.errors.repositories),
  });
}

function useTasksPageSetup(props: TasksPageClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const display = useKanbanDisplaySettings();
  const { activeWorkspaceId } = display;
  const store = useAppStoreApi();
  const { settings, commitSettings } = useUserDisplaySettings({
    workspaceId: activeWorkspaceId,
    workflowId: display.activeWorkflowId,
  });
  const persistScope = useCallback(
    (patch: Partial<TasksListScope>) => {
      const current = store.getState().userSettings;
      const workflowId = patch.workflowId === undefined ? current.workflowId : patch.workflowId;
      let repositoryIds = current.repositoryIds;
      if (patch.repositoryId !== undefined)
        repositoryIds = patch.repositoryId ? [patch.repositoryId] : [];
      commitSettings({ ...current, workspaceId: activeWorkspaceId, workflowId, repositoryIds });
      if (patch.workflowId !== undefined) store.getState().setActiveWorkflow(workflowId);
    },
    [store, commitSettings, activeWorkspaceId],
  );
  const query = useTasksListQueryState({
    workspaceId: activeWorkspaceId,
    workflowId: display.activeWorkflowId,
    repositoryId: settings.repositoryIds[0] ?? null,
    persistScope,
  });
  const [tasksListSort, setTasksListSort] = useState<TasksListSortPreference>(props.initialSort);
  const [tasksListGroup, setTasksListGroup] = useState<TasksListGroupPreference>(
    props.initialGroup,
  );
  const fallbackRepositories =
    props.initialWorkspaceId === activeWorkspaceId ? props.initialRepositories : [];
  const repositories =
    display.repositories.length > 0 ? display.repositories : fallbackRepositories;
  const metadata = useTasksListMetadata(
    activeWorkspaceId,
    props.initialWorkspaceId,
    repositories,
    query,
    props.initialMetadataLoaded,
  );
  const debouncedQuery = useDebounce(query.searchQuery, 300);
  const requestedWorkspace = searchParams.get("workspace") ?? activeWorkspaceId;
  const onPageChange = useCallback(
    (pageIndex: number) => query.setPagination((prev) => ({ ...prev, pageIndex })),
    [query.setPagination],
  );
  const data = useTasksListData({
    workspaceId: activeWorkspaceId,
    query,
    sort: tasksListSort,
    pagination: query.pagination,
    enabled: Boolean(
      activeWorkspaceId &&
      requestedWorkspace === activeWorkspaceId &&
      !metadata.unavailableFilter &&
      !metadata.metadataLoading &&
      !metadata.metadataError &&
      (query.searchQuery === "" || debouncedQuery === query.searchQuery),
    ),
    onPageChange,
    initialData:
      props.initialDataLoaded &&
      canUseTasksListBootData(searchParams, props.initialWorkspaceId, activeWorkspaceId)
        ? { tasks: props.initialTasks, total: props.initialTotal }
        : undefined,
  });
  const mutations = useTaskMutations(data.fetchTasks);
  const pageCount = Math.ceil(data.total / query.pagination.pageSize);
  const handleRowClick = useCallback((task: Task) => router.push(linkToTask(task.id)), [router]);
  return {
    ...query,
    ...data,
    ...mutations,
    activeWorkspaceId,
    activeWorkflowId: query.workflowId,
    tasksListSort,
    setTasksListSort,
    tasksListGroup,
    setTasksListGroup,
    repositories,
    ...metadata,
    debouncedQuery,
    pageCount,
    handleRowClick,
  };
}

function useTasksListPreferenceSync({
  tasksListSort,
  setTasksListSort,
  tasksListGroup,
  setTasksListGroup,
  setPagination,
}: {
  tasksListSort: TasksListSortPreference;
  setTasksListSort: (sort: TasksListSortPreference) => void;
  tasksListGroup: TasksListGroupPreference;
  setTasksListGroup: (group: TasksListGroupPreference) => void;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
}) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const store = useAppStoreApi();
  // Preference snapshots must reach the server in selection order; a
  // fire-and-forget write can otherwise land after a newer one and restore the
  // previous sort/group pair.
  const pendingWrite = useRef<Promise<unknown>>(Promise.resolve());

  const persistPreferences = useCallback(
    (sort: TasksListSortPreference, group: TasksListGroupPreference) => {
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
      pendingWrite.current = pendingWrite.current
        .catch(() => undefined)
        .then(() =>
          updateUserSettings(
            {
              tasks_list_sort: sort,
              tasks_list_group: group,
            },
            { cache: "no-store" },
          ),
        )
        .catch(() => undefined);
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
    setTasksListGroup,
    setTasksListSort,
    tasksListGroup,
    tasksListSort,
  ]);

  const writeUrl = useCallback(
    (sort: TasksListSortPreference, group: TasksListGroupPreference) => {
      const params = new URLSearchParams(window.location.search);
      params.set("sort", sort);
      params.set("group", group);
      const query = params.toString();
      router.replace(`${window.location.pathname}${query ? `?${query}` : ""}`, { scroll: false });
    },
    [router],
  );

  const handleSortChange = useCallback(
    (sort: TasksListSortPreference) => {
      setTasksListSort(sort);
      setPagination((prev) => ({ ...prev, pageIndex: 0 }));
      writeUrl(sort, tasksListGroup);
      persistPreferences(sort, tasksListGroup);
    },
    [persistPreferences, setPagination, setTasksListSort, tasksListGroup, writeUrl],
  );

  const handleGroupChange = useCallback(
    (group: TasksListGroupPreference) => {
      setTasksListGroup(group);
      writeUrl(tasksListSort, group);
      persistPreferences(tasksListSort, group);
    },
    [persistPreferences, setTasksListGroup, tasksListSort, writeUrl],
  );

  return { handleSortChange, handleGroupChange };
}

function useTasksPageClientEffects({
  setMobileSearchOpen,
  setView,
}: {
  setMobileSearchOpen: (open: boolean) => void;
  setView: (view: "list") => void;
}) {
  useEffect(() => {
    setMobileSearchOpen(false);
    return () => setMobileSearchOpen(false);
  }, [setMobileSearchOpen]);

  useEffect(() => {
    setView("list");
  }, [setView]);
}

function useTasksListPresentation(s: ReturnType<typeof useTasksPageSetup>) {
  const { facets, values: facetValues } = useTaskListFacets(s.tasks, s.activeWorkspaceId);
  const facetOptions = useMemo(
    () => facets.map((facet) => ({ value: facet.key, label: facet.label })),
    [facets],
  );
  const activeSteps = useAppStore((state) =>
    state.kanban.workflowId === s.activeWorkflowId ? state.kanban.steps : EMPTY_WORKFLOW_STEPS,
  );
  useWorkflowSnapshot(s.activeWorkflowId);
  const { handleSortChange, handleGroupChange } = useTasksListPreferenceSync({
    tasksListSort: s.tasksListSort,
    setTasksListSort: s.setTasksListSort,
    tasksListGroup: s.tasksListGroup,
    setTasksListGroup: s.setTasksListGroup,
    setPagination: s.setPagination,
  });
  const { displayedTasks, sort, group, selectSort, selectGroup } = useTaskListFacetSelection({
    facetKeys: facetOptions.map((facet) => facet.value),
    requestedSort: s.tasksListSort,
    requestedGroup: s.tasksListGroup,
    tasks: s.tasks,
    facetValues,
    onSortChange: handleSortChange,
    onGroupChange: handleGroupChange,
  });
  const stepMetadata = useTasksListStepRefresh(s, displayedTasks, group);

  return {
    displayedTasks,
    facetOptions,
    facetValues,
    activeSteps,
    sort,
    group,
    selectSort,
    selectGroup,
    stepMetadata,
  };
}

export function TasksPageClient(props: TasksPageClientProps) {
  const s = useTasksPageSetup(props);
  const store = useAppStoreApi();
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const { setView } = useTaskListingView();
  const setMobileSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);
  const isMobileSearchOpen = useAppStore((state) => state.mobileKanban.isSearchOpen);
  const { isMobile } = useResponsiveBreakpoint();
  const showTaskDetails = useAppStore((state) => state.userSettings.tasksListShowDetails ?? false);
  const {
    displayedTasks,
    facetOptions,
    facetValues,
    activeSteps,
    sort,
    group,
    selectSort,
    selectGroup,
    stepMetadata,
  } = useTasksListPresentation(s);
  useWorkspacePRs(s.activeWorkspaceId);
  useWorkspaceMRs(s.activeWorkspaceId);

  useTasksPageClientEffects({ setMobileSearchOpen, setView });

  return (
    <TasksPageContent
      filters={{
        searchQuery: s.searchQuery,
        onSearchChange: s.setSearchQuery,
        workflowId: s.workflowId,
        onWorkflowChange: s.setWorkflowId,
        repositoryId: s.repositoryId,
        onRepositoryChange: s.setRepositoryId,
        workflows: stepMetadata.workflows,
        repositories: s.repositories,
        onClearFilters: s.clearFilters,
      }}
      readError={Boolean(s.error || s.metadataError)}
      unavailableFilter={s.unavailableFilter}
      onRetry={() => {
        if (s.metadataError) store.getState().requestWorkspaceContextRefresh();
        else void s.fetchTasks();
      }}
      header={{
        workspaceId: s.activeWorkspaceId ?? undefined,
        currentPage: "tasks",
        tasksListOptions: {
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
      workflows={stepMetadata.workflows}
      repositories={s.repositories}
      facetOptions={facetOptions}
      facetValues={facetValues}
      workflowStepPreviews={stepMetadata.previews}
      total={s.total}
      pageCount={s.pageCount}
      pagination={s.pagination}
      setPagination={s.setPagination}
      isLoading={s.isLoading}
      archiveMode={s.archiveMode}
      showTaskDetails={showTaskDetails}
      sort={sort}
      group={group}
      onSortChange={selectSort}
      onGroupChange={selectGroup}
      onArchiveModeChange={s.setArchiveMode}
      onRowClick={s.handleRowClick}
      deletingTaskId={s.deletingTaskId}
      onArchive={s.handleArchive}
      onUnarchive={s.handleUnarchive}
      onDelete={s.handleDelete}
      onRefresh={stepMetadata.refresh}
      mobileActions={
        isMobile && s.activeWorkspaceId ? (
          <MobileTasksActions
            workspaceId={s.activeWorkspaceId}
            workflowId={s.activeWorkflowId}
            steps={activeSteps}
            open={isCreateOpen}
            onOpenChange={setIsCreateOpen}
            onCreated={() => s.fetchTasks(true)}
          />
        ) : null
      }
    />
  );
}
