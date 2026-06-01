"use client";

import { useCallback, useMemo, useState } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { replaceTaskUrl } from "@/lib/links";
import { fetchWorkflowSnapshot, listWorkflows } from "@/lib/api";
import { qk } from "@/lib/query/keys";
import {
  snapshotToWorkflowSnapshotData,
  workflowsFromResponse,
  type KanbanMultiData,
  type WorkflowsListData,
} from "@/lib/query/query-options/kanban";
import {
  useActiveWorkflowSteps,
  useKanbanSnapshots,
} from "@/hooks/domains/kanban/use-kanban-tasks";
import { useKanbanSnapshotMutator } from "@/hooks/domains/kanban/use-kanban-snapshots";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import { useWorkspaceSidebarTasks } from "@/hooks/domains/kanban/use-workspace-sidebar-tasks";
import { useWorkspaces } from "@/hooks/domains/workspace/use-workspaces";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import {
  useAllTaskSessionsByTaskFromCache,
  useTaskSessionById,
} from "@/hooks/domains/session/use-task-session-by-id";
import {
  useMessagesBySessionFromCache,
  useStablePrimarySessionIds,
} from "@/hooks/domains/session/use-messages-by-session-cache";
import { useGitStatusByEnvFromCache } from "@/hooks/domains/session/use-git-status-cache";
import { useTaskActions, useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { getSessionInfoForTask } from "@/lib/utils/session-info";
import {
  hasPendingClarificationForSession,
  hasPendingPermissionForSession,
} from "@/lib/utils/pending-clarification";
import {
  repositoryId as toRepositoryId,
  type TaskState,
  type TaskSessionState,
  type Repository,
  type Task,
} from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { repositorySlug } from "@/lib/repository-slug";
import { resolvePreferredSessionId } from "../task-select-helpers";

function sortByUpdatedAtDesc<T extends { updated_at?: string | null }>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const aDate = a.updated_at ? new Date(a.updated_at).getTime() : 0;
    const bDate = b.updated_at ? new Date(b.updated_at).getTime() : 0;
    return bDate - aDate;
  });
}

type SheetItemCtx = {
  repositoryPathsById: Map<string, string | undefined>;
  workflowNameById: Map<string, string>;
  stepTitleById: Map<string, string>;
  sessionsByTaskId: Parameters<typeof getSessionInfoForTask>[1];
  gitStatusByEnvId: Parameters<typeof getSessionInfoForTask>[2];
  envIdBySessionId: Parameters<typeof getSessionInfoForTask>[3];
  messagesBySession: Parameters<typeof hasPendingClarificationForSession>[0];
};

function toSheetItem(
  task: KanbanState["tasks"][number] & { _workflowId: string },
  ctx: SheetItemCtx,
) {
  const sessionInfo = getSessionInfoForTask(
    task.id,
    ctx.sessionsByTaskId,
    ctx.gitStatusByEnvId,
    ctx.envIdBySessionId,
  );
  return {
    id: task.id,
    title: task.title,
    // Carry the parent link so the mobile task switcher nests subtasks the same
    // way the desktop sidebar does (applyView/TaskSwitcher read parentTaskId).
    parentTaskId: task.parentTaskId ?? undefined,
    state: task.state as TaskState | undefined,
    sessionState:
      sessionInfo.sessionState ?? (task.primarySessionState as TaskSessionState | undefined),
    description: task.description,
    workflowId: task._workflowId,
    workflowName: ctx.workflowNameById.get(task._workflowId),
    workflowStepId: task.workflowStepId,
    workflowStepTitle: ctx.stepTitleById.get(task.workflowStepId),
    repositoryPath: task.repositoryId
      ? ctx.repositoryPathsById.get(toRepositoryId(task.repositoryId))
      : undefined,
    diffStats: sessionInfo.diffStats,
    updatedAt: sessionInfo.updatedAt ?? task.updatedAt,
    isRemoteExecutor: task.isRemoteExecutor,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    remoteExecutorName: task.primaryExecutorName ?? undefined,
    primarySessionId: task.primarySessionId ?? null,
    hasPendingClarification: hasPendingClarificationForSession(
      ctx.messagesBySession,
      task.primarySessionId,
    ),
    hasPendingPermission: hasPendingPermissionForSession(
      ctx.messagesBySession,
      task.primarySessionId,
    ),
  };
}

export function useSheetData(workspaceId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSession = useTaskSessionById(activeSessionId);
  const sessionsByTaskId = useAllTaskSessionsByTaskFromCache();
  const envIdBySessionId = useAppStore((state) => state.environmentIdBySessionId);
  const {
    allTasks,
    allSteps,
    stepsByWorkflowId,
    workflows,
    isLoading: tasksLoading,
  } = useWorkspaceSidebarTasks(workspaceId);
  // Pending-permission / pending-clarification indicators read messages from
  // the TanStack Query cache (canonical post-migration), not the Zustand mirror
  // which no longer holds fetched history. See use-messages-by-session-cache.
  const primarySessionIds = useStablePrimarySessionIds(allTasks);
  const messagesBySession = useMessagesBySessionFromCache(primarySessionIds);
  // Git indicators read the TQ git cache keyed by environment (bridge-populated).
  const gitStatusByEnvId = useGitStatusByEnvFromCache(primarySessionIds);
  const steps = useActiveWorkflowSteps();
  const { workspaces } = useWorkspaces();
  // Observe-only repos for the active workspace (no per-sheet fetch).
  const { repositories: workspaceRepositories } = useRepositories(workspaceId, false);

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) return activeSession?.task_id ?? activeTaskId;
    return activeTaskId;
  }, [activeSessionId, activeTaskId, activeSession]);

  const tasksWithRepositories = useMemo(() => {
    const repositories = workspaceRepositories;
    const ctx: SheetItemCtx = {
      repositoryPathsById: new Map(
        repositories.map((repo: Repository) => [repo.id, repositorySlug(repo)]),
      ),
      workflowNameById: new Map(workflows.map((w) => [w.id, w.name])),
      stepTitleById: new Map(allSteps.map((s) => [s.id, s.title])),
      sessionsByTaskId,
      gitStatusByEnvId,
      envIdBySessionId,
      messagesBySession,
    };
    return allTasks.map((task) => toSheetItem(task, ctx));
  }, [
    workspaceRepositories,
    allTasks,
    allSteps,
    workflows,
    sessionsByTaskId,
    gitStatusByEnvId,
    envIdBySessionId,
    messagesBySession,
  ]);

  const dialogSteps = useMemo(
    () =>
      steps.map((step: KanbanState["steps"][number]) => ({
        id: step.id,
        title: step.title,
        color: step.color,
        events: step.events,
      })),
    [steps],
  );

  return {
    activeTaskId,
    selectedTaskId,
    workspaces,
    workflows,
    stepsByWorkflowId,
    // Skeleton while the first snapshot fetch is in flight — otherwise shows "No tasks yet." even when tasks exist.
    tasksLoading,
    tasksWithRepositories,
    dialogSteps,
  };
}

type SheetNavOptions = {
  workspaceId: string | null;
  store: ReturnType<typeof useAppStoreApi>;
  queryClient: QueryClient;
  loadTaskSessionsForTask: (
    taskId: string,
  ) => Promise<Array<{ id: string; updated_at?: string | null }>>;
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
  onOpenChange: (open: boolean) => void;
};

async function switchWorkspace(newWorkspaceId: string, opts: SheetNavOptions) {
  const { store, queryClient, loadTaskSessionsForTask, setActiveSession, setActiveTask } = opts;
  const { onOpenChange } = opts;
  try {
    const workflowsResponse = await listWorkflows(newWorkspaceId, {
      cache: "no-store",
      includeHidden: true,
    });
    const newWorkspaceWorkflows = workflowsResponse.workflows ?? [];
    const firstWorkflow = newWorkspaceWorkflows.find((w) => !w.hidden);
    if (!firstWorkflow) return;

    const snapshot = await fetchWorkflowSnapshot(firstWorkflow.id, { cache: "no-store" });

    // Seed the workflows-list cache for the new workspace and the multi snapshot
    // cache for the chosen workflow; mark it active (client-only selection).
    queryClient.setQueryData<WorkflowsListData>(
      qk.kanban.workflowsList(newWorkspaceId),
      workflowsFromResponse(workflowsResponse),
    );
    queryClient.setQueryData<KanbanMultiData>(qk.kanban.multi(), (prev) => {
      const snap = snapshotToWorkflowSnapshotData(firstWorkflow.id, firstWorkflow.name, snapshot);
      if (!prev) return { snapshots: { [firstWorkflow.id]: snap } };
      return { ...prev, snapshots: { ...prev.snapshots, [firstWorkflow.id]: snap } };
    });
    store.getState().setActiveWorkflow(firstWorkflow.id);

    const mostRecentTask = sortByUpdatedAtDesc(snapshot.tasks)[0];
    if (mostRecentTask) {
      const sessions = await loadTaskSessionsForTask(mostRecentTask.id);
      const mostRecentSession = sortByUpdatedAtDesc(sessions)[0];
      if (mostRecentSession) {
        setActiveSession(mostRecentTask.id, mostRecentSession.id);
      } else {
        setActiveTask(mostRecentTask.id);
      }
      replaceTaskUrl(mostRecentTask.id);
    }
    onOpenChange(false);
  } catch (error) {
    console.error("Failed to switch workspace:", error);
  }
}

function mapTaskRepositories(
  repositories: Task["repositories"],
): KanbanState["tasks"][number]["repositories"] {
  return repositories?.map((r) => ({
    id: r.id,
    repository_id: r.repository_id,
    base_branch: r.base_branch,
    checkout_branch: r.checkout_branch,
    position: r.position,
  }));
}

function mergeSessionFields(
  task: Task,
  existing: KanbanState["tasks"][number] | undefined,
  taskSessionId: string | null,
) {
  return {
    primarySessionId:
      taskSessionId ?? task.primary_session_id ?? existing?.primarySessionId ?? undefined,
    primarySessionState: task.primary_session_state ?? existing?.primarySessionState ?? undefined,
    sessionCount: task.session_count ?? existing?.sessionCount ?? (taskSessionId ? 1 : undefined),
    reviewStatus: task.review_status ?? existing?.reviewStatus ?? undefined,
  };
}

/**
 * Build the kanban-store representation of a task for an upsert. Session-
 * derived fields (primarySessionId, sessionCount, etc.) fall through new
 * DTO → existing entry → meta.taskSessionId — that way an "edit" call doesn't
 * wipe sessions the existing entry carried, and "create with session" still
 * sets the primary correctly.
 */
function buildKanbanTaskUpsert(
  task: Task,
  existing: KanbanState["tasks"][number] | undefined,
  meta: { taskSessionId?: string | null } | undefined,
): KanbanState["tasks"][number] {
  const taskSessionId = meta?.taskSessionId ?? null;
  return {
    id: task.id,
    parentTaskId: task.parent_id ?? undefined,
    workflowStepId: task.workflow_step_id,
    title: task.title,
    description: task.description,
    position: task.position ?? 0,
    state: task.state,
    repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
    repositories: mapTaskRepositories(task.repositories),
    updatedAt: task.updated_at,
    ...mergeSessionFields(task, existing, taskSessionId),
    primaryExecutorId: task.primary_executor_id ?? undefined,
    primaryExecutorType: task.primary_executor_type ?? undefined,
    primaryExecutorName: task.primary_executor_name ?? undefined,
    isRemoteExecutor: task.is_remote_executor ?? false,
  };
}

function useWorkspaceAndTaskCreatedActions(opts: SheetNavOptions) {
  const {
    workspaceId,
    store,
    queryClient,
    loadTaskSessionsForTask,
    setActiveSession,
    setActiveTask,
    onOpenChange,
  } = opts;
  const { getSnapshot, setSnapshot } = useKanbanSnapshotMutator();

  const handleWorkspaceChange = useCallback(
    async (newWorkspaceId: string) => {
      if (newWorkspaceId === workspaceId) return;
      await switchWorkspace(newWorkspaceId, {
        workspaceId,
        store,
        queryClient,
        loadTaskSessionsForTask,
        setActiveSession,
        setActiveTask,
        onOpenChange,
      });
    },
    // Spread the individual fields rather than the `opts` object so callers
    // re-passing a fresh literal each render don't defeat memoization.
    [
      workspaceId,
      store,
      queryClient,
      loadTaskSessionsForTask,
      setActiveSession,
      setActiveTask,
      onOpenChange,
    ],
  );

  const handleTaskCreated = useCallback(
    (task: Task, _mode: "create" | "edit", meta?: { taskSessionId?: string | null }) => {
      // Optimistically upsert into the task's workflow snapshot in the TQ cache.
      const wfId = task.workflow_id;
      const snapshot = getSnapshot(wfId);
      if (snapshot) {
        const existing = snapshot.tasks.find(
          (item: KanbanState["tasks"][number]) => item.id === task.id,
        );
        const nextTask = buildKanbanTaskUpsert(task, existing, meta);
        setSnapshot(wfId, {
          ...snapshot,
          tasks: snapshot.tasks.some((item) => item.id === task.id)
            ? snapshot.tasks.map((item) => (item.id === task.id ? nextTask : item))
            : [...snapshot.tasks, nextTask],
        });
      }
      setActiveTask(task.id);
      if (meta?.taskSessionId) {
        setActiveSession(task.id, meta.taskSessionId);
      }
      replaceTaskUrl(task.id);
      onOpenChange(false);
    },
    [getSnapshot, setSnapshot, setActiveTask, setActiveSession, onOpenChange],
  );

  return { handleWorkspaceChange, handleTaskCreated };
}

type SelectTaskOptions = {
  setActiveTask: (taskId: string) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
  loadTaskSessionsForTask: SheetNavOptions["loadTaskSessionsForTask"];
  onOpenChange: (open: boolean) => void;
};

async function selectTaskWithoutPrimarySession(taskId: string, opts: SelectTaskOptions) {
  const { setActiveTask, setActiveSession, loadTaskSessionsForTask, onOpenChange } = opts;
  try {
    const sessions = await loadTaskSessionsForTask(taskId);
    const sessionId = sessions[0]?.id ?? null;
    if (sessionId) {
      setActiveSession(taskId, sessionId);
      replaceTaskUrl(taskId);
      onOpenChange(false);
      return;
    }
    // No session — prepare workspace.
    const { request } = buildPrepareRequest(taskId);
    try {
      const resp = await launchSession(request);
      if (resp.session_id) {
        setActiveSession(taskId, resp.session_id);
        replaceTaskUrl(taskId);
        onOpenChange(false);
        return;
      }
    } catch {
      // Fall through to default navigation.
    }
  } catch (error) {
    // Loading sessions can reject (network / 5xx). Don't strand the user;
    // fall back to plain task navigation so URL + state still align with tap.
    console.error("Failed to load sessions for task:", error);
  }
  setActiveTask(taskId);
  replaceTaskUrl(taskId);
  onOpenChange(false);
}

function useSheetDeleteActions(
  store: ReturnType<typeof useAppStoreApi>,
  removeTaskFromBoard: ReturnType<typeof useTaskRemoval>["removeTaskFromBoard"],
) {
  const { deleteTaskById } = useTaskActions();
  const snapshots = useKanbanSnapshots();
  const [deletingTask, setDeletingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDeleteTask = useCallback(
    (taskId: string) => {
      const task = findTaskInSnapshots(taskId, snapshots);
      setDeletingTask({
        id: taskId,
        title: task?.title ?? "this task",
        executorType: task?.primaryExecutorType,
      });
    },
    [snapshots],
  );

  const handleDeleteConfirm = useCallback(
    async (opts?: { cascade?: boolean }) => {
      if (!deletingTask || isDeleting) return;
      const taskId = deletingTask.id;
      setIsDeleting(true);
      // Capture active state before the async API call — the WS "task.deleted"
      // handler may clear activeTaskId/activeSessionId before removeTaskFromBoard runs.
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
    deletingTaskId,
    deletingTask,
    setDeletingTask,
    isDeleting,
    handleDeleteTask,
    handleDeleteConfirm,
  };
}

export function useSheetActions(workspaceId: string | null, onOpenChange: (open: boolean) => void) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const store = useAppStoreApi();
  const queryClient = useQueryClient();
  const snapshots = useKanbanSnapshots();
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { removeTaskFromBoard, loadTaskSessionsForTask } = useTaskRemoval({ store });
  const deleteActions = useSheetDeleteActions(store, removeTaskFromBoard);

  const handleSelectTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findTaskInSnapshots(taskId, snapshots);
      if (task?.primarySessionId) {
        const targetSessionId = resolvePreferredSessionId(
          taskId,
          task.primarySessionId,
          state.tasks.lastSessionByTaskId,
          state.environmentIdBySessionId,
        );
        setActiveSession(taskId, targetSessionId);
        loadTaskSessionsForTask(taskId);
        replaceTaskUrl(taskId);
        onOpenChange(false);
        return;
      }
      void selectTaskWithoutPrimarySession(taskId, {
        setActiveTask,
        setActiveSession,
        loadTaskSessionsForTask,
        onOpenChange,
      });
    },
    [snapshots, loadTaskSessionsForTask, setActiveSession, setActiveTask, store, onOpenChange],
  );

  const [archivingTask, setArchivingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [isArchiving, setIsArchiving] = useState(false);

  const handleArchiveTask = useCallback(
    (taskId: string) => {
      const task = findTaskInSnapshots(taskId, snapshots);
      setArchivingTask({
        id: taskId,
        title: task?.title ?? "this task",
        executorType: task?.primaryExecutorType,
      });
    },
    [snapshots],
  );

  const handleArchiveConfirm = useCallback(
    async (opts?: { cascade?: boolean }) => {
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

  const { handleWorkspaceChange, handleTaskCreated } = useWorkspaceAndTaskCreatedActions({
    workspaceId,
    store,
    queryClient,
    loadTaskSessionsForTask,
    setActiveSession,
    setActiveTask,
    onOpenChange,
  });

  return {
    handleSelectTask,
    handleArchiveTask,
    handleWorkspaceChange,
    handleTaskCreated,
    archivingTask,
    setArchivingTask,
    isArchiving,
    handleArchiveConfirm,
    ...deleteActions,
  };
}
