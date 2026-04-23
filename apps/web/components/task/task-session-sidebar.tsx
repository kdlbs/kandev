"use client";

import { useCallback, useEffect, useMemo, useState, memo } from "react";
import type { TaskState, TaskSession, TaskSessionState, Repository } from "@/lib/types/http";
import type { TaskPR } from "@/lib/types/github";
import type { KanbanState } from "@/lib/state/slices";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { TaskSwitcher } from "./task-switcher";
import { applyView } from "@/lib/sidebar/apply-view";
import { SidebarFilterBar } from "./sidebar-filter/sidebar-filter-bar";
import { MOCK_ITEMS, MOCK_SIDEBAR } from "./sidebar-mock-data";
import { TaskRenameDialog } from "./task-rename-dialog";
import { TaskArchiveConfirmDialog } from "./task-archive-confirm-dialog";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { replaceTaskUrl } from "@/lib/links";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useTaskActions, useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { performLayoutSwitch, useDockviewStore } from "@/lib/state/dockview-store";
import { INTENT_PR_REVIEW } from "@/lib/state/layout-manager";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import { getSessionInfoForTask } from "@/lib/utils/session-info";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useArchivedTaskState } from "./task-archived-context";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useWorkspacePRs } from "@/hooks/domains/github/use-task-pr";

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

/** Find a task across all workflow snapshots */
function findTaskInSnapshots(
  snapshots: Record<string, { tasks: KanbanState["tasks"] }>,
  taskId: string,
): KanbanState["tasks"][number] | undefined {
  for (const snapshot of Object.values(snapshots)) {
    const task = snapshot.tasks.find((t: KanbanState["tasks"][number]) => t.id === taskId);
    if (task) return task;
  }
  return undefined;
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
  taskPRsByTaskId: Record<string, TaskPR | undefined>;
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
  const pr = ctx.taskPRsByTaskId[task.id];

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

type StepInfo = { id: string; title: string; color: string; position: number };
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
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const isMultiLoading = useAppStore((state) => state.kanbanMulti.isLoading);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const taskPRsByTaskId = useAppStore((state) => state.taskPRs.byTaskId);
  const archivedState = useArchivedTaskState();

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);

  const isLoadingWorkflow = isMultiLoading && Object.keys(snapshots).length === 0;

  const { allTasks, allSteps, stepsByWorkflowId } = useMemo(() => {
    const tasks: Array<KanbanState["tasks"][number] & { _workflowId: string }> = [];
    const stepMap = new Map<string, StepInfo>();
    const wfSteps: Record<string, StepInfo[]> = {};
    for (const [wfId, snapshot] of Object.entries(snapshots)) {
      for (const step of snapshot.steps) if (!stepMap.has(step.id)) stepMap.set(step.id, step);
      wfSteps[wfId] = [...snapshot.steps].sort((a, b) => a.position - b.position);
      tasks.push(...snapshot.tasks.map((t) => ({ ...t, _workflowId: wfId })));
    }
    const sortedSteps = [...stepMap.values()].sort((a, b) => a.position - b.position);
    return { allTasks: tasks, allSteps: sortedSteps, stepsByWorkflowId: wfSteps };
  }, [snapshots]);

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
    const workflowNameById = new Map(
      Object.entries(snapshots).map(([wfId, snap]) => [wfId, snap.workflowName]),
    );
    const stepTitleById = new Map(allSteps.map((s) => [s.id, s.title]));
    const mapCtx = {
      sessionsByTaskId,
      gitStatusByEnvId,
      envIdBySessionId,
      repositorySlugById,
      taskPRsByTaskId: taskPRsByTaskId as Record<string, TaskPR | undefined>,
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
    snapshots,
    workspaceId,
    sessionsByTaskId,
    gitStatusByEnvId,
    envIdBySessionId,
    taskPRsByTaskId,
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
  };
}

type StoreApi = ReturnType<typeof useAppStoreApi>;
type SwitchFn = (
  taskId: string,
  sessionId: string,
  oldSessionId: string | null | undefined,
) => void;

function buildSwitchToSession(
  setActiveSession: (taskId: string, sessionId: string) => void,
): SwitchFn {
  return (taskId, sessionId, oldSessionId) => {
    setActiveSession(taskId, sessionId);
    performLayoutSwitch(oldSessionId ?? null, sessionId);
  };
}

async function prepareAndSwitchTask(
  taskId: string,
  store: StoreApi,
  switchToSession: SwitchFn,
  setPreparingTaskId: (id: string | null) => void,
): Promise<boolean> {
  setPreparingTaskId(taskId);
  // Capture before the async launch — WS events may update activeSessionId
  // by the time launchSession resolves, causing the layout switch to use the
  // wrong "old" session and leave stale panels (e.g. plan panel) visible.
  const oldSessionId = store.getState().tasks.activeSessionId;
  try {
    const { request } = buildPrepareRequest(taskId);
    const resp = await launchSession(request);
    if (resp.session_id) {
      switchToSession(taskId, resp.session_id, oldSessionId);
      // Apply PR review layout if the task has PR metadata
      if (store.getState().taskPRs.byTaskId[taskId]) {
        const { api, buildDefaultLayout } = useDockviewStore.getState();
        if (api) buildDefaultLayout(api, INTENT_PR_REVIEW);
      }
      return true;
    }
    return false;
  } catch {
    return false;
  } finally {
    setPreparingTaskId(null);
  }
}

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
      const task = findTaskInSnapshots(store.getState().kanbanMulti.snapshots, taskId);
      setArchivingTask({ id: taskId, title: task?.title ?? "this task" });
    },
    [store],
  );

  const handleArchiveConfirm = useCallback(async () => {
    if (!archivingTask) return;
    setIsArchiving(true);
    try {
      await archiveAndSwitch(archivingTask.id);
    } catch (error) {
      console.error("Failed to archive task:", error);
    } finally {
      setIsArchiving(false);
      setArchivingTask(null);
    }
  }, [archivingTask, archiveAndSwitch]);

  return { archivingTask, setArchivingTask, isArchiving, handleArchiveTask, handleArchiveConfirm };
}

function useSidebarActions(store: StoreApi) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);
  const [preparingTaskId, setPreparingTaskId] = useState<string | null>(null);
  const { deleteTaskById, renameTaskById } = useTaskActions();
  const { removeTaskFromBoard, loadTaskSessionsForTask } = useTaskRemoval({
    store,
    useLayoutSwitch: true,
  });

  const switchToSession = useMemo(() => buildSwitchToSession(setActiveSession), [setActiveSession]);

  const handleSelectTask = useCallback(
    (taskId: string) => {
      const oldSessionId = store.getState().tasks.activeSessionId;
      const task = findTaskInSnapshots(store.getState().kanbanMulti.snapshots, taskId);
      if (task?.primarySessionId) {
        switchToSession(taskId, task.primarySessionId, oldSessionId);
        loadTaskSessionsForTask(taskId);
        replaceTaskUrl(taskId);
        return;
      }
      loadTaskSessionsForTask(taskId).then(async (sessions) => {
        const currentOldSessionId = store.getState().tasks.activeSessionId;
        const primary = sessions.find((s: { is_primary?: boolean }) => s.is_primary);
        const sessionId = primary?.id ?? sessions[0]?.id ?? null;
        if (sessionId) {
          switchToSession(taskId, sessionId, currentOldSessionId);
          replaceTaskUrl(taskId);
          return;
        }
        // No session — prepare workspace and switch to it
        const switched = await prepareAndSwitchTask(
          taskId,
          store,
          switchToSession,
          setPreparingTaskId,
        );
        if (!switched) setActiveTask(taskId);
        replaceTaskUrl(taskId);
      });
    },
    [loadTaskSessionsForTask, switchToSession, setActiveTask, store],
  );

  const archiveActions = useArchiveActions(store);

  const handleDeleteTask = useCallback(
    async (taskId: string) => {
      setDeletingTaskId(taskId);
      const { activeTaskId: wasActiveTaskId, activeSessionId: wasActiveSessionId } =
        store.getState().tasks;
      try {
        await deleteTaskById(taskId);
        await removeTaskFromBoard(taskId, { wasActiveTaskId, wasActiveSessionId });
      } finally {
        setDeletingTaskId(null);
      }
    },
    [deleteTaskById, removeTaskFromBoard, store],
  );

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
    deletingTaskId,
    preparingTaskId,
    handleSelectTask,
    handleDeleteTask,
    handleMoveToStep,
    renamingTask,
    setRenamingTask,
    handleRenameTask,
    handleRenameSubmit,
    ...archiveActions,
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

function useEffectiveSidebarView() {
  const sidebarSlice = useAppStore((state) => state.sidebarViews);
  return useMemo(() => {
    const active = sidebarSlice.views.find((v) => v.id === sidebarSlice.activeViewId);
    if (!active) return sidebarSlice.views[0];
    const d = sidebarSlice.draft;
    if (!d || d.baseViewId !== active.id) return active;
    return { ...active, filters: d.filters, sort: d.sort, group: d.group };
  }, [sidebarSlice.views, sidebarSlice.activeViewId, sidebarSlice.draft]);
}

export const TaskSessionSidebar = memo(function TaskSessionSidebar({
  workspaceId,
}: TaskSessionSidebarProps) {
  const store = useAppStoreApi();
  useAllWorkflowSnapshots(workspaceId);
  useRepositories(workspaceId);
  useWorkspacePRs(workspaceId);

  const {
    activeTaskId,
    selectedTaskId,
    stepsByWorkflowId,
    isLoadingWorkflow,
    tasksWithRepositories,
    primarySessionIds,
  } = useSidebarData(workspaceId);

  useBulkGitStatusSubscription(primarySessionIds);

  const {
    deletingTaskId,
    preparingTaskId,
    handleSelectTask,
    handleArchiveTask,
    handleDeleteTask,
    handleMoveToStep,
    renamingTask,
    setRenamingTask,
    handleRenameTask,
    handleRenameSubmit,
    archivingTask,
    setArchivingTask,
    isArchiving,
    handleArchiveConfirm,
  } = useSidebarActions(store);

  const displayTasks = useMemo(() => {
    if (MOCK_SIDEBAR) return MOCK_ITEMS;
    return preparingTaskId
      ? tasksWithRepositories.map((t) =>
          t.id === preparingTaskId ? { ...t, sessionState: "STARTING" as TaskSessionState } : t,
        )
      : tasksWithRepositories;
  }, [tasksWithRepositories, preparingTaskId]);

  const toggleSidebarGroupCollapsed = useAppStore((state) => state.toggleSidebarGroupCollapsed);
  const effectiveView = useEffectiveSidebarView();
  const grouped = useMemo(
    () => applyView(displayTasks, effectiveView),
    [displayTasks, effectiveView],
  );

  return (
    <PanelRoot data-testid="task-sidebar">
      <SidebarFilterBar />
      <PanelBody className="space-y-4 p-0">
        <TaskSwitcher
          grouped={grouped}
          stepsByWorkflowId={stepsByWorkflowId}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          collapsedGroupKeys={effectiveView.collapsedGroups}
          onToggleGroup={(groupKey) => toggleSidebarGroupCollapsed(effectiveView.id, groupKey)}
          onSelectTask={handleSelectTask}
          onRenameTask={handleRenameTask}
          onArchiveTask={handleArchiveTask}
          onDeleteTask={handleDeleteTask}
          onMoveToStep={handleMoveToStep}
          deletingTaskId={deletingTaskId}
          isLoading={isLoadingWorkflow}
          totalTaskCount={displayTasks.length}
        />
      </PanelBody>
      <TaskRenameDialog
        open={renamingTask !== null}
        onOpenChange={(open) => {
          if (!open) setRenamingTask(null);
        }}
        currentTitle={renamingTask?.title ?? ""}
        onSubmit={handleRenameSubmit}
      />
      <TaskArchiveConfirmDialog
        open={archivingTask !== null}
        onOpenChange={(open) => {
          if (!open) setArchivingTask(null);
        }}
        taskTitle={archivingTask?.title ?? ""}
        isArchiving={isArchiving}
        onConfirm={handleArchiveConfirm}
      />
    </PanelRoot>
  );
});
