'use client';

import { useCallback, useMemo, useState, memo } from 'react';
import { IconPlus } from '@tabler/icons-react';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@kandev/ui/sheet';
import { Button } from '@kandev/ui/button';
import { TaskSwitcher } from '../task-switcher';
import { WorkspaceSwitcher } from '../workspace-switcher';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { linkToSession } from '@/lib/links';
import { fetchWorkflowSnapshot, listWorkflows } from '@/lib/api';
import { useTasks } from '@/hooks/use-tasks';
import { useTaskActions } from '@/hooks/use-task-actions';
import { useTaskRemoval } from '@/hooks/use-task-removal';
import type { TaskState, Workspace, Repository, TaskSession, Task, WorkflowSnapshot } from '@/lib/types/http';
import type { KanbanState } from '@/lib/state/slices';

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
      sessionCount: task.session_count ?? undefined,
      reviewStatus: task.review_status ?? undefined,
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

export const SessionTaskSwitcherSheet = memo(function SessionTaskSwitcherSheet({
  open,
  onOpenChange,
  workspaceId,
  workflowId,
}: SessionTaskSwitcherSheetProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);

  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const sessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);
  const gitStatusBySessionId = useAppStore((state) => state.gitStatus.bySessionId);
  const { tasks } = useTasks(workflowId);
  const steps = useAppStore((state) => state.kanban.steps);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const store = useAppStoreApi();

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) {
      return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    }
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);

  const kanbanIsLoading = useAppStore((state) => state.kanban.isLoading ?? false);
  const { deleteTaskById, archiveTaskById } = useTaskActions();
  const { removeTaskFromBoard, loadTaskSessionsForTask } = useTaskRemoval({ store });

  // Get session info for a task (diff stats)
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
    [sessionsByTaskId, gitStatusBySessionId]
  );

  const tasksWithRepositories = useMemo(() => {
    const repositories = workspaceId ? repositoriesByWorkspace[workspaceId] ?? [] : [];
    const repositoryPathsById = new Map(repositories.map((repo: Repository) => [repo.id, repo.local_path]));
    return tasks.map((task: KanbanState['tasks'][number]) => {
      const sessionInfo = getSessionInfoForTask(task.id);
      return {
        id: task.id,
        title: task.title,
        state: task.state as TaskState | undefined,
        description: task.description,
        workflowStepId: task.workflowStepId,
        repositoryPath: task.repositoryId ? repositoryPathsById.get(task.repositoryId) : undefined,
        diffStats: sessionInfo.diffStats,
        updatedAt: sessionInfo.updatedAt ?? task.updatedAt,
      };
    });
  }, [repositoriesByWorkspace, tasks, workspaceId, getSessionInfoForTask]);

  const updateUrl = useCallback((sessionId: string) => {
    if (typeof window === 'undefined') return;
    window.history.replaceState({}, '', linkToSession(sessionId));
  }, []);

  const handleSelectTask = useCallback(
    (taskId: string) => {
      const kanbanTasks = store.getState().kanban.tasks;
      const task = kanbanTasks.find((t) => t.id === taskId);

      if (task?.primarySessionId) {
        setActiveSession(taskId, task.primarySessionId);
        updateUrl(task.primarySessionId);
        loadTaskSessionsForTask(taskId);
        onOpenChange(false);
        return;
      }

      loadTaskSessionsForTask(taskId).then((sessions) => {
        const sessionId = sessions[0]?.id ?? null;
        if (!sessionId) {
          setActiveTask(taskId);
          onOpenChange(false);
          return;
        }
        setActiveSession(taskId, sessionId);
        updateUrl(sessionId);
        onOpenChange(false);
      });
    },
    [loadTaskSessionsForTask, setActiveSession, setActiveTask, updateUrl, store, onOpenChange]
  );

  const handleArchiveTask = useCallback(
    async (taskId: string) => {
      try {
        await archiveTaskById(taskId);
        await removeTaskFromBoard(taskId);
      } catch (error) {
        console.error('Failed to archive task:', error);
      }
    },
    [archiveTaskById, removeTaskFromBoard]
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
    [deleteTaskById, removeTaskFromBoard]
  );

  const handleWorkspaceChange = useCallback(
    async (newWorkspaceId: string) => {
      if (newWorkspaceId === workspaceId) return;

      store.setState((state) => ({
        ...state,
        kanban: { ...state.kanban, isLoading: true },
      }));

      try {
        const workflowsResponse = await listWorkflows(newWorkspaceId, { cache: 'no-store' });
        const newWorkspaceWorkflows = workflowsResponse.workflows ?? [];

        const firstWorkflow = newWorkspaceWorkflows[0];
        if (!firstWorkflow) {
          store.setState((state) => ({
            ...state,
            kanban: { ...state.kanban, isLoading: false },
          }));
          return;
        }

        const snapshot = await fetchWorkflowSnapshot(firstWorkflow.id);

        store.setState((state) => ({
          ...state,
          workflows: {
            ...state.workflows,
            items: [
              ...state.workflows.items.filter((w: { workspaceId: string }) => w.workspaceId !== newWorkspaceId),
              ...newWorkspaceWorkflows.map((w) => ({ id: w.id, workspaceId: w.workspace_id, name: w.name })),
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
            updateUrl(mostRecentSession.id);
          } else {
            setActiveTask(mostRecentTask.id);
          }
        }
        onOpenChange(false);
      } catch (error) {
        console.error('Failed to switch workspace:', error);
        store.setState((state) => ({
          ...state,
          kanban: { ...state.kanban, isLoading: false },
        }));
      }
    },
    [workspaceId, store, loadTaskSessionsForTask, setActiveSession, setActiveTask, updateUrl, onOpenChange]
  );

  const dialogSteps = useMemo(
    () => steps.map((step: KanbanState['steps'][number]) => ({
      id: step.id,
      title: step.title,
      color: step.color,
      events: step.events,
    })),
    [steps]
  );

  const handleTaskCreated = useCallback(
    (task: Task, _mode: 'create' | 'edit', meta?: { taskSessionId?: string | null }) => {
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
        };
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.some((item: KanbanState['tasks'][number]) => item.id === task.id)
              ? state.kanban.tasks.map((item: KanbanState['tasks'][number]) => (item.id === task.id ? nextTask : item))
              : [...state.kanban.tasks, nextTask],
          },
        };
      });
      setActiveTask(task.id);
      if (meta?.taskSessionId) {
        setActiveSession(task.id, meta.taskSessionId);
        updateUrl(meta.taskSessionId);
      }
      onOpenChange(false);
    },
    [store, setActiveTask, setActiveSession, updateUrl, onOpenChange]
  );

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent showCloseButton={false} side="left" className="w-[85vw] max-w-sm p-0 flex flex-col">
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
          <TaskSwitcher
            tasks={tasksWithRepositories}
            steps={steps.map((step: KanbanState['steps'][number]) => ({ id: step.id, title: step.title, color: step.color }))}
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
    </Sheet>
  );
});
