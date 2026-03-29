"use client";

import { useCallback, useMemo, useState, memo } from "react";
import type { TaskState, TaskSession, TaskSessionState, Repository, Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { TaskSwitcher } from "./task-switcher";
import { TaskRenameDialog } from "./task-rename-dialog";
import { Button } from "@kandev/ui/button";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { IconPlus } from "@tabler/icons-react";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import type { CommandItem } from "@/lib/commands/types";
import { replaceTaskUrl } from "@/lib/links";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useTaskActions, useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { performLayoutSwitch, useDockviewStore } from "@/lib/state/dockview-store";
import { INTENT_PR_REVIEW } from "@/lib/state/layout-manager";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildPrepareRequest } from "@/lib/services/session-launch-helpers";
import { getSessionInfoForTask } from "@/lib/utils/session-info";
import { useArchivedTaskState } from "./task-archived-context";

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

// Extracted component to isolate dialog state from sidebar
type NewTaskButtonProps = {
  workspaceId: string | null;
  workflowId: string | null;
  steps: Array<{
    id: string;
    title: string;
    color?: string;
    events?: {
      on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
      on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
    };
  }>;
  onSuccess: (
    task: Task,
    mode: "create" | "edit",
    meta?: { taskSessionId?: string | null },
  ) => void;
};

export const NewTaskButton = memo(function NewTaskButton({
  workspaceId,
  workflowId,
  steps,
  onSuccess,
}: NewTaskButtonProps) {
  const [dialogOpen, setDialogOpen] = useState(false);

  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const newTaskShortcut = getShortcut("NEW_TASK", keyboardShortcuts);
  const commands = useMemo<CommandItem[]>(
    () => [
      {
        id: "task-create",
        label: "Create New Task",
        group: "Tasks",
        icon: <IconPlus className="size-3.5" />,
        shortcut: newTaskShortcut,
        keywords: ["new", "create", "task", "add"],
        action: () => setDialogOpen(true),
        priority: 0,
      },
    ],
    [newTaskShortcut],
  );
  useRegisterCommands(commands);

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        className="h-6 gap-1 cursor-pointer"
        onClick={() => setDialogOpen(true)}
      >
        <IconPlus className="h-4 w-4" />
        Task
      </Button>
      <TaskCreateDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        mode="create"
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={steps[0]?.id ?? null}
        steps={steps}
        onSuccess={onSuccess}
      />
    </>
  );
});

/** Map a kanban task to a sidebar item with session info and repository metadata. */
function toSidebarItem(
  task: KanbanState["tasks"][number] & { _workflowId: string },
  ctx: {
    sessionsByTaskId: Record<string, TaskSession[]>;
    gitStatusByEnvId: Record<string, GitStatusEntry>;
    envIdBySessionId: Record<string, string>;
    repositorySlugById: Map<string, string | undefined>;
    taskPRsByTaskId: Record<string, { owner: string; repo: string } | undefined>;
    titleById: Map<string, string>;
  },
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
  return {
    id: task.id,
    title: task.title,
    state: task.state as TaskState | undefined,
    sessionState: resolvedSessionState,
    description: task.description,
    workflowId: task._workflowId,
    workflowStepId: task.workflowStepId as string | undefined,
    repositoryPath: pr ? `${pr.owner}/${pr.repo}` : repoSlug,
    diffStats: sessionInfo.diffStats,
    isRemoteExecutor: task.isRemoteExecutor,
    remoteExecutorType: task.primaryExecutorType ?? undefined,
    remoteExecutorName: task.primaryExecutorName ?? undefined,
    primarySessionId: task.primarySessionId ?? null,
    updatedAt: sessionInfo.updatedAt ?? task.updatedAt,
    createdAt: task.createdAt,
    isArchived: false as boolean,
    parentTaskTitle: task.parentTaskId ? ctx.titleById.get(task.parentTaskId) : undefined,
  };
}

type TaskSessionSidebarProps = {
  workspaceId: string | null;
  workflowId: string | null;
};

type StepInfo = { id: string; title: string; color: string; position: number };

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
          : repo.local_path,
      ]),
    );
    const titleById = new Map(allTasks.map((t) => [t.id, t.title]));
    const mapCtx = {
      sessionsByTaskId,
      gitStatusByEnvId,
      envIdBySessionId,
      repositorySlugById,
      taskPRsByTaskId: taskPRsByTaskId as Record<
        string,
        { owner: string; repo: string } | undefined
      >,
      titleById,
    };
    type SidebarItem = Omit<ReturnType<typeof toSidebarItem>, "workflowId"> & {
      workflowId?: string;
    };
    const items: SidebarItem[] = allTasks.map((task) => toSidebarItem(task, mapCtx));
    if (
      archivedState.isArchived &&
      archivedState.archivedTaskId &&
      !items.some((t) => t.id === archivedState.archivedTaskId)
    ) {
      items.unshift({
        id: archivedState.archivedTaskId,
        title: archivedState.archivedTaskTitle ?? "Archived task",
        state: undefined,
        sessionState: undefined,
        description: undefined,
        workflowId: undefined,
        workflowStepId: undefined,
        repositoryPath: archivedState.archivedTaskRepositoryPath,
        diffStats: undefined,
        isRemoteExecutor: false,
        remoteExecutorType: undefined,
        remoteExecutorName: undefined,
        primarySessionId: null,
        updatedAt: archivedState.archivedTaskUpdatedAt,
        createdAt: undefined,
        isArchived: true,
        parentTaskTitle: undefined,
      });
    }
    return items;
  }, [
    repositoriesByWorkspace,
    allTasks,
    workspaceId,
    sessionsByTaskId,
    gitStatusByEnvId,
    envIdBySessionId,
    taskPRsByTaskId,
    archivedState,
  ]);

  return {
    activeTaskId,
    selectedTaskId,
    allSteps,
    stepsByWorkflowId,
    isLoadingWorkflow,
    tasksWithRepositories,
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

      const targetTasks = snapshot.tasks
        .filter((t) => t.workflowStepId === targetStepId && t.id !== taskId)
        .sort((a, b) => a.position - b.position);
      const nextPosition = targetTasks.length;

      const originalTasks = snapshot.tasks;

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
        // Rollback
        const currentSnapshot = store.getState().kanbanMulti.snapshots[workflowId];
        if (currentSnapshot) {
          store.getState().setWorkflowSnapshot(workflowId, {
            ...currentSnapshot,
            tasks: originalTasks,
          });
        }
        console.error("Failed to move task:", error);
      }
    },
    [store, moveTaskById],
  );
}

function useSidebarActions(store: StoreApi) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);
  const [preparingTaskId, setPreparingTaskId] = useState<string | null>(null);
  const { deleteTaskById, renameTaskById } = useTaskActions();
  const archiveAndSwitch = useArchiveAndSwitchTask({ useLayoutSwitch: true });
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

  const handleArchiveTask = useCallback(
    async (taskId: string) => {
      try {
        await archiveAndSwitch(taskId);
      } catch (error) {
        console.error("Failed to archive task:", error);
      }
    },
    [archiveAndSwitch],
  );

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
    handleArchiveTask,
    handleDeleteTask,
    handleMoveToStep,
    renamingTask,
    setRenamingTask,
    handleRenameTask,
    handleRenameSubmit,
  };
}

export const TaskSessionSidebar = memo(function TaskSessionSidebar({
  workspaceId,
}: TaskSessionSidebarProps) {
  const store = useAppStoreApi();
  useAllWorkflowSnapshots(workspaceId);

  const {
    activeTaskId,
    selectedTaskId,
    allSteps,
    stepsByWorkflowId,
    isLoadingWorkflow,
    tasksWithRepositories,
  } = useSidebarData(workspaceId);
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
  } = useSidebarActions(store);

  const displayTasks = useMemo(
    () =>
      preparingTaskId
        ? tasksWithRepositories.map((t) =>
            t.id === preparingTaskId ? { ...t, sessionState: "STARTING" as TaskSessionState } : t,
          )
        : tasksWithRepositories,
    [tasksWithRepositories, preparingTaskId],
  );

  return (
    <PanelRoot data-testid="task-sidebar">
      <PanelBody className="space-y-4 p-0">
        <TaskSwitcher
          tasks={displayTasks}
          steps={allSteps.map((step) => ({ id: step.id, title: step.title, color: step.color }))}
          stepsByWorkflowId={stepsByWorkflowId}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          onSelectTask={handleSelectTask}
          onRenameTask={handleRenameTask}
          onArchiveTask={handleArchiveTask}
          onDeleteTask={handleDeleteTask}
          onMoveToStep={handleMoveToStep}
          deletingTaskId={deletingTaskId}
          isLoading={isLoadingWorkflow}
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
    </PanelRoot>
  );
});
