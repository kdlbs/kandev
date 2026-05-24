"use client";

import { useCallback, useEffect, useMemo, useState, memo } from "react";
import type {
  Message,
  TaskState,
  TaskSession,
  TaskSessionState,
  Repository,
} from "@/lib/types/http";
import type { TaskPR } from "@/lib/types/github";
import type { KanbanState } from "@/lib/state/slices";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { TaskSwitcher, type TaskSwitcherItem } from "./task-switcher";
import { applyView } from "@/lib/sidebar/apply-view";
import { SidebarFilterBar } from "./sidebar-filter/sidebar-filter-bar";
import { MOCK_ITEMS, MOCK_SIDEBAR } from "./sidebar-mock-data";
import { SidebarDialogs } from "./task-session-sidebar-dialogs";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useWorkspaceSidebarTasks } from "@/hooks/domains/kanban/use-workspace-sidebar-tasks";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { useTaskActions, useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { buildSwitchToSession, selectTaskWithLayout } from "./task-select-helpers";
import { getSessionInfoForTask } from "@/lib/utils/session-info";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useArchivedTaskState } from "./task-archived-context";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useWorkspacePRs } from "@/hooks/domains/github/use-task-pr";
import {
  hasPendingClarificationForSession,
  hasPendingPermissionForSession,
} from "@/lib/utils/pending-clarification";

/**
 * Stabilize a derived array of primary session IDs so the reference only
 * changes when the actual contents change. This prevents the bulk-subscribe
 * effect from tearing down and recreating all subscriptions on every kanban
 * snapshot update.
 */
function useStablePrimarySessionIds(
  allTasks: Array<{ primarySessionId?: string | null }>,
): string[] {
  const key = useMemo(
    () =>
      allTasks
        .map((t) => t.primarySessionId)
        .filter((id): id is string => id != null)
        .join("\0"),
    [allTasks],
  );
  return useMemo(() => (key ? key.split("\0") : []), [key]);
}

/** Look up git status directly via primarySessionId, bypassing the session list. */
function getGitStatusForTask(
  task: { primarySessionId?: string | null },
  envIdBySessionId: Record<string, string>,
  gitStatusByEnvId: Record<string, GitStatusEntry>,
): GitStatusEntry | undefined {
  if (!task.primarySessionId) return undefined;
  const envKey = envIdBySessionId[task.primarySessionId] ?? task.primarySessionId;
  return gitStatusByEnvId[envKey];
}

/** Resolve diff stats for a task, falling back to direct git status when sessions aren't loaded. */
function resolveDiffStats(
  sessionDiffStats: { additions: number; deletions: number } | undefined,
  task: { primarySessionId?: string | null },
  envIdBySessionId: Record<string, string>,
  gitStatusByEnvId: Record<string, GitStatusEntry>,
): { additions: number; deletions: number } | undefined {
  if (sessionDiffStats) return sessionDiffStats;
  if (!task.primarySessionId) return undefined;
  const gs = getGitStatusForTask(task, envIdBySessionId, gitStatusByEnvId);
  if (!gs) return undefined;
  const a = gs.branch_additions ?? 0;
  const d = gs.branch_deletions ?? 0;
  return a > 0 || d > 0 ? { additions: a, deletions: d } : undefined;
}

/** Format PR info for display, capitalising the state. */
function toPrInfo(pr: TaskPR | undefined): { number: number; state: string } | undefined {
  if (!pr?.state) return undefined;
  return { number: pr.pr_number, state: pr.state[0].toUpperCase() + pr.state.slice(1) };
}

/** Map a kanban task to a sidebar item with session info and repository metadata. */
type SidebarCtx = {
  sessionsByTaskId: Record<string, TaskSession[]>;
  gitStatusByEnvId: Record<string, GitStatusEntry>;
  envIdBySessionId: Record<string, string>;
  repositorySlugById: Map<string, string | undefined>;
  taskPRsByTaskId: Record<string, TaskPR[] | undefined>;
  messagesBySession: Record<string, Message[]>;
  titleById: Map<string, string>;
  workflowNameById: Map<string, string>;
  stepTitleById: Map<string, string>;
};

function toIssueInfo(
  task: KanbanState["tasks"][number],
): { url: string; number: number } | undefined {
  return task.issueUrl && task.issueNumber
    ? { url: task.issueUrl, number: task.issueNumber }
    : undefined;
}

/** Map a kanban task to a sidebar item with session info and repository metadata. */
function toSidebarItem(
  task: KanbanState["tasks"][number] & { _workflowId: string },
  ctx: SidebarCtx,
) {
  const sessionInfo = getSessionInfoForTask(
    task.id,
    ctx.sessionsByTaskId,
    ctx.gitStatusByEnvId,
    ctx.envIdBySessionId,
  );
  const resolvedSessionState =
    sessionInfo.sessionState ?? (task.primarySessionState as TaskSessionState | undefined);
  const repoSlug = task.repositoryId ? ctx.repositorySlugById.get(task.repositoryId) : undefined;
  // Sidebar shows just one slot; pick the primary PR (first by created_at).
  const pr = ctx.taskPRsByTaskId[task.id]?.[0];
  const hasPendingClarificationRequest = hasPendingClarificationForSession(
    ctx.messagesBySession,
    task.primarySessionId,
  );
  const hasPendingPermission = hasPendingPermissionForSession(
    ctx.messagesBySession,
    task.primarySessionId,
  );

  const diffStats = resolveDiffStats(
    sessionInfo.diffStats,
    task,
    ctx.envIdBySessionId,
    ctx.gitStatusByEnvId,
  );

  return {
    id: task.id,
    title: task.title,
    state: task.state as TaskState | undefined,
    sessionState: resolvedSessionState,
    description: task.description,
    workflowId: task._workflowId,
    workflowName: ctx.workflowNameById.get(task._workflowId),
    workflowStepId: task.workflowStepId as string | undefined,
    workflowStepTitle: task.workflowStepId
      ? ctx.stepTitleById.get(task.workflowStepId as string)
      : undefined,
    repositoryPath: pr ? `${pr.owner}/${pr.repo}` : repoSlug,
    diffStats,
    isRemoteExecutor: task.isRemoteExecutor,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    remoteExecutorName: task.primaryExecutorName ?? undefined,
    primarySessionId: task.primarySessionId ?? null,
    hasPendingClarification: hasPendingClarificationRequest,
    hasPendingPermission,
    updatedAt: sessionInfo.updatedAt ?? task.updatedAt ?? task.createdAt,
    createdAt: task.createdAt,
    isArchived: false as boolean,
    parentTaskTitle: task.parentTaskId ? ctx.titleById.get(task.parentTaskId) : undefined,
    parentTaskId: task.parentTaskId ?? undefined,
    prInfo: toPrInfo(pr),
    isPRReview: task.isPRReview ?? false,
    isIssueWatch: task.isIssueWatch ?? false,
    issueInfo: toIssueInfo(task),
  };
}

type TaskSessionSidebarProps = {
  workspaceId: string | null;
  workflowId: string | null;
};

type SidebarItem = Omit<ReturnType<typeof toSidebarItem>, "workflowId"> & { workflowId?: string };

function buildArchivedItem(s: ReturnType<typeof useArchivedTaskState>): SidebarItem {
  return {
    id: s.archivedTaskId!,
    title: s.archivedTaskTitle ?? "Archived task",
    state: undefined,
    sessionState: undefined,
    description: undefined,
    workflowId: undefined,
    workflowName: undefined,
    workflowStepId: undefined,
    workflowStepTitle: undefined,
    repositoryPath: s.archivedTaskRepositoryPath,
    diffStats: undefined,
    isRemoteExecutor: false,
    remoteExecutorType: undefined,
    remoteExecutorName: undefined,
    primarySessionId: null,
    hasPendingClarification: false,
    hasPendingPermission: false,
    updatedAt: s.archivedTaskUpdatedAt,
    createdAt: undefined,
    isArchived: true,
    parentTaskTitle: undefined,
    parentTaskId: undefined,
    prInfo: undefined,
    isPRReview: false,
    isIssueWatch: false,
    issueInfo: undefined,
  };
}

function useSidebarData(workspaceId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const sessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);
  const gitStatusByEnvId = useAppStore((state) => state.gitStatus.byEnvironmentId);
  const envIdBySessionId = useAppStore((state) => state.environmentIdBySessionId);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const taskPRsByTaskId = useAppStore((state) => state.taskPRs.byTaskId);
  const messagesBySession = useAppStore((state) => state.messages.bySession);
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
      repositories.map((repo: Repository) => [
        repo.id,
        repo.provider_owner && repo.provider_name
          ? `${repo.provider_owner}/${repo.provider_name}`
          : repo.name || repo.local_path?.split("/").filter(Boolean).pop() || repo.local_path,
      ]),
    );
    const titleById = new Map(allTasks.map((t) => [t.id, t.title]));
    const workflowNameById = new Map(workflows.map((w) => [w.id, w.name]));
    const stepTitleById = new Map(allSteps.map((s) => [s.id, s.title]));
    const mapCtx = {
      sessionsByTaskId,
      gitStatusByEnvId,
      envIdBySessionId,
      repositorySlugById,
      taskPRsByTaskId,
      messagesBySession,
      titleById,
      workflowNameById,
      stepTitleById,
    };
    const items: SidebarItem[] = allTasks.map((task) => toSidebarItem(task, mapCtx));
    if (
      archivedState.isArchived &&
      archivedState.archivedTaskId &&
      !items.some((t) => t.id === archivedState.archivedTaskId)
    ) {
      items.unshift(buildArchivedItem(archivedState));
    }
    return items;
  }, [
    repositoriesByWorkspace,
    allTasks,
    allSteps,
    workflows,
    workspaceId,
    sessionsByTaskId,
    gitStatusByEnvId,
    envIdBySessionId,
    taskPRsByTaskId,
    messagesBySession,
    archivedState,
  ]);

  // Stable list of primary session IDs for the bulk-subscribe effect.
  // Derived from kanban tasks (always available) rather than sessionsByTaskId (loaded on-demand).
  const primarySessionIds = useStablePrimarySessionIds(allTasks);

  return {
    activeTaskId,
    selectedTaskId,
    allSteps,
    stepsByWorkflowId,
    isLoadingWorkflow,
    tasksWithRepositories,
    primarySessionIds,
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
  const [archivingTask, setArchivingTask] = useState<{ id: string; title: string } | null>(null);
  const [isArchiving, setIsArchiving] = useState(false);

  const handleArchiveTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      setArchivingTask({ id: taskId, title: task?.title ?? "this task" });
    },
    [store],
  );

  const handleArchiveConfirm = useCallback(
    async (opts: { cascade: boolean }) => {
      if (!archivingTask) return;
      setIsArchiving(true);
      try {
        await archiveAndSwitch(archivingTask.id, opts);
      } catch (error) {
        console.error("Failed to archive task:", error);
      } finally {
        setIsArchiving(false);
        setArchivingTask(null);
      }
    },
    [archivingTask, archiveAndSwitch],
  );

  return { archivingTask, setArchivingTask, isArchiving, handleArchiveTask, handleArchiveConfirm };
}

function useDeleteActions(
  store: StoreApi,
  removeTaskFromBoard: ReturnType<typeof useTaskRemoval>["removeTaskFromBoard"],
) {
  const { deleteTaskById } = useTaskActions();
  const [deletingTask, setDeletingTask] = useState<{ id: string; title: string } | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDeleteTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
      setDeletingTask({ id: taskId, title: task?.title ?? "this task" });
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

function useSidebarActions(store: StoreApi) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const [preparingTaskId, setPreparingTaskId] = useState<string | null>(null);
  const { renameTaskById } = useTaskActions();
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
    [loadTaskSessionsForTask, switchToSession, setActiveTask, store],
  );

  const archiveActions = useArchiveActions(store);
  const deleteActions = useDeleteActions(store, removeTaskFromBoard);

  const [renamingTask, setRenamingTask] = useState<{ id: string; title: string } | null>(null);

  const handleRenameTask = useCallback((taskId: string, currentTitle: string) => {
    setRenamingTask({ id: taskId, title: currentTitle });
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
    ...archiveActions,
    ...deleteActions,
  };
}

function useBulkGitStatusSubscription(primarySessionIds: string[]) {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  useEffect(() => {
    if (connectionStatus !== "connected" || primarySessionIds.length === 0) return;
    const client = getWebSocketClient();
    if (!client) return;
    // Skip active session — it's already subscribed + focused by the task page hooks
    const backgroundIds = activeSessionId
      ? primarySessionIds.filter((id) => id !== activeSessionId)
      : primarySessionIds;
    const unsubscribes = backgroundIds.map((id) => client.subscribeSession(id));
    return () => unsubscribes.forEach((u) => u());
  }, [primarySessionIds, connectionStatus, activeSessionId]);
}

function useGroupedSidebarView(displayTasks: TaskSwitcherItem[]) {
  const prefs = useSidebarTaskPrefs();
  const effectiveView = useEffectiveSidebarView();
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = prefs;
  const grouped = useMemo(
    () =>
      applyView(displayTasks, effectiveView, {
        pinnedTaskIds,
        orderedTaskIds,
        subtaskOrderByParentId,
      }),
    [displayTasks, effectiveView, pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId],
  );
  return { grouped, effectiveView, prefs };
}

export const TaskSessionSidebar = memo(function TaskSessionSidebar({
  workspaceId,
}: TaskSessionSidebarProps) {
  const store = useAppStoreApi();
  useRepositories(workspaceId);
  useWorkspacePRs(workspaceId);

  const {
    activeTaskId,
    selectedTaskId,
    stepsByWorkflowId,
    workflows,
    isLoadingWorkflow,
    tasksWithRepositories,
    primarySessionIds,
  } = useSidebarData(workspaceId);

  useBulkGitStatusSubscription(primarySessionIds);

  const sidebarActions = useSidebarActions(store);
  const {
    deletingTaskId,
    preparingTaskId,
    handleSelectTask,
    handleArchiveTask,
    handleDeleteTask,
    handleMoveToStep,
    handleRenameTask,
  } = sidebarActions;

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
  return (
    <PanelRoot data-testid="task-sidebar">
      <SidebarFilterBar />
      <PanelBody className="space-y-4 p-0" data-testid="task-sidebar-scroll">
        <TaskSwitcher
          grouped={grouped}
          workflows={workflows}
          stepsByWorkflowId={stepsByWorkflowId}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          collapsedGroupKeys={effectiveView.collapsedGroups}
          onToggleGroup={(groupKey) => toggleSidebarGroupCollapsed(effectiveView.id, groupKey)}
          collapsedSubtaskParentIds={collapsedSubtaskParents}
          onToggleSubtasks={toggleSubtaskCollapsed}
          onSelectTask={handleSelectTask}
          onRenameTask={handleRenameTask}
          onArchiveTask={handleArchiveTask}
          onDeleteTask={handleDeleteTask}
          onMoveToStep={handleMoveToStep}
          onTogglePin={togglePinnedTask}
          onReorderGroup={handleReorderGroup}
          onReorderSubtasks={handleReorderSubtasks}
          pinnedTaskIds={pinnedTaskIds}
          deletingTaskId={deletingTaskId}
          isLoading={isLoadingWorkflow}
          totalTaskCount={displayTasks.length}
        />
      </PanelBody>
      <SidebarDialogs actions={sidebarActions} />
    </PanelRoot>
  );
});
