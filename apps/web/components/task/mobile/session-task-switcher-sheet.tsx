"use client";

import { useCallback, useMemo, useState, memo } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Button } from "@kandev/ui/button";
import { TaskSwitcher } from "../task-switcher";
import type { TaskSwitcherItem } from "../task-switcher";
import { applyView } from "@/lib/sidebar/apply-view";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { WorkspaceSwitcher } from "../workspace-switcher";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { TaskArchiveConfirmDialog } from "../task-archive-confirm-dialog";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { replaceTaskUrl } from "@/lib/links";
import { fetchWorkflowSnapshot, listWorkflows } from "@/lib/api";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import { useTasks } from "@/hooks/use-tasks";
import { useTaskActions, useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { getSessionInfoForTask } from "@/lib/utils/session-info";
import { hasPendingClarificationForSession } from "@/lib/utils/pending-clarification";
import type {
  TaskState,
  TaskSessionState,
  Workspace,
  Repository,
  Task,
  WorkflowSnapshot,
} from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type SessionTaskSwitcherSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
};

// Helper to map workflow snapshot to kanban state
function mapSnapshotToKanban(snapshot: WorkflowSnapshot, newWorkflowId: string) {
  return {
    workflowId: newWorkflowId,
    isLoading: false,
    steps: snapshot.steps.map((step) => ({
      id: step.id,
      title: step.name,
      color: step.color,
      position: step.position,
      events: step.events,
    })),
    tasks: snapshot.tasks.map((task) => ({
      id: task.id,
      workflowStepId: task.workflow_step_id,
      title: task.title,
      description: task.description ?? undefined,
      position: task.position ?? 0,
      state: task.state,
      repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
      primarySessionId: task.primary_session_id ?? undefined,
      primarySessionState: task.primary_session_state ?? undefined,
      sessionCount: task.session_count ?? undefined,
      reviewStatus: task.review_status ?? undefined,
      primaryExecutorId: task.primary_executor_id ?? undefined,
      primaryExecutorType: task.primary_executor_type ?? undefined,
      primaryExecutorName: task.primary_executor_name ?? undefined,
      isRemoteExecutor: task.is_remote_executor ?? false,
      updatedAt: task.updated_at,
    })),
  };
}

// Helper to sort by updated_at descending
function sortByUpdatedAtDesc<T extends { updated_at?: string | null }>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const aDate = a.updated_at ? new Date(a.updated_at).getTime() : 0;
    const bDate = b.updated_at ? new Date(b.updated_at).getTime() : 0;
    return bDate - aDate;
  });
}

function useSheetData(workspaceId: string | null, workflowId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const sessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);
  const gitStatusByEnvId = useAppStore((state) => state.gitStatus.byEnvironmentId);
  const envIdBySessionId = useAppStore((state) => state.environmentIdBySessionId);
  const messagesBySession = useAppStore((state) => state.messages.bySession);
  const { tasks } = useTasks(workflowId);
  const steps = useAppStore((state) => state.kanban.steps);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const kanbanIsLoading = useAppStore((state) => state.kanban.isLoading ?? false);

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);

  const tasksWithRepositories = useMemo(() => {
    const repositories = workspaceId ? (repositoriesByWorkspace[workspaceId] ?? []) : [];
    const repositoryPathsById = new Map(
      repositories.map((repo: Repository) => [repo.id, repo.local_path]),
    );
    return tasks.map((task: KanbanState["tasks"][number]) => {
      const sessionInfo = getSessionInfoForTask(
        task.id,
        sessionsByTaskId,
        gitStatusByEnvId,
        envIdBySessionId,
      );
      return {
        id: task.id,
        title: task.title,
        state: task.state as TaskState | undefined,
        sessionState:
          sessionInfo.sessionState ?? (task.primarySessionState as TaskSessionState | undefined),
        description: task.description,
        workflowStepId: task.workflowStepId,
        repositoryPath: task.repositoryId ? repositoryPathsById.get(task.repositoryId) : undefined,
        diffStats: sessionInfo.diffStats,
        updatedAt: sessionInfo.updatedAt ?? task.updatedAt,
        isRemoteExecutor: task.isRemoteExecutor,
        remoteExecutorType: task.primaryExecutorType ?? undefined,
        remoteExecutorName: task.primaryExecutorName ?? undefined,
        primarySessionId: task.primarySessionId ?? null,
        hasPendingClarification: hasPendingClarificationForSession(
          messagesBySession,
          task.primarySessionId,
        ),
      };
    });
  }, [
    repositoriesByWorkspace,
    tasks,
    workspaceId,
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
    steps,
    workspaces,
    kanbanIsLoading,
    tasksWithRepositories,
    dialogSteps,
  };
}

type SheetNavOptions = {
  workspaceId: string | null;
  store: ReturnType<typeof useAppStoreApi>;
  loadTaskSessionsForTask: (
    taskId: string,
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

function useWorkspaceAndTaskCreatedActions(opts: SheetNavOptions) {
  const { workspaceId, store, setActiveSession, setActiveTask, onOpenChange } = opts;

  const handleWorkspaceChange = useCallback(
    async (newWorkspaceId: string) => {
      if (newWorkspaceId === workspaceId) return;
      await switchWorkspace(newWorkspaceId, opts);
    },
    [workspaceId, opts],
  );

  const handleTaskCreated = useCallback(
    (task: Task, _mode: "create" | "edit", meta?: { taskSessionId?: string | null }) => {
      store.setState((state) => {
        if (state.kanban.workflowId !== task.workflow_id) return state;
        const nextTask = {
          id: task.id,
          workflowStepId: task.workflow_step_id,
          title: task.title,
          description: task.description,
          position: task.position ?? 0,
          state: task.state,
          repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
          updatedAt: task.updated_at,
          primaryExecutorId: task.primary_executor_id ?? undefined,
          primaryExecutorType: task.primary_executor_type ?? undefined,
          primaryExecutorName: task.primary_executor_name ?? undefined,
          isRemoteExecutor: task.is_remote_executor ?? false,
        };
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

function useSheetActions(workspaceId: string | null, onOpenChange: (open: boolean) => void) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const store = useAppStoreApi();
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);
  const { deleteTaskById } = useTaskActions();
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { removeTaskFromBoard, loadTaskSessionsForTask } = useTaskRemoval({ store });

  const handleSelectTask = useCallback(
    (taskId: string) => {
      const kanbanTasks = store.getState().kanban.tasks;
      const task = kanbanTasks.find((t) => t.id === taskId);
      if (task?.primarySessionId) {
        setActiveSession(taskId, task.primarySessionId);
        loadTaskSessionsForTask(taskId);
        replaceTaskUrl(taskId);
        onOpenChange(false);
        return;
      }
      loadTaskSessionsForTask(taskId).then(async (sessions) => {
        const sessionId = sessions[0]?.id ?? null;
        if (sessionId) {
          setActiveSession(taskId, sessionId);
          replaceTaskUrl(taskId);
          onOpenChange(false);
          return;
        }
        // No session — prepare workspace
        const { request } = buildPrepareRequest(taskId);
        try {
          const resp = await launchSession(request);
          if (resp.session_id) {
            setActiveSession(taskId, resp.session_id);
          }
        } catch {
          // Fall through to default behavior
        }
        setActiveTask(taskId);
        replaceTaskUrl(taskId);
        onOpenChange(false);
      });
    },
    [loadTaskSessionsForTask, setActiveSession, setActiveTask, store, onOpenChange],
  );

  const [archivingTask, setArchivingTask] = useState<{ id: string; title: string } | null>(null);
  const [isArchiving, setIsArchiving] = useState(false);

  const handleArchiveTask = useCallback(
    (taskId: string) => {
      const task = store.getState().kanban.tasks.find((t) => t.id === taskId);
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

  const handleDeleteTask = useCallback(
    async (taskId: string) => {
      setDeletingTaskId(taskId);
      // Capture active state before the async API call — the WS "task.deleted"
      // handler may clear activeTaskId/activeSessionId before removeTaskFromBoard runs.
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

  const { handleWorkspaceChange, handleTaskCreated } = useWorkspaceAndTaskCreatedActions({
    workspaceId,
    store,
    loadTaskSessionsForTask,
    setActiveSession,
    setActiveTask,
    onOpenChange,
  });

  return {
    deletingTaskId,
    handleSelectTask,
    handleArchiveTask,
    handleDeleteTask,
    handleWorkspaceChange,
    handleTaskCreated,
    archivingTask,
    setArchivingTask,
    isArchiving,
    handleArchiveConfirm,
  };
}

function MobileTaskList({
  tasks,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onArchiveTask,
  onDeleteTask,
  deletingTaskId,
  isLoading,
}: {
  tasks: TaskSwitcherItem[];
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onArchiveTask: (taskId: string) => void;
  onDeleteTask: (taskId: string) => Promise<void> | void;
  deletingTaskId: string | null;
  isLoading?: boolean;
}) {
  const view = useEffectiveSidebarView();
  const { pinnedTaskIds, orderedTaskIds, togglePinnedTask, handleReorderGroup } =
    useSidebarTaskPrefs();
  const grouped = useMemo(
    () => applyView(tasks, view, { pinnedTaskIds, orderedTaskIds }),
    [tasks, view, pinnedTaskIds, orderedTaskIds],
  );
  return (
    <TaskSwitcher
      grouped={grouped}
      activeTaskId={activeTaskId}
      selectedTaskId={selectedTaskId}
      onSelectTask={onSelectTask}
      onArchiveTask={onArchiveTask}
      onDeleteTask={onDeleteTask}
      onTogglePin={togglePinnedTask}
      onReorderGroup={handleReorderGroup}
      pinnedTaskIds={pinnedTaskIds}
      deletingTaskId={deletingTaskId}
      isLoading={isLoading}
      totalTaskCount={tasks.length}
    />
  );
}

export const SessionTaskSwitcherSheet = memo(function SessionTaskSwitcherSheet({
  open,
  onOpenChange,
  workspaceId,
  workflowId,
}: SessionTaskSwitcherSheetProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const {
    activeTaskId,
    selectedTaskId,
    workspaces,
    kanbanIsLoading,
    tasksWithRepositories,
    dialogSteps,
  } = useSheetData(workspaceId, workflowId);
  const {
    deletingTaskId,
    handleSelectTask,
    handleArchiveTask,
    handleDeleteTask,
    handleWorkspaceChange,
    handleTaskCreated,
    archivingTask,
    setArchivingTask,
    isArchiving,
    handleArchiveConfirm,
  } = useSheetActions(workspaceId, onOpenChange);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        showCloseButton={false}
        side="left"
        className="w-[85vw] max-w-sm p-0 flex flex-col"
      >
        <SheetHeader className="p-4 pb-2 border-b border-border">
          <div className="flex items-center justify-between">
            <SheetTitle className="text-base">Tasks</SheetTitle>
            <Button
              size="sm"
              variant="outline"
              className="h-7 gap-1 cursor-pointer"
              onClick={() => setDialogOpen(true)}
            >
              <IconPlus className="h-4 w-4" />
              New
            </Button>
          </div>
          <div className="pt-2">
            <WorkspaceSwitcher
              workspaces={workspaces.map((w: Workspace) => ({ id: w.id, name: w.name }))}
              activeWorkspaceId={workspaceId}
              onSelect={handleWorkspaceChange}
            />
          </div>
        </SheetHeader>

        <div className="flex-1 min-h-0 overflow-y-auto p-2">
          <MobileTaskList
            tasks={tasksWithRepositories}
            activeTaskId={activeTaskId}
            selectedTaskId={selectedTaskId}
            onSelectTask={handleSelectTask}
            onArchiveTask={handleArchiveTask}
            onDeleteTask={handleDeleteTask}
            deletingTaskId={deletingTaskId}
            isLoading={kanbanIsLoading}
          />
        </div>
      </SheetContent>

      <TaskCreateDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        mode="create"
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={dialogSteps[0]?.id ?? null}
        steps={dialogSteps}
        onSuccess={handleTaskCreated}
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
    </Sheet>
  );
});
