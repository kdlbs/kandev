"use client";

import { useCallback, useEffect, useMemo, useRef, useState, memo } from "react";
import { useTranslation } from "react-i18next";
import { usePathname, useRouter } from "@/lib/routing/client-router";
import { linkToTask, replaceTaskUrl } from "@/lib/links";
import type { Repository, TaskSessionState } from "@/lib/types/http";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { TaskSwitcher, type TaskSwitcherItem } from "./task-switcher";
import { buildTaskSwitcherProps } from "./task-session-sidebar-switcher-props";
import { SidebarFilterBar } from "./sidebar-filter/sidebar-filter-bar";
import { MOCK_ITEMS, MOCK_SIDEBAR } from "./sidebar-mock-data";
import { SidebarDialogs } from "./task-session-sidebar-dialogs";
import { PanelRoot } from "./panel-primitives";
import { TaskSidebarScrollArea } from "./task-sidebar-scroll-area";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useWorkspaceSidebarTasks } from "@/hooks/domains/kanban/use-workspace-sidebar-tasks";
import { useTaskActions, useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskDetachDialog } from "@/hooks/use-detach-task";
import { useNestTaskByDrag } from "@/hooks/use-nest-task";
import { useSidebarSelection, SidebarBulkDialogs } from "./task-session-sidebar-selection";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { repositorySlug } from "@/lib/repository-slug";
import {
  buildSwitchToSession,
  effectiveTaskPendingAction,
  selectTaskWithLayout,
} from "./task-select-helpers";
import { useArchivedTaskState } from "./task-archived-context";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useGroupedSidebarView } from "./task-session-sidebar-grouped-view";
import { useSidebarLinkActions } from "./task-session-sidebar-link-actions";
import { buildArchivedSidebarItem } from "./task-session-sidebar-archived-item";
import { useSidebarTaskLinking } from "./task-session-sidebar-task-linking";
import { buildSidebarItem } from "./task-session-sidebar-item";
import { useSidebarTaskEdit } from "./task-session-sidebar-edit";
import { TaskMoveErrorBanner } from "./task-move-error-banner";
import { useMoveToStep } from "./task-session-sidebar-move";

type TaskSessionSidebarProps = {
  workspaceId: string | null;
  workflowId: string | null;
  /** Hide the embedded filter bar when the host surface (e.g. AppSidebar) renders its own. */
  hideFilterBar?: boolean;
};

function findSidebarTask(state: ReturnType<StoreApi["getState"]>, taskId: string) {
  const activeTask = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
  if (activeTask) return activeTask;
  for (const tasks of Object.values(state.sidebarArchivedTasks?.itemsByWorkspaceId ?? {})) {
    const archivedTask = tasks.find((task) => task.id === taskId);
    if (archivedTask) return archivedTask;
  }
  return undefined;
}

function useSidebarData(workspaceId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const acknowledgedAgentErrors = useAppStore((state) => state.acknowledgedAgentErrors);
  const dismissedAgentErrors = useAppStore((state) => state.dismissedAgentErrors);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const archivedState = useArchivedTaskState();

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);

  const {
    allTasks,
    allSteps,
    stepsByWorkflowId,
    wipQueueByTaskId,
    workflows,
    isLoading: isLoadingWorkflow,
    archivedError,
    retryArchivedTasks,
  } = useWorkspaceSidebarTasks(workspaceId);

  const tasksWithRepositories = useMemo(() => {
    const repositories = workspaceId ? (repositoriesByWorkspace[workspaceId] ?? []) : [];
    const repositorySlugById = new Map(
      repositories.map((repo: Repository) => [repo.id, repositorySlug(repo)]),
    );
    const titleById = new Map(allTasks.map((t) => [t.id, t.title]));
    const workflowNameById = new Map(workflows.map((w) => [w.id, w.name]));
    const stepTitleById = new Map(allSteps.map((s) => [s.id, s.title]));
    const mapCtx = {
      repositorySlugById,
      titleById,
      workflowNameById,
      stepTitleById,
      wipQueueByTaskId,
      acknowledgedAgentErrors,
      dismissedAgentErrors,
    };
    const items: TaskSwitcherItem[] = allTasks.map((task) => buildSidebarItem(task, mapCtx));
    if (
      archivedState.isArchived &&
      archivedState.archivedTaskId &&
      !items.some((t) => t.id === archivedState.archivedTaskId)
    ) {
      items.unshift(buildArchivedSidebarItem(archivedState));
    }
    return items;
  }, [
    repositoriesByWorkspace,
    allTasks,
    allSteps,
    workflows,
    workspaceId,
    archivedState,
    wipQueueByTaskId,
    acknowledgedAgentErrors,
    dismissedAgentErrors,
  ]);

  return {
    activeTaskId,
    selectedTaskId,
    allSteps,
    stepsByWorkflowId,
    isLoadingWorkflow,
    archivedError,
    retryArchivedTasks,
    tasksWithRepositories,
    workflows,
  };
}

type StoreApi = ReturnType<typeof useAppStoreApi>;

function useArchiveActions(store: StoreApi) {
  const { t } = useTranslation();
  const archiveAndSwitch = useArchiveAndSwitchTask({ useLayoutSwitch: true });
  const [archivingTask, setArchivingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [archivingTaskId, setArchivingTaskId] = useState<string | null>(null);
  const [isArchiving, setIsArchiving] = useState(false);

  const runArchive = useCallback(
    async (taskId: string, opts: { cascade?: boolean }) => {
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
    (taskId: string, opts?: { cascade?: boolean }) => {
      if (opts) {
        void runArchive(taskId, opts);
        return;
      }
      const state = store.getState();
      const task = findSidebarTask(state, taskId);
      setArchivingTask({
        id: taskId,
        title: task?.title ?? t("task:thisTask"),
        executorType: task?.primaryExecutorType,
      });
    },
    [runArchive, store, t],
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

function useDeleteActions(
  store: StoreApi,
  removeTaskFromBoard: ReturnType<typeof useTaskRemoval>["removeTaskFromBoard"],
) {
  const { t } = useTranslation();
  const { deleteTaskById } = useTaskActions();
  const [deletingTask, setDeletingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDeleteTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findSidebarTask(state, taskId);
      setDeletingTask({
        id: taskId,
        title: task?.title ?? t("task:thisTask"),
        executorType: task?.primaryExecutorType,
      });
    },
    [store],
  );

  const handleDeleteConfirm = useCallback(
    async (opts: { cascade: boolean }) => {
      if (!deletingTask || isDeleting) return;
      const taskId = deletingTask.id;
      setIsDeleting(true);
      const { activeTaskId: wasActiveTaskId, activeSessionId: wasActiveSessionId } =
        store.getState().tasks;
      try {
        await deleteTaskById(taskId, opts);
        await removeTaskFromBoard(taskId, { wasActiveTaskId, wasActiveSessionId });
      } catch (error) {
        console.error("Failed to delete task:", error);
      } finally {
        setIsDeleting(false);
        setDeletingTask(null);
      }
    },
    [deletingTask, isDeleting, deleteTaskById, removeTaskFromBoard, store],
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
  pathname: string | null;
  router: ReturnType<typeof useRouter>;
  loadTaskSessionsForTask: ReturnType<typeof useTaskRemoval>["loadTaskSessionsForTask"];
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
  setPreparingTaskId: (taskId: string | null) => void;
}) {
  const {
    store,
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
      const task = findSidebarTask(state, taskId);
      const onTaskRoute =
        !!pathname && (pathname.startsWith("/t/") || pathname.startsWith("/office/tasks/"));
      if (!onTaskRoute && (!effectiveTaskPendingAction(task) || task?.isArchived)) {
        setActiveTask(taskId);
        router.push(linkToTask(taskId));
        return;
      }
      if (task?.isArchived) {
        setActiveTask(taskId);
        replaceTaskUrl(taskId);
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
          ? replaceTaskUrl
          : (selectedTaskId) => router.push(linkToTask(selectedTaskId)),
        selectionSignal: selectionControllerRef.current?.signal,
      });
    },
    [
      loadTaskSessionsForTask,
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

export function useSidebarActions(store: StoreApi) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const [preparingTaskId, setPreparingTaskId] = useState<string | null>(null);
  const { renameTaskById } = useTaskActions();
  const router = useRouter();
  const pathname = usePathname();
  const { removeTaskFromBoard, loadTaskSessionsForTask } = useTaskRemoval({
    store,
    useLayoutSwitch: true,
  });

  const handleSelectTask = useSidebarTaskSelection({
    store,
    pathname,
    router,
    loadTaskSessionsForTask,
    setActiveSession,
    setActiveTask,
    setPreparingTaskId,
  });

  const archiveActions = useArchiveActions(store);
  const deleteActions = useDeleteActions(store, removeTaskFromBoard);
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
    preparingTaskId,
    taskMoveError,
    handleSelectTask,
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

export const TaskSessionSidebar = memo(function TaskSessionSidebar({
  workspaceId,
  hideFilterBar,
}: TaskSessionSidebarProps) {
  const store = useAppStoreApi();
  const { t } = useTranslation();
  useRepositories(workspaceId);
  useWorkspaceMRs(workspaceId);
  const pathname = usePathname();

  const {
    activeTaskId,
    selectedTaskId,
    stepsByWorkflowId,
    workflows,
    isLoadingWorkflow,
    archivedError,
    retryArchivedTasks,
    tasksWithRepositories,
  } = useSidebarData(workspaceId);

  // Only highlight while viewing a task route; AppSidebar is global and activeTaskId lingers.
  const onTaskRoute =
    !!pathname && (pathname.startsWith("/t/") || pathname.startsWith("/office/tasks/"));
  const highlightedTaskId = onTaskRoute ? activeTaskId : null;
  const highlightedSelectedTaskId = onTaskRoute ? selectedTaskId : null;

  const sidebarActions = useSidebarActions(store);
  const { preparingTaskId, taskMoveError } = sidebarActions;
  const taskLinkHandlers = useSidebarTaskLinking(workspaceId, sidebarActions);
  const repositories =
    useAppStore((state) =>
      workspaceId ? state.repositories.itemsByWorkspaceId[workspaceId] : undefined,
    ) ?? [];

  const displayTasks = useMemo(() => {
    if (MOCK_SIDEBAR) return MOCK_ITEMS;
    return preparingTaskId
      ? tasksWithRepositories.map((t) =>
          t.id === preparingTaskId ? { ...t, sessionState: "STARTING" as TaskSessionState } : t,
        )
      : tasksWithRepositories;
  }, [tasksWithRepositories, preparingTaskId]);

  const toggleSidebarGroupCollapsed = useAppStore((state) => state.toggleSidebarGroupCollapsed);
  const collapsedSubtaskParents = useAppStore((state) => state.collapsedSubtaskParents);
  const toggleSubtaskCollapsed = useAppStore((state) => state.toggleSubtaskCollapsed);
  const { grouped, effectiveView, prefs } = useGroupedSidebarView(displayTasks);
  const { pinnedTaskIds, togglePinnedTask, handleReorderGroup, handleReorderSubtasks } = prefs;
  const selection = useSidebarSelection({
    workspaceId,
    grouped,
    collapsedGroups: effectiveView.collapsedGroups,
    collapsedSubtaskParents,
    displayTasks,
  });
  const handleToggleGroup = useCallback(
    (groupKey: string) => toggleSidebarGroupCollapsed(effectiveView.id, groupKey),
    [toggleSidebarGroupCollapsed, effectiveView.id],
  );
  const switcherProps = buildTaskSwitcherProps({
    grouped,
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
    archivedError,
    retryArchivedTasks,
    archivedLoadErrorLabel: t("sidebar:archivedLoadFailed"),
    archivedRetryLabel: t("sidebar:retry"),
    totalTaskCount: displayTasks.length,
    selection,
  });
  return (
    <PanelRoot data-testid="task-sidebar">
      {!hideFilterBar && <SidebarFilterBar />}
      {taskMoveError !== null && <TaskMoveErrorBanner error={taskMoveError} />}
      <TaskSidebarScrollArea>
        <TaskSwitcher {...switcherProps} />
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
