"use client";

import { useCallback, useMemo, useState } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { replaceTaskUrl } from "@/lib/links";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import { useWorkspaceSidebarTasks } from "@/hooks/domains/kanban/use-workspace-sidebar-tasks";
import { useCachedRepositories } from "@/hooks/domains/workspace/use-repository-cache";
import { useWorkspaces } from "@/hooks/domains/workspace/use-workspaces";
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
  type TaskSession,
  type TaskSessionState,
  type Repository,
  type Task,
} from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import { workflowsQueryOptions, workflowSnapshotQueryOptions } from "@/lib/query/query-options";
import { qk } from "@/lib/query/keys";
import {
  updateWorkflowSnapshotQuery,
  workflowSnapshotQueryData,
} from "@/lib/query/workflow-snapshot-cache";
import { workflowSnapshotToKanbanData } from "@/lib/kanban/snapshot";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { repositorySlug } from "@/lib/repository-slug";
import { resolvePreferredSessionId } from "../task-select-helpers";
import { agentErrorMessageForTask } from "@/lib/task-agent-error";
import { sessionId as toSessionId } from "@/lib/types/ids";
import { usePersistTaskAgentErrorAcknowledgements } from "../use-agent-error-acknowledgements";

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
  sessionsById: Record<string, TaskSession>;
  sessionsByTaskId: Parameters<typeof getSessionInfoForTask>[1];
  gitStatusByEnvId: Parameters<typeof getSessionInfoForTask>[2];
  envIdBySessionId: Parameters<typeof getSessionInfoForTask>[3];
  messagesBySession: Parameters<typeof hasPendingClarificationForSession>[0];
  dismissedAgentErrors: Record<string, string>;
  acknowledgedAgentErrors: Record<string, string>;
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
    agentErrorMessage: agentErrorMessageForTask(task, ctx.sessionsById, ctx.sessionsByTaskId, ctx),
  };
}

export function useSheetData(workspaceId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const sessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);
  const gitStatusByEnvId = useAppStore((state) => state.gitStatus.byEnvironmentId);
  const envIdBySessionId = useAppStore((state) => state.environmentIdBySessionId);
  const messagesBySession = useAppStore((state) => state.messages.bySession);
  const dismissedAgentErrors = useAppStore((state) => state.dismissedAgentErrors);
  const acknowledgedAgentErrors = useAppStore((state) => state.acknowledgedAgentErrors);
  const {
    allTasks,
    allSteps,
    stepsByWorkflowId,
    workflows,
    isLoading: tasksLoading,
  } = useWorkspaceSidebarTasks(workspaceId);
  const { items: workspaces } = useWorkspaces();
  const repositories = useCachedRepositories(workspaceId);

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);

  usePersistTaskAgentErrorAcknowledgements({
    tasks: allTasks,
    sessionsByTaskId,
    sessionsById,
    messagesBySession,
    dismissedAgentErrors,
  });

  const tasksWithRepositories = useMemo(() => {
    const ctx: SheetItemCtx = {
      repositoryPathsById: new Map(
        repositories.map((repo: Repository) => [repo.id, repositorySlug(repo)]),
      ),
      workflowNameById: new Map(workflows.map((w) => [w.id, w.name])),
      stepTitleById: new Map(allSteps.map((s) => [s.id, s.title])),
      sessionsById,
      sessionsByTaskId,
      gitStatusByEnvId,
      envIdBySessionId,
      messagesBySession,
      dismissedAgentErrors,
      acknowledgedAgentErrors,
    };
    return allTasks.map((task) => toSheetItem(task, ctx));
  }, [
    repositories,
    allTasks,
    allSteps,
    workflows,
    sessionsById,
    sessionsByTaskId,
    gitStatusByEnvId,
    envIdBySessionId,
    messagesBySession,
    dismissedAgentErrors,
    acknowledgedAgentErrors,
  ]);

  const dialogSteps = useMemo(
    () =>
      allSteps.map((step: KanbanState["steps"][number]) => ({
        id: step.id,
        title: step.title,
        color: step.color,
        events: step.events,
      })),
    [allSteps],
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
  const {
    store,
    queryClient,
    loadTaskSessionsForTask,
    setActiveSession,
    setActiveTask,
    onOpenChange,
  } = opts;
  try {
    const newWorkspaceWorkflows = await queryClient.fetchQuery({
      ...workflowsQueryOptions(newWorkspaceId, { includeHidden: true }),
      staleTime: 0,
    });
    const firstWorkflow = newWorkspaceWorkflows.find((w) => !w.hidden);
    if (!firstWorkflow) return;
    const snapshot = await queryClient.fetchQuery({
      ...workflowSnapshotQueryOptions(firstWorkflow.id),
      staleTime: 0,
    });
    store.setState((state) => ({
      ...state,
      workflows: {
        ...state.workflows,
        activeId: firstWorkflow.id,
      },
    }));
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

type TaskSuccessMeta = { taskSessionId?: string | null; willNavigate?: boolean };

function firstPresent<T>(...values: Array<T | null | undefined>): T | undefined {
  for (const value of values) {
    if (value !== null && value !== undefined) return value;
  }
  return undefined;
}

function mergeSnapshotSessionFields(
  task: Task,
  existing: Task | undefined,
  taskSessionId: string | null,
) {
  const metaSessionId = taskSessionId ? toSessionId(taskSessionId) : undefined;
  return {
    primary_session_id: firstPresent(
      metaSessionId,
      task.primary_session_id,
      existing?.primary_session_id,
    ),
    primary_session_state: firstPresent(
      task.primary_session_state,
      existing?.primary_session_state,
    ),
    session_count: firstPresent(
      task.session_count,
      existing?.session_count,
      taskSessionId ? 1 : undefined,
    ),
    review_status: firstPresent(task.review_status, existing?.review_status),
  };
}

/**
 * Build the workflow-snapshot representation of a task for an upsert. Session-
 * derived fields (primary_session_id, session_count, etc.) fall through new
 * DTO → existing entry → meta.taskSessionId so edits don't wipe sessions and
 * "create with session" still sets the primary correctly.
 */
function buildSnapshotTaskUpsert(
  task: Task,
  existing: Task | undefined,
  meta: TaskSuccessMeta | undefined,
): Task {
  const taskSessionId = meta?.taskSessionId ?? null;
  return {
    ...task,
    ...mergeSnapshotSessionFields(task, existing, taskSessionId),
  };
}

function upsertTaskInWorkflowSnapshot(
  queryClient: QueryClient,
  task: Task,
  meta?: TaskSuccessMeta,
): void {
  let nextTask: Task | null = null;
  updateWorkflowSnapshotQuery(queryClient, task.workflow_id, (snapshot) => {
    const existing = snapshot.tasks.find((item) => item.id === task.id);
    nextTask = buildSnapshotTaskUpsert(task, existing, meta);
    return {
      ...snapshot,
      tasks: existing
        ? snapshot.tasks.map((item) => (item.id === task.id ? nextTask! : item))
        : [...snapshot.tasks, nextTask],
    };
  });
  if (!nextTask) return;
  queryClient.setQueryData(qk.tasks.detail(task.id), (current: unknown) => {
    if (!current) return nextTask;
    return { ...(current as Task), ...nextTask };
  });
}

function findTaskInCachedQueryData(
  queryClient: QueryClient,
  taskId: string,
): KanbanState["tasks"][number] | null {
  const detail = queryClient.getQueryData<Task>(qk.tasks.detail(taskId));
  if (detail) return toKanbanTask(detail);

  for (const snapshot of workflowSnapshotQueryData(queryClient)) {
    const found = workflowSnapshotToKanbanData(snapshot).tasks.find((task) => task.id === taskId);
    if (found) return found;
  }
  return null;
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
    (task: Task, _mode: "create" | "edit", meta?: TaskSuccessMeta) => {
      upsertTaskInWorkflowSnapshot(queryClient, task, meta);
      setActiveTask(task.id);
      if (meta?.taskSessionId) {
        setActiveSession(task.id, meta.taskSessionId);
      }
      replaceTaskUrl(task.id);
      onOpenChange(false);
    },
    [queryClient, setActiveTask, setActiveSession, onOpenChange],
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
  queryClient: QueryClient,
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
      const task = findTaskInCachedQueryData(queryClient, taskId);
      setDeletingTask({
        id: taskId,
        title: task?.title ?? "this task",
        executorType: task?.primaryExecutorType,
      });
    },
    [queryClient],
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
  const queryClient = useQueryClient();
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const store = useAppStoreApi();
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { removeTaskFromBoard, loadTaskSessionsForTask } = useTaskRemoval({ store });
  const deleteActions = useSheetDeleteActions(store, queryClient, removeTaskFromBoard);

  const handleSelectTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      const task = findTaskInCachedQueryData(queryClient, taskId);
      if (task?.primarySessionId) {
        const targetSessionId = resolvePreferredSessionId({
          taskId,
          primarySessionId: task.primarySessionId,
          lastSessionByTaskId: state.tasks.lastSessionByTaskId,
          environmentIdBySessionId: state.environmentIdBySessionId,
          taskSessionsById: state.taskSessions.items,
        });
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
    [loadTaskSessionsForTask, setActiveSession, setActiveTask, store, queryClient, onOpenChange],
  );

  const [archivingTask, setArchivingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [isArchiving, setIsArchiving] = useState(false);

  const handleArchiveTask = useCallback(
    (taskId: string) => {
      const task = findTaskInCachedQueryData(queryClient, taskId);
      setArchivingTask({
        id: taskId,
        title: task?.title ?? "this task",
        executorType: task?.primaryExecutorType,
      });
    },
    [queryClient],
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
