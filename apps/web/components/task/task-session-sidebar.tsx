"use client";

import { useCallback, useMemo, useState, memo } from "react";
import { usePathname, useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
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
import { useSidebarSelection, SidebarBulkDialogs } from "./task-session-sidebar-selection";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { repositorySlug } from "@/lib/repository-slug";
import { buildSwitchToSession, selectTaskWithLayout } from "./task-select-helpers";
import { useArchivedTaskState } from "./task-archived-context";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useGroupedSidebarView } from "./task-session-sidebar-grouped-view";
import { useSidebarLinkActions } from "./task-session-sidebar-link-actions";
import { buildArchivedSidebarItem } from "./task-session-sidebar-archived-item";
import { useSidebarTaskLinking } from "./task-session-sidebar-task-linking";
import { buildSidebarItem } from "./task-session-sidebar-item";
import { useSidebarTaskEdit } from "./task-session-sidebar-edit";

type TaskSessionSidebarProps = {
  workspaceId: string | null;
  workflowId: string | null;
  /** Hide the embedded filter bar when the host surface (e.g. AppSidebar) renders its own. */
  hideFilterBar?: boolean;
};

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
    workflows,
    isLoading: isLoadingWorkflow,
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
    acknowledgedAgentErrors,
    dismissedAgentErrors,
  ]);

  return {
    activeTaskId,
    selectedTaskId,
    allSteps,
    stepsByWorkflowId,
    isLoadingWorkflow,
    tasksWithRepositories,
    workflows,
  };
}

type StoreApi = ReturnType<typeof useAppStoreApi>;

function useMoveToStep(store: StoreApi) {
  const { moveTaskById } = useTaskActions();

  return useCallback(
    async (taskId: string, workflowId: string, targetStepId: string) => {
      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const originalTask = snapshot.tasks.find((t) => t.id === taskId);
      if (!originalTask) return;

      const targetTasks = snapshot.tasks
        .filter((t) => t.workflowStepId === targetStepId && t.id !== taskId)
        .sort((a, b) => a.position - b.position);
      const nextPosition = targetTasks.length;

      // Optimistic update
      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t) =>
          t.id === taskId ? { ...t, workflowStepId: targetStepId, position: nextPosition } : t,
        ),
      });

      try {
        await moveTaskById(taskId, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
      } catch (error) {
        // Rollback only the moved task, and only if it still has the optimistic values
        const cur = store.getState().kanbanMulti.snapshots[workflowId];
        const curTask = cur?.tasks.find((t) => t.id === taskId);
        if (cur && curTask?.workflowStepId === targetStepId && curTask.position === nextPosition) {
          store.getState().setWorkflowSnapshot(workflowId, {
            ...cur,
            tasks: cur.tasks.map((t) =>
              t.id === taskId
                ? {
                    ...t,
                    workflowStepId: originalTask.workflowStepId,
                    position: originalTask.position,
                  }
                : t,
            ),
          });
        }
        console.error("Failed to move task:", error);
      }
    },
    [store, moveTaskById],
  );
}

function useArchiveActions(store: StoreApi) {
  const archiveAndSwitch = useArchiveAndSwitchTask({ useLayoutSwitch: true });
  const [archivingTask, setArchivingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [archivingTaskId, setArchivingTaskId] = useState<string | null>(null);
  const [isArchiving, setIsArchiving] = useState(false);

  const handleArchiveTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      setArchivingTask({
        id: taskId,
        title: task?.title ?? "this task",
        executorType: task?.primaryExecutorType,
      });
    },
    [store],
  );

  const handleArchiveConfirm = useCallback(
    async (opts: { cascade: boolean }) => {
      if (!archivingTask) return;
      const taskId = archivingTask.id;
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
    [archivingTask, archiveAndSwitch],
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
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      setDeletingTask({
        id: taskId,
        title: task?.title ?? "this task",
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

  const switchToSession = useMemo(
    () => buildSwitchToSession(store, setActiveSession),
    [store, setActiveSession],
  );

  const handleSelectTask = useCallback(
    (taskId: string) => {
      // The AppSidebar is mounted globally. On a non-task route the dockview
      // isn't mounted, so the in-place layout switch (which only rewrites the
      // URL via history.replaceState) would change the address bar without
      // ever showing the task. Navigate to the task page in that case; the
      // in-place fast-switch is only correct once the dockview is on screen.
      const onTaskRoute =
        !!pathname && (pathname.startsWith("/t/") || pathname.startsWith("/office/tasks/"));
      if (!onTaskRoute) {
        setActiveTask(taskId);
        router.push(linkToTask(taskId));
        return;
      }
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      selectTaskWithLayout({
        taskId,
        task: task ?? undefined,
        store,
        switchToSession,
        loadTaskSessionsForTask,
        setActiveTask,
        setPreparingTaskId,
      });
    },
    [loadTaskSessionsForTask, switchToSession, setActiveTask, store, router, pathname],
  );

  const archiveActions = useArchiveActions(store);
  const deleteActions = useDeleteActions(store, removeTaskFromBoard);
  const detachActions = useTaskDetachDialog(store);
  const linkActions = useSidebarLinkActions(store);
  const editActions = useSidebarTaskEdit();

  const [renamingTask, setRenamingTask] = useState<{ id: string; title: string } | null>(null);
  const [creatingSubtask, setCreatingSubtask] = useState<{ id: string; title: string } | null>(
    null,
  );

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

  const handleMoveToStep = useMoveToStep(store);

  return {
    preparingTaskId,
    handleSelectTask,
    handleMoveToStep,
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
  useRepositories(workspaceId);
  const pathname = usePathname();

  const {
    activeTaskId,
    selectedTaskId,
    stepsByWorkflowId,
    workflows,
    isLoadingWorkflow,
    tasksWithRepositories,
  } = useSidebarData(workspaceId);

  // Only highlight while viewing a task route; AppSidebar is global and activeTaskId lingers.
  const onTaskRoute =
    !!pathname && (pathname.startsWith("/t/") || pathname.startsWith("/office/tasks/"));
  const highlightedTaskId = onTaskRoute ? activeTaskId : null;
  const highlightedSelectedTaskId = onTaskRoute ? selectedTaskId : null;

  const sidebarActions = useSidebarActions(store);
  const { preparingTaskId } = sidebarActions;
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
    isLoadingWorkflow,
    totalTaskCount: displayTasks.length,
    selection,
  });
  return (
    <PanelRoot data-testid="task-sidebar">
      {!hideFilterBar && <SidebarFilterBar />}
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
