"use client";

import { useCallback, useEffect, useMemo, useRef, useState, memo } from "react";
import { useTranslation } from "react-i18next";
import { usePathname, useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
import type { Repository, SidebarTaskPageResponse, TaskSessionState } from "@/lib/types/http";
import type { AggregatedSidebarTasks } from "./task-session-sidebar-aggregate";
import type { SidebarItemContext } from "./task-session-sidebar-item";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { buildTaskSwitcherProps } from "./task-session-sidebar-switcher-props";
import { SidebarFilterBar } from "./sidebar-filter/sidebar-filter-bar";
import { MOCK_ITEMS, MOCK_SIDEBAR } from "./sidebar-mock-data";
import { SidebarDialogs } from "./task-session-sidebar-dialogs";
import { PanelRoot } from "./panel-primitives";
import { TaskSidebarScrollArea } from "./task-sidebar-scroll-area";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useWorkspaceSidebarTasks } from "@/hooks/domains/kanban/use-workspace-sidebar-tasks";
import { useTaskActions, type TaskActionOptions } from "@/hooks/use-task-actions";
import { useTaskMenuActions } from "@/hooks/use-task-menu-actions";
import { useTaskDetachDialog } from "@/hooks/use-detach-task";
import { useNestTaskByDrag } from "@/hooks/use-nest-task";
import { useSidebarSelection, SidebarBulkDialogs } from "./task-session-sidebar-selection";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { repositorySlug } from "@/lib/repository-slug";
import {
  buildSwitchToSession,
  effectiveTaskPendingAction,
  selectTaskWithLayout,
} from "./task-select-helpers";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { groupSidebarTaskPage } from "./task-session-sidebar-grouped-view";
import { useSidebarLinkActions } from "./task-session-sidebar-link-actions";
import { useSidebarTaskLinking } from "./task-session-sidebar-task-linking";
import { buildSidebarItem } from "./task-session-sidebar-item";
import { useSidebarTaskEdit } from "./task-session-sidebar-edit";
import { TaskMoveErrorBanner } from "./task-move-error-banner";
import { useMoveToStep } from "./task-session-sidebar-move";
import { SidebarTaskPageContent } from "./sidebar-task-page-content";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { applyView } from "@/lib/sidebar/apply-view";
import type { WipQueueStatus } from "@/lib/kanban/wip-queue";
import { applySidebarPageMetadata } from "./sidebar-page-metadata";
import { findSidebarTask } from "./task-session-sidebar-task-lookup";

type TaskSessionSidebarProps = {
  workspaceId: string | null;
  workflowId: string | null;
  /** Hide the embedded filter bar when the host surface (e.g. AppSidebar) renders its own. */
  hideFilterBar?: boolean;
};

const EMPTY_SIDEBAR_TASKS: AggregatedSidebarTasks["allTasks"] = [];

function buildSidebarTaskItems(params: {
  workspaceId: string | null;
  repositoriesByWorkspace: Record<string, Repository[]>;
  allTasks: AggregatedSidebarTasks["allTasks"];
  allSteps: AggregatedSidebarTasks["allSteps"];
  pageEntries: SidebarTaskPageResponse["entries"] | undefined;
  workflows: Array<{ id: string; name: string }>;
  wipQueueByTaskId: Map<string, WipQueueStatus>;
  acknowledgedAgentErrors: Record<string, string>;
  dismissedAgentErrors: Record<string, string>;
  automaticColorSettings: SidebarItemContext["automaticColorSettings"];
  manualColors: SidebarItemContext["manualColors"];
  pendingRemovalTaskIds: ReadonlySet<string>;
}): ReturnType<typeof buildSidebarItem>[] {
  const {
    workspaceId,
    repositoriesByWorkspace,
    allTasks,
    allSteps,
    pageEntries,
    workflows,
    wipQueueByTaskId,
    acknowledgedAgentErrors,
    dismissedAgentErrors,
    automaticColorSettings,
    manualColors,
    pendingRemovalTaskIds,
  } = params;
  const repositories = workspaceId ? (repositoriesByWorkspace[workspaceId] ?? []) : [];
  const repositorySlugById = new Map(repositories.map((repo) => [repo.id, repositorySlug(repo)]));
  const repositoriesById = new Map(
    Object.values(repositoriesByWorkspace)
      .flat()
      .map((repo) => [repo.id, repo]),
  );
  const stepColorById = new Map(allSteps.map((step) => [step.id, step.color]));
  const titleById = new Map(allTasks.map((task) => [task.id, task.title]));
  const workflowNameById = new Map(workflows.map((workflow) => [workflow.id, workflow.name]));
  const stepTitleById = new Map(allSteps.map((step) => [step.id, step.title]));
  applySidebarPageMetadata(pageEntries ?? [], {
    titleById,
    workflowNameById,
    stepTitleById,
    stepColorById,
  });
  const context: SidebarItemContext = {
    repositorySlugById,
    titleById,
    workflowNameById,
    stepTitleById,
    wipQueueByTaskId,
    acknowledgedAgentErrors,
    dismissedAgentErrors,
    workspaceId: workspaceId ?? undefined,
    repositoriesById,
    stepColorById,
    automaticColorSettings,
    manualColors,
    pendingRemovalTaskIds,
  };
  return allTasks.map((task) => buildSidebarItem(task, context));
}

export function useSidebarData(workspaceId: string | null, activeTaskOnly = false) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const acknowledgedAgentErrors = useAppStore((state) => state.acknowledgedAgentErrors);
  const dismissedAgentErrors = useAppStore((state) => state.dismissedAgentErrors);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const automaticColorSettings = useAppStore(
    (state) => state.userSettings.sidebarTaskColorAutomation,
  );
  const manualColors = useAppStore((state) => state.userSettings.sidebarTaskColors);

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);

  const {
    allTasks,
    pendingRemovalTaskIds,
    allSteps,
    stepsByWorkflowId,
    page,
    pageEntries,
    wipQueueByTaskId,
    workflows,
    isLoading: isLoadingWorkflow,
    archivedError,
    retryArchivedTasks,
    workspaceContextError,
    workspaceContextPending,
    workspaceContextAccessDenied,
    retryWorkspaceContext,
  } = useWorkspaceSidebarTasks(workspaceId, activeTaskOnly);

  const tasksWithRepositories = useMemo(
    () =>
      buildSidebarTaskItems({
        workspaceId,
        repositoriesByWorkspace,
        allTasks,
        allSteps,
        pageEntries,
        workflows,
        wipQueueByTaskId,
        acknowledgedAgentErrors,
        dismissedAgentErrors,
        automaticColorSettings,
        manualColors,
        pendingRemovalTaskIds,
      }),
    [
      repositoriesByWorkspace,
      allTasks,
      allSteps,
      pageEntries,
      workflows,
      workspaceId,
      wipQueueByTaskId,
      acknowledgedAgentErrors,
      dismissedAgentErrors,
      automaticColorSettings,
      manualColors,
      pendingRemovalTaskIds,
    ],
  );

  return {
    activeTaskId,
    selectedTaskId,
    allSteps,
    stepsByWorkflowId,
    isLoadingWorkflow,
    archivedError,
    retryArchivedTasks,
    page,
    pageEntries,
    workspaceContextError,
    workspaceContextPending,
    workspaceContextAccessDenied,
    retryWorkspaceContext,
    allTasks,
    tasksWithRepositories,
    workflows,
  };
}

type StoreApi = ReturnType<typeof useAppStoreApi>;

function useArchiveActions(store: StoreApi, pageTasks: AggregatedSidebarTasks["allTasks"]) {
  const { t } = useTranslation();
  const { runArchive: archiveAndSwitch } = useTaskMenuActions({ useLayoutSwitch: true });
  const [archivingTask, setArchivingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [archivingTaskId, setArchivingTaskId] = useState<string | null>(null);
  const [isArchiving, setIsArchiving] = useState(false);

  const runArchive = useCallback(
    async (taskId: string, opts: TaskActionOptions) => {
      setIsArchiving(true);
      setArchivingTaskId(taskId);
      try {
        await archiveAndSwitch(taskId, opts);
      } catch (error) {
        console.error("Failed to archive task:", error);
      } finally {
        setIsArchiving(false);
        setArchivingTaskId((current) => (current === taskId ? null : current));
        setArchivingTask((current) => (current?.id === taskId ? null : current));
      }
    },
    [archiveAndSwitch],
  );

  const handleArchiveTask = useCallback(
    (taskId: string, opts?: TaskActionOptions) => {
      if (opts) {
        void runArchive(taskId, opts);
        return;
      }
      const state = store.getState();
      const task = findSidebarTask(state, taskId, pageTasks);
      setArchivingTask({
        id: taskId,
        title: task?.title ?? t("task:thisTask"),
        executorType: task?.primaryExecutorType,
      });
    },
    [pageTasks, runArchive, store, t],
  );

  const handleArchiveConfirm = useCallback(
    async (opts: { cascade: boolean }) => {
      if (!archivingTask) return;
      await runArchive(archivingTask.id, opts);
    },
    [archivingTask, runArchive],
  );

  return {
    archivingTask,
    setArchivingTask,
    archivingTaskId,
    isArchiving,
    handleArchiveTask,
    handleArchiveConfirm,
  };
}

function useDeleteActions(store: StoreApi, pageTasks: AggregatedSidebarTasks["allTasks"]) {
  const { t } = useTranslation();
  const { runDelete } = useTaskMenuActions({ useLayoutSwitch: true });
  const [deletingTask, setDeletingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDeleteTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findSidebarTask(state, taskId, pageTasks);
      setDeletingTask({
        id: taskId,
        title: task?.title ?? t("task:thisTask"),
        executorType: task?.primaryExecutorType,
      });
    },
    [pageTasks, store, t],
  );

  const handleDeleteConfirm = useCallback(
    async (opts: TaskActionOptions & { cascade: boolean; discardWorktreeChanges: boolean }) => {
      if (!deletingTask || isDeleting) return;
      const taskId = deletingTask.id;
      setIsDeleting(true);
      try {
        await runDelete(taskId, opts);
      } catch (error) {
        console.error("Failed to delete task:", error);
      } finally {
        setIsDeleting(false);
        setDeletingTask(null);
      }
    },
    [deletingTask, isDeleting, runDelete],
  );

  const deletingTaskId = isDeleting ? (deletingTask?.id ?? null) : null;

  return {
    deletingTask,
    setDeletingTask,
    deletingTaskId,
    isDeleting,
    handleDeleteTask,
    handleDeleteConfirm,
  };
}

function useSidebarTaskSelection(params: {
  store: StoreApi;
  pageTasks: AggregatedSidebarTasks["allTasks"];
  pathname: string | null;
  router: ReturnType<typeof useRouter>;
  loadTaskSessionsForTask: ReturnType<typeof useTaskRemoval>["loadTaskSessionsForTask"];
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
  setPreparingTaskId: (taskId: string | null) => void;
}) {
  const {
    store,
    pageTasks,
    pathname,
    router,
    loadTaskSessionsForTask,
    setActiveSession,
    setActiveTask,
    setPreparingTaskId,
  } = params;
  const switchToSession = useMemo(
    () => buildSwitchToSession(store, setActiveSession),
    [store, setActiveSession],
  );
  const selectionControllerRef = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    selectionControllerRef.current = controller;
    return () => {
      controller.abort();
      if (selectionControllerRef.current === controller) selectionControllerRef.current = null;
    };
  }, [pathname]);
  return useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findSidebarTask(state, taskId, pageTasks);
      const onTaskRoute =
        !!pathname && (pathname.startsWith("/t/") || pathname.startsWith("/office/tasks/"));
      if (!onTaskRoute && !task?.isArchived && !effectiveTaskPendingAction(task)) {
        setActiveTask(taskId);
        router.push(linkToTask(taskId));
        return;
      }
      selectTaskWithLayout({
        taskId,
        task: task ?? undefined,
        store,
        switchToSession: onTaskRoute
          ? switchToSession
          : (selectedTaskId, sessionId) => setActiveSession(selectedTaskId, sessionId),
        loadTaskSessionsForTask,
        setActiveTask,
        setPreparingTaskId,
        navigateToTask: onTaskRoute
          ? (selectedTaskId, sessionId) => router.replace(linkToTask(selectedTaskId, { sessionId }))
          : (selectedTaskId, sessionId) => router.push(linkToTask(selectedTaskId, { sessionId })),
        selectionSignal: selectionControllerRef.current?.signal,
      });
    },
    [
      loadTaskSessionsForTask,
      pageTasks,
      pathname,
      router,
      setActiveSession,
      setActiveTask,
      setPreparingTaskId,
      store,
      switchToSession,
    ],
  );
}

export function useSidebarActions(store: StoreApi, pageTasks: AggregatedSidebarTasks["allTasks"]) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const [preparingTaskId, setPreparingTaskId] = useState<string | null>(null);
  const router = useRouter();
  const pathname = usePathname();
  const { loadTaskSessionsForTask } = useTaskRemoval({
    store,
    useLayoutSwitch: true,
  });

  const handleSelectTask = useSidebarTaskSelection({
    store,
    pageTasks,
    pathname,
    router,
    loadTaskSessionsForTask,
    setActiveSession,
    setActiveTask,
    setPreparingTaskId,
  });

  return { preparingTaskId, handleSelectTask, ...useTaskRowActions(store, pageTasks) };
}

export function useTaskRowActions(
  store: StoreApi,
  pageTasks: AggregatedSidebarTasks["allTasks"] = EMPTY_SIDEBAR_TASKS,
) {
  const { renameTaskById } = useTaskActions();
  const archiveActions = useArchiveActions(store, pageTasks);
  const deleteActions = useDeleteActions(store, pageTasks);
  const detachActions = useTaskDetachDialog(store);
  const handleNestTask = useNestTaskByDrag();
  const linkActions = useSidebarLinkActions(store);
  const editActions = useSidebarTaskEdit();

  const [renamingTask, setRenamingTask] = useState<{ id: string; title: string } | null>(null);
  const [creatingSubtask, setCreatingSubtask] = useState<{ id: string; title: string } | null>(
    null,
  );
  const [taskMoveError, setTaskMoveError] = useState<unknown>(null);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  useEffect(() => {
    setTaskMoveError(null);
  }, [activeTaskId]);

  const handleRenameTask = useCallback((taskId: string, currentTitle: string) => {
    setRenamingTask({ id: taskId, title: currentTitle });
  }, []);

  const handleCreateSubtask = useCallback((taskId: string, taskTitle: string) => {
    setCreatingSubtask({ id: taskId, title: taskTitle });
  }, []);

  const handleRenameSubmit = useCallback(
    async (newTitle: string) => {
      if (!renamingTask) return;
      try {
        await renameTaskById(renamingTask.id, newTitle);
      } catch (error) {
        console.error("Failed to rename task:", error);
      }
      setRenamingTask(null);
    },
    [renamingTask, renameTaskById],
  );

  const clearTaskMoveError = useCallback(() => setTaskMoveError(null), []);
  const reportTaskMoveError = useCallback((error: unknown) => setTaskMoveError(error), []);
  const handleMoveToStep = useMoveToStep(store, clearTaskMoveError, reportTaskMoveError);

  return {
    taskMoveError,
    handleMoveToStep,
    handleNestTask,
    renamingTask,
    setRenamingTask,
    handleRenameTask,
    handleRenameSubmit,
    creatingSubtask,
    setCreatingSubtask,
    handleCreateSubtask,
    ...linkActions,
    ...archiveActions,
    ...deleteActions,
    ...detachActions,
    ...editActions,
  };
}

// eslint-disable-next-line max-lines-per-function -- desktop sidebar wiring must preserve one shared task surface
export const TaskSessionSidebar = memo(function TaskSessionSidebar({
  workspaceId,
  hideFilterBar,
}: TaskSessionSidebarProps) {
  const store = useAppStoreApi();
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  useRepositories(workspaceId);
  useWorkspaceMRs(workspaceId);
  const pathname = usePathname();

  const {
    activeTaskId,
    selectedTaskId,
    stepsByWorkflowId,
    workflows,
    isLoadingWorkflow,
    retryArchivedTasks,
    workspaceContextError,
    workspaceContextPending,
    workspaceContextAccessDenied,
    retryWorkspaceContext,
    tasksWithRepositories,
    allTasks,
    page,
    pageEntries,
  } = useSidebarData(workspaceId, isMobile);

  // Only highlight while viewing a task route; AppSidebar is global and activeTaskId lingers.
  const onTaskRoute =
    !!pathname && (pathname.startsWith("/t/") || pathname.startsWith("/office/tasks/"));
  const highlightedTaskId = onTaskRoute ? activeTaskId : null;
  const highlightedSelectedTaskId = onTaskRoute ? selectedTaskId : null;

  const sidebarActions = useSidebarActions(store, allTasks);
  const { preparingTaskId, taskMoveError } = sidebarActions;
  const taskLinkHandlers = useSidebarTaskLinking(workspaceId, sidebarActions);
  const repositories =
    useAppStore((state) =>
      workspaceId ? state.repositories.itemsByWorkspaceId[workspaceId] : undefined,
    ) ?? [];

  const displayTasks = useMemo(() => {
    if (workspaceContextAccessDenied) return [];
    if (MOCK_SIDEBAR) return MOCK_ITEMS;
    return preparingTaskId
      ? tasksWithRepositories.map((t) =>
          t.id === preparingTaskId ? { ...t, sessionState: "STARTING" as TaskSessionState } : t,
        )
      : tasksWithRepositories;
  }, [tasksWithRepositories, preparingTaskId, workspaceContextAccessDenied]);

  const toggleSidebarGroupCollapsed = useAppStore((state) => state.toggleSidebarGroupCollapsed);
  const collapsedSubtaskParents = useAppStore((state) => state.collapsedSubtaskParents);
  const toggleSubtaskCollapsed = useAppStore((state) => state.toggleSubtaskCollapsed);
  const effectiveView = page.view;
  const prefs = useSidebarTaskPrefs();
  const grouped = useMemo(
    () =>
      MOCK_SIDEBAR || pageEntries === undefined
        ? applyView(displayTasks, effectiveView, {
            pinnedTaskIds: prefs.pinnedTaskIds,
            orderedTaskIds: prefs.orderedTaskIds,
            subtaskOrderByParentId: prefs.subtaskOrderByParentId,
          })
        : groupSidebarTaskPage(
            displayTasks,
            pageEntries,
            effectiveView.group,
            prefs.subtaskOrderByParentId,
          ),
    [
      displayTasks,
      effectiveView,
      pageEntries,
      prefs.orderedTaskIds,
      prefs.pinnedTaskIds,
      prefs.subtaskOrderByParentId,
    ],
  );
  const { pinnedTaskIds, togglePinnedTask, handleReorderGroup, handleReorderSubtasks } = prefs;
  const selection = useSidebarSelection({
    workspaceId,
    grouped,
    collapsedGroups: effectiveView.collapsedGroups,
    collapsedSubtaskParents,
    displayTasks,
    selectionScopeKey: page.scopeKey,
  });
  const handleToggleGroup = useCallback(
    (groupKey: string) => toggleSidebarGroupCollapsed(effectiveView.id, groupKey),
    [toggleSidebarGroupCollapsed, effectiveView.id],
  );
  const switcherProps = buildTaskSwitcherProps({
    grouped,
    nestHierarchyTasks: displayTasks,
    workflows,
    stepsByWorkflowId,
    highlightedTaskId,
    highlightedSelectedTaskId,
    effectiveView,
    handleToggleGroup,
    collapsedSubtaskParents,
    toggleSubtaskCollapsed,
    sidebarActions,
    taskLinkHandlers,
    pinnedTaskIds,
    togglePinnedTask,
    handleReorderGroup,
    handleReorderSubtasks,
    handleNestTask: sidebarActions.handleNestTask,
    isLoadingWorkflow,
    archivedError: null,
    retryArchivedTasks,
    archivedLoadErrorLabel: t("sidebar:archivedLoadFailed"),
    archivedRetryLabel: t("sidebar:retry"),
    workspaceContextError,
    workspaceContextPending,
    workspaceContextAccessDenied,
    workspaceContextLoadErrorLabel: t("sidebar:workspaceContextRefreshFailed"),
    workspaceContextAccessDeniedLabel: t("sidebar:workspaceContextAccessDenied"),
    retryWorkspaceContext,
    totalTaskCount: page.response?.total_tasks ?? displayTasks.length,
    selection,
  });
  const listScrollRef = useRef<HTMLDivElement>(null);
  return (
    <PanelRoot data-testid="task-sidebar">
      {!hideFilterBar && <SidebarFilterBar />}
      {taskMoveError !== null && <TaskMoveErrorBanner error={taskMoveError} />}
      <TaskSidebarScrollArea viewportRef={listScrollRef}>
        <SidebarTaskPageContent
          page={page}
          workspaceContextError={workspaceContextError}
          switcherProps={switcherProps}
          scrollRef={listScrollRef}
        />
        <PluginSlot name="task-sidebar" />
      </TaskSidebarScrollArea>
      <SidebarDialogs
        actions={sidebarActions}
        repositories={repositories}
        workspaceId={workspaceId}
        stepsByWorkflowId={stepsByWorkflowId}
      />
      <SidebarBulkDialogs selection={selection} />
    </PanelRoot>
  );
});
