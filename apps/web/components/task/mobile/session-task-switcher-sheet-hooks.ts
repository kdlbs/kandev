"use client";

import { useCallback, useMemo, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { replaceTaskUrl } from "@/lib/links";
import { fetchWorkflowSnapshot, listWorkflows } from "@/lib/api";
import { useWorkspaceSidebarTasks } from "@/hooks/domains/kanban/use-workspace-sidebar-tasks";
import { useTaskActions, useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskDetachDialog } from "@/hooks/use-detach-task";
import { useNestTaskByDrag } from "@/hooks/use-nest-task";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { workspaceModeFromMetadata } from "@/lib/kanban/map-task";
import { type Repository, type Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { repositorySlug } from "@/lib/repository-slug";
import { mapSnapshotToKanban, sortByUpdatedAtDesc } from "./session-task-switcher-sheet-helpers";
import { toSheetItem, type SheetItemCtx } from "./session-task-switcher-sheet-item";
import {
  selectTaskFromSheet,
  type TaskSheetSelectionController,
} from "./session-task-switcher-sheet-selection";
import { taskPendingSelectionSnapshot } from "../task-select-helpers";
import { useTranslation } from "react-i18next";

function findSheetTask(
  state: ReturnType<ReturnType<typeof useAppStoreApi>["getState"]>,
  taskId: string,
) {
  const activeTask = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
  if (activeTask) return activeTask;
  for (const tasks of Object.values(state.sidebarArchivedTasks?.itemsByWorkspaceId ?? {})) {
    const archivedTask = tasks.find((task) => task.id === taskId);
    if (archivedTask) return archivedTask;
  }
  return undefined;
}

export function useSheetData(workspaceId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const {
    allTasks,
    allSteps,
    stepsByWorkflowId,
    wipQueueByTaskId,
    workflows,
    isLoading: tasksLoading,
    archivedError,
    retryArchivedTasks,
  } = useWorkspaceSidebarTasks(workspaceId);
  const steps = useAppStore((state) => state.kanban.steps);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const acknowledgedAgentErrors = useAppStore((state) => state.acknowledgedAgentErrors);
  const dismissedAgentErrors = useAppStore((state) => state.dismissedAgentErrors);

  const selectedTaskId = activeTaskId;

  const tasksWithRepositories = useMemo(() => {
    const repositories = workspaceId ? (repositoriesByWorkspace[workspaceId] ?? []) : [];
    const ctx: SheetItemCtx = {
      repositoryPathsById: new Map(
        repositories.map((repo: Repository) => [repo.id, repositorySlug(repo)]),
      ),
      workflowNameById: new Map(workflows.map((w) => [w.id, w.name])),
      stepTitleById: new Map(allSteps.map((s) => [s.id, s.title])),
      acknowledgedAgentErrors,
      dismissedAgentErrors,
      wipQueueByTaskId,
    };
    return allTasks.map((task) => toSheetItem(task, ctx));
  }, [
    repositoriesByWorkspace,
    allTasks,
    allSteps,
    workflows,
    workspaceId,
    acknowledgedAgentErrors,
    dismissedAgentErrors,
    wipQueueByTaskId,
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
    archivedError,
    retryArchivedTasks,
    tasksWithRepositories,
    dialogSteps,
  };
}

type SheetNavOptions = {
  workspaceId: string | null;
  store: ReturnType<typeof useAppStoreApi>;
  loadTaskSessionsForTask: (
    taskId: string,
    options?: { force?: boolean },
  ) => Promise<Array<{ id: string; updated_at?: string | null }>>;
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
  onOpenChange: (open: boolean) => void;
};

async function switchWorkspace(newWorkspaceId: string, opts: SheetNavOptions) {
  const { store, loadTaskSessionsForTask, setActiveSession, setActiveTask, onOpenChange } = opts;
  store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: true } }));
  try {
    const workflowsResponse = await listWorkflows(newWorkspaceId, {
      cache: "no-store",
      includeHidden: true,
    });
    const newWorkspaceWorkflows = workflowsResponse.workflows ?? [];
    const firstWorkflow = newWorkspaceWorkflows.find((w) => !w.hidden);
    if (!firstWorkflow) {
      store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: false } }));
      return;
    }
    const snapshot = await fetchWorkflowSnapshot(firstWorkflow.id);
    store.setState((state) => ({
      ...state,
      workflows: {
        ...state.workflows,
        items: [
          ...state.workflows.items.filter(
            (w: { workspaceId: string }) => w.workspaceId !== newWorkspaceId,
          ),
          ...newWorkspaceWorkflows.map((w) => ({
            id: w.id,
            workspaceId: w.workspace_id,
            name: w.name,
            hidden: w.hidden,
          })),
        ],
        activeId: firstWorkflow.id,
      },
      kanban: mapSnapshotToKanban(snapshot, firstWorkflow.id),
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
    store.setState((state) => ({ ...state, kanban: { ...state.kanban, isLoading: false } }));
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
    branch_policy_id: r.branch_policy_id,
    branch_policy_name: r.branch_policy_name,
    branch_policy_base_branch: r.branch_policy_base_branch,
    branch_policy_branch_template: r.branch_policy_branch_template,
    branch_policy_pull_request_target: r.branch_policy_pull_request_target,
    position: r.position,
  }));
}

function mergeSessionFields(
  task: Task,
  existing: KanbanState["tasks"][number] | undefined,
  taskSessionId: string | null,
) {
  return {
    primarySessionId: resolvePrimarySessionId(task, existing, taskSessionId),
    primarySessionState: resolvePrimarySessionState(task, existing),
    primarySessionPendingAction: resolvePrimarySessionPendingAction(task, existing),
    taskPendingAction: resolveTaskPendingAction(task, existing),
    sessionCount: resolveSessionCount(task, existing, taskSessionId),
    reviewStatus: resolveReviewStatus(task, existing),
  };
}

function resolveTaskPendingAction(task: Task, existing: KanbanState["tasks"][number] | undefined) {
  if ("task_pending_action" in task) return task.task_pending_action ?? undefined;
  return existing?.taskPendingAction ?? undefined;
}

function resolvePrimarySessionId(
  task: Task,
  existing: KanbanState["tasks"][number] | undefined,
  taskSessionId: string | null,
) {
  return taskSessionId ?? task.primary_session_id ?? existing?.primarySessionId ?? undefined;
}

function resolvePrimarySessionState(
  task: Task,
  existing: KanbanState["tasks"][number] | undefined,
) {
  return task.primary_session_state ?? existing?.primarySessionState ?? undefined;
}

function resolvePrimarySessionPendingAction(
  task: Task,
  existing: KanbanState["tasks"][number] | undefined,
) {
  if ("primary_session_pending_action" in task) {
    return task.primary_session_pending_action ?? undefined;
  }
  return existing?.primarySessionPendingAction ?? undefined;
}

function resolveSessionCount(
  task: Task,
  existing: KanbanState["tasks"][number] | undefined,
  taskSessionId: string | null,
) {
  return task.session_count ?? existing?.sessionCount ?? (taskSessionId ? 1 : undefined);
}

function resolveReviewStatus(task: Task, existing: KanbanState["tasks"][number] | undefined) {
  return task.review_status ?? existing?.reviewStatus ?? undefined;
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
    workspaceMode: workspaceModeFromMetadata(task.metadata),
    workflowId: task.workflow_id,
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
        loadTaskSessionsForTask,
        setActiveSession,
        setActiveTask,
        onOpenChange,
      });
    },
    // Spread the individual fields rather than the `opts` object so callers
    // re-passing a fresh literal each render don't defeat memoization.
    [workspaceId, store, loadTaskSessionsForTask, setActiveSession, setActiveTask, onOpenChange],
  );

  const handleTaskCreated = useCallback(
    (task: Task, _mode: "create" | "edit", meta?: { taskSessionId?: string | null }) => {
      store.setState((state) => {
        if (state.kanban.workflowId !== task.workflow_id) return state;
        const existing = state.kanban.tasks.find(
          (item: KanbanState["tasks"][number]) => item.id === task.id,
        );
        const nextTask = buildKanbanTaskUpsert(task, existing, meta);
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.some(
              (item: KanbanState["tasks"][number]) => item.id === task.id,
            )
              ? state.kanban.tasks.map((item: KanbanState["tasks"][number]) =>
                  item.id === task.id ? nextTask : item,
                )
              : [...state.kanban.tasks, nextTask],
          },
        };
      });
      setActiveTask(task.id);
      if (meta?.taskSessionId) {
        setActiveSession(task.id, meta.taskSessionId);
      }
      replaceTaskUrl(task.id);
      onOpenChange(false);
    },
    [store, setActiveTask, setActiveSession, onOpenChange],
  );

  return { handleWorkspaceChange, handleTaskCreated };
}

function useSheetDeleteActions(
  store: ReturnType<typeof useAppStoreApi>,
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
      const task = findSheetTask(state, taskId);
      setDeletingTask({
        id: taskId,
        title: task?.title ?? t("task:thisTask"),
        executorType: task?.primaryExecutorType,
      });
    },
    [store, t],
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

/**
 * Re-parent via drag: runs the same composite nest operation the context menu
 * uses, resolving the workflow from the snapshot keys.
 */
function useSheetNestTask() {
  return useNestTaskByDrag();
}

export function useSheetArchiveActions(
  store: ReturnType<typeof useAppStoreApi>,
  archiveAndSwitch: ReturnType<typeof useArchiveAndSwitchTask>,
) {
  const { t } = useTranslation();
  const [archivingTask, setArchivingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const [isArchiving, setIsArchiving] = useState(false);
  const [archivingTaskId, setArchivingTaskId] = useState<string | null>(null);

  const runArchive = useCallback(
    async (taskId: string, opts?: { cascade?: boolean }) => {
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
      const task = findSheetTask(store.getState(), taskId);
      setArchivingTask({
        id: taskId,
        title: task?.title ?? t("task:thisTask"),
        executorType: task?.primaryExecutorType,
      });
    },
    [runArchive, store, t],
  );

  const handleArchiveConfirm = useCallback(
    async (opts?: { cascade?: boolean }) => {
      if (!archivingTask) return;
      await runArchive(archivingTask.id, opts);
    },
    [archivingTask, runArchive],
  );

  return {
    handleArchiveTask,
    archivingTask,
    archivingTaskId,
    setArchivingTask,
    isArchiving,
    handleArchiveConfirm,
  };
}

export function useSheetActions(
  workspaceId: string | null,
  onOpenChange: (open: boolean) => void,
  selection: TaskSheetSelectionController,
) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const store = useAppStoreApi();
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const archiveActions = useSheetArchiveActions(store, archiveAndSwitch);
  const { removeTaskFromBoard, loadTaskSessionsForTask } = useTaskRemoval({ store });
  const deleteActions = useSheetDeleteActions(store, removeTaskFromBoard);
  const detachActions = useTaskDetachDialog(store);
  const handleNestTask = useSheetNestTask();
  const handleSelectTask = useCallback(
    (taskId: string) => {
      const state = store.getState();
      selectTaskFromSheet({
        taskId,
        selectionController: selection,
        task: findSheetTask(state, taskId),
        state: {
          lastSessionByTaskId: state.tasks.lastSessionByTaskId,
          environmentIdBySessionId: state.environmentIdBySessionId,
          taskSessionsById: state.taskSessions.items,
        },
        setActiveTask,
        setActiveSession,
        loadTaskSessionsForTask,
        getTaskPendingSnapshot: (selectedTaskId) => {
          const selectedTask = findSheetTask(store.getState(), selectedTaskId);
          return selectedTask ? taskPendingSelectionSnapshot(selectedTask) : undefined;
        },
        navigate: replaceTaskUrl,
        onOpenChange,
      });
    },
    [loadTaskSessionsForTask, setActiveSession, setActiveTask, store, onOpenChange, selection],
  );

  const { handleWorkspaceChange, handleTaskCreated } = useWorkspaceAndTaskCreatedActions({
    workspaceId,
    store,
    loadTaskSessionsForTask,
    setActiveSession,
    setActiveTask,
    onOpenChange,
  });

  return {
    handleSelectTask,
    ...archiveActions,
    handleWorkspaceChange,
    handleTaskCreated,
    handleNestTask,
    ...deleteActions,
    ...detachActions,
  };
}
