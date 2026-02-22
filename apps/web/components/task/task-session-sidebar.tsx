"use client";

import { useCallback, useMemo, useState, memo } from "react";
import type { TaskState, Repository, TaskSession, Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import { TaskSwitcher } from "./task-switcher";
import { Button } from "@kandev/ui/button";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { IconPlus } from "@tabler/icons-react";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { linkToSession } from "@/lib/links";
import { useAllWorkflowSnapshots } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { performLayoutSwitch } from "@/lib/state/dockview-store";
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

type TaskSessionSidebarProps = {
  workspaceId: string | null;
  workflowId: string | null;
};

function useSidebarData(workspaceId: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const sessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);
  const gitStatusBySessionId = useAppStore((state) => state.gitStatus.bySessionId);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const isMultiLoading = useAppStore((state) => state.kanbanMulti.isLoading);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const archivedState = useArchivedTaskState();

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);

  const isLoadingWorkflow = isMultiLoading && Object.keys(snapshots).length === 0;

  const { allTasks, allSteps } = useMemo(() => {
    const tasks: KanbanState["tasks"] = [];
    const stepMap = new Map<
      string,
      { id: string; title: string; color: string; position: number }
    >();
    for (const snapshot of Object.values(snapshots)) {
      for (const step of snapshot.steps) {
        if (!stepMap.has(step.id)) stepMap.set(step.id, step);
      }
      tasks.push(...snapshot.tasks);
    }
    const sortedSteps = [...stepMap.values()].sort((a, b) => a.position - b.position);
    return { allTasks: tasks, allSteps: sortedSteps };
  }, [snapshots]);

  const getSessionInfoForTask = useCallback(
    (taskId: string) => {
      const sessions = sessionsByTaskId[taskId] ?? [];
      if (sessions.length === 0) return { diffStats: undefined, updatedAt: undefined };
      const primarySession = sessions.find((s: TaskSession) => s.is_primary);
      const latestSession = primarySession ?? sessions[0];
      if (!latestSession) return { diffStats: undefined, updatedAt: undefined };
      const updatedAt = latestSession.updated_at;
      const gitStatus = gitStatusBySessionId[latestSession.id];
      if (!gitStatus?.files) return { diffStats: undefined, updatedAt };
      let additions = 0;
      let deletions = 0;
      for (const file of Object.values(gitStatus.files)) {
        additions += file.additions ?? 0;
        deletions += file.deletions ?? 0;
      }
      const diffStats = additions === 0 && deletions === 0 ? undefined : { additions, deletions };
      return { diffStats, updatedAt };
    },
    [sessionsByTaskId, gitStatusBySessionId],
  );

  const tasksWithRepositories = useMemo(() => {
    const repositories = workspaceId ? (repositoriesByWorkspace[workspaceId] ?? []) : [];
    const repositoryPathsById = new Map(
      repositories.map((repo: Repository) => [repo.id, repo.local_path]),
    );
    const items = allTasks.map((task: KanbanState["tasks"][number]) => {
      const sessionInfo = getSessionInfoForTask(task.id);
      return {
        id: task.id,
        title: task.title,
        state: task.state as TaskState | undefined,
        description: task.description,
        workflowStepId: task.workflowStepId as string | undefined,
        repositoryPath: task.repositoryId ? repositoryPathsById.get(task.repositoryId) : undefined,
        diffStats: sessionInfo.diffStats,
        isRemoteExecutor: task.isRemoteExecutor,
        remoteExecutorType: task.primaryExecutorType ?? undefined,
        remoteExecutorName: task.primaryExecutorName ?? undefined,
        primarySessionId: task.primarySessionId ?? null,
        updatedAt: sessionInfo.updatedAt ?? task.updatedAt,
        isArchived: false as boolean,
      };
    });
    if (
      archivedState.isArchived &&
      archivedState.archivedTaskId &&
      !items.some((t) => t.id === archivedState.archivedTaskId)
    ) {
      items.unshift({
        id: archivedState.archivedTaskId,
        title: archivedState.archivedTaskTitle ?? "Archived task",
        state: undefined,
        description: undefined,
        workflowStepId: undefined,
        repositoryPath: archivedState.archivedTaskRepositoryPath,
        diffStats: undefined,
        isRemoteExecutor: false,
        remoteExecutorType: undefined,
        remoteExecutorName: undefined,
        primarySessionId: null,
        updatedAt: archivedState.archivedTaskUpdatedAt,
        isArchived: true,
      });
    }
    return items;
  }, [repositoriesByWorkspace, allTasks, workspaceId, getSessionInfoForTask, archivedState]);

  return { activeTaskId, selectedTaskId, allSteps, isLoadingWorkflow, tasksWithRepositories };
}

function useSidebarActions(store: ReturnType<typeof useAppStoreApi>) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);
  const { deleteTaskById, archiveTaskById } = useTaskActions();
  const { removeTaskFromBoard, loadTaskSessionsForTask } = useTaskRemoval({
    store,
    useLayoutSwitch: true,
  });

  const updateUrl = useCallback((sessionId: string) => {
    if (typeof window === "undefined") return;
    window.history.replaceState({}, "", linkToSession(sessionId));
  }, []);

  const switchToSession = useCallback(
    (taskId: string, sessionId: string, oldSessionId: string | null | undefined) => {
      setActiveSession(taskId, sessionId);
      performLayoutSwitch(oldSessionId ?? null, sessionId);
      updateUrl(sessionId);
    },
    [setActiveSession, updateUrl],
  );

  const handleSelectTask = useCallback(
    (taskId: string) => {
      const oldSessionId = store.getState().tasks.activeSessionId;
      const task = findTaskInSnapshots(store.getState().kanbanMulti.snapshots, taskId);
      if (task?.primarySessionId) {
        switchToSession(taskId, task.primarySessionId, oldSessionId);
        loadTaskSessionsForTask(taskId);
        return;
      }
      loadTaskSessionsForTask(taskId).then((sessions) => {
        const currentOldSessionId = store.getState().tasks.activeSessionId;
        const sessionId = sessions[0]?.id ?? null;
        if (!sessionId) {
          setActiveTask(taskId);
          return;
        }
        switchToSession(taskId, sessionId, currentOldSessionId);
      });
    },
    [loadTaskSessionsForTask, switchToSession, setActiveTask, store],
  );

  const handleArchiveTask = useCallback(
    async (taskId: string) => {
      try {
        await archiveTaskById(taskId);
        await removeTaskFromBoard(taskId);
      } catch (error) {
        console.error("Failed to archive task:", error);
      }
    },
    [archiveTaskById, removeTaskFromBoard],
  );

  const handleDeleteTask = useCallback(
    async (taskId: string) => {
      setDeletingTaskId(taskId);
      try {
        await deleteTaskById(taskId);
        await removeTaskFromBoard(taskId);
      } finally {
        setDeletingTaskId(null);
      }
    },
    [deleteTaskById, removeTaskFromBoard],
  );

  return { deletingTaskId, handleSelectTask, handleArchiveTask, handleDeleteTask };
}

export const TaskSessionSidebar = memo(function TaskSessionSidebar({
  workspaceId,
}: TaskSessionSidebarProps) {
  const store = useAppStoreApi();
  useAllWorkflowSnapshots(workspaceId);

  const { activeTaskId, selectedTaskId, allSteps, isLoadingWorkflow, tasksWithRepositories } =
    useSidebarData(workspaceId);
  const { deletingTaskId, handleSelectTask, handleArchiveTask, handleDeleteTask } =
    useSidebarActions(store);

  return (
    <PanelRoot>
      <PanelBody className="space-y-4 p-0">
        <TaskSwitcher
          tasks={tasksWithRepositories}
          steps={allSteps.map((step) => ({ id: step.id, title: step.title, color: step.color }))}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          onSelectTask={handleSelectTask}
          onArchiveTask={handleArchiveTask}
          onDeleteTask={handleDeleteTask}
          deletingTaskId={deletingTaskId}
          isLoading={isLoadingWorkflow}
        />
      </PanelBody>
    </PanelRoot>
  );
});
