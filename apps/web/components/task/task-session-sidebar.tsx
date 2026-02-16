'use client';

import { useCallback, useMemo, useState, memo } from 'react';
import type { TaskState, Repository, TaskSession, Task } from '@/lib/types/http';
import type { KanbanState } from '@/lib/state/slices';
import { TaskSwitcher } from './task-switcher';
import { Button } from '@kandev/ui/button';
import { PanelRoot, PanelBody } from './panel-primitives';
import { IconPlus } from '@tabler/icons-react';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { linkToSession } from '@/lib/links';
import { listTaskSessions } from '@/lib/api';
import { useAllWorkflowSnapshots } from '@/hooks/domains/kanban/use-all-workflow-snapshots';
import { useTaskActions } from '@/hooks/use-task-actions';
import { performLayoutSwitch } from '@/lib/state/dockview-store';

// Extracted component to isolate dialog state from sidebar
type NewTaskButtonProps = {
  workspaceId: string | null;
  workflowId: string | null;
  steps: Array<{ id: string; title: string; color?: string; events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }>; on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }> } }>;
  onSuccess: (task: Task, mode: 'create' | 'edit', meta?: { taskSessionId?: string | null }) => void;
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

export const TaskSessionSidebar = memo(function TaskSessionSidebar({ workspaceId, workflowId: _workflowId }: TaskSessionSidebarProps) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const sessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);
  const gitStatusBySessionId = useAppStore((state) => state.gitStatus.bySessionId);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const isMultiLoading = useAppStore((state) => state.kanbanMulti.isLoading);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const setTaskSessionsForTask = useAppStore((state) => state.setTaskSessionsForTask);
  const setTaskSessionsLoading = useAppStore((state) => state.setTaskSessionsLoading);
  const selectedTaskId = useMemo(() => {
    if (activeSessionId) {
      return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    }
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);
  const snapshotKeys = Object.keys(snapshots);
  const isLoadingWorkflow = isMultiLoading && snapshotKeys.length === 0;


  // Load all workflow snapshots for the workspace
  useAllWorkflowSnapshots(workspaceId);

  // Merge tasks and steps from all workflow snapshots
  const { allTasks, allSteps } = useMemo(() => {
    const tasks: KanbanState['tasks'] = [];
    const stepMap = new Map<string, { id: string; title: string; color: string; position: number }>();

    for (const snapshot of Object.values(snapshots)) {
      for (const step of snapshot.steps) {
        if (!stepMap.has(step.id)) {
          stepMap.set(step.id, step);
        }
      }
      tasks.push(...snapshot.tasks);
    }

    // Sort steps by position for consistent ordering
    const sortedSteps = [...stepMap.values()].sort((a, b) => a.position - b.position);
    return { allTasks: tasks, allSteps: sortedSteps };
  }, [snapshots]);
  const store = useAppStoreApi();

  // Task deletion state and handler
  const { deleteTaskById } = useTaskActions();
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);

  // Helper to get session info for a task (diff stats and updatedAt)
  const getSessionInfoForTask = useCallback(
    (taskId: string) => {
      // Get sessions for this task
      const sessions = sessionsByTaskId[taskId] ?? [];
      if (sessions.length === 0) return { diffStats: undefined, updatedAt: undefined };

      // Find the primary session or the most recent one
      const primarySession = sessions.find((s: TaskSession) => s.is_primary);
      const latestSession = primarySession ?? sessions[0];
      if (!latestSession) return { diffStats: undefined, updatedAt: undefined };

      const updatedAt = latestSession.updated_at;

      // Get git status for this session
      const gitStatus = gitStatusBySessionId[latestSession.id];
      if (!gitStatus?.files) return { diffStats: undefined, updatedAt };

      // Sum up additions and deletions from all files
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
    return allTasks.map((task: KanbanState['tasks'][number]) => {
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
  }, [repositoriesByWorkspace, allTasks, workspaceId, getSessionInfoForTask]);


  const updateUrl = useCallback((sessionId: string) => {
    if (typeof window === 'undefined') return;
    window.history.replaceState({}, '', linkToSession(sessionId));
  }, []);

  const loadTaskSessionsForTask = useCallback(
    async (taskId: string) => {
      const state = store.getState();
      if (state.taskSessionsByTask.loadedByTaskId[taskId]) {
        return state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
      }
      if (state.taskSessionsByTask.loadingByTaskId[taskId]) {
        return state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
      }
      setTaskSessionsLoading(taskId, true);
      try {
        const response = await listTaskSessions(taskId, { cache: 'no-store' });
        setTaskSessionsForTask(taskId, response.sessions ?? []);
        return response.sessions ?? [];
      } catch (error) {
        console.error('Failed to load task sessions:', error);
        setTaskSessionsForTask(taskId, []);
        return [];
      } finally {
        setTaskSessionsLoading(taskId, false);
      }
    },
    [setTaskSessionsForTask, setTaskSessionsLoading, store]
  );

  const handleDeleteTask = useCallback(
    async (taskId: string) => {
      setDeletingTaskId(taskId);
      try {
        await deleteTaskById(taskId);

        // Remove task from multi snapshots
        const currentSnapshots = store.getState().kanbanMulti.snapshots;
        for (const [wfId, snapshot] of Object.entries(currentSnapshots)) {
          const hadTask = snapshot.tasks.some((t: KanbanState['tasks'][number]) => t.id === taskId);
          if (hadTask) {
            store.getState().setWorkflowSnapshot(wfId, {
              ...snapshot,
              tasks: snapshot.tasks.filter((t: KanbanState['tasks'][number]) => t.id !== taskId),
            });
          }
        }

        // Also update single kanban state if task was there
        const currentKanbanTasks = store.getState().kanban.tasks;
        if (currentKanbanTasks.some((t: KanbanState['tasks'][number]) => t.id === taskId)) {
          store.setState((state) => ({
            ...state,
            kanban: {
              ...state.kanban,
              tasks: state.kanban.tasks.filter((t: KanbanState['tasks'][number]) => t.id !== taskId),
            },
          }));
        }

        // Collect all remaining tasks across snapshots
        const allRemainingTasks: KanbanState['tasks'] = [];
        for (const snapshot of Object.values(store.getState().kanbanMulti.snapshots)) {
          allRemainingTasks.push(...snapshot.tasks);
        }

        // If deleted task was active, switch to another task or go home
        const currentActiveTaskId = store.getState().tasks.activeTaskId;
        if (currentActiveTaskId === taskId) {
          const oldSessionId = store.getState().tasks.activeSessionId;
          if (allRemainingTasks.length > 0) {
            const nextTask = allRemainingTasks[0];
            if (nextTask.primarySessionId) {
              setActiveSession(nextTask.id, nextTask.primarySessionId);
              performLayoutSwitch(oldSessionId, nextTask.primarySessionId);
              window.history.replaceState({}, '', linkToSession(nextTask.primarySessionId));
            } else {
              const sessions = await loadTaskSessionsForTask(nextTask.id);
              const sessionId = sessions[0]?.id ?? null;
              if (sessionId) {
                setActiveSession(nextTask.id, sessionId);
                performLayoutSwitch(oldSessionId, sessionId);
                window.history.replaceState({}, '', linkToSession(sessionId));
              } else {
                setActiveTask(nextTask.id);
              }
            }
          } else {
            window.location.href = '/';
          }
        }
      } finally {
        setDeletingTaskId(null);
      }
    },
    [deleteTaskById, store, setActiveSession, setActiveTask, loadTaskSessionsForTask]
  );

  const handleSelectTask = useCallback(
    (taskId: string) => {
      const oldSessionId = store.getState().tasks.activeSessionId;
      // Search for task across all workflow snapshots
      let task: KanbanState['tasks'][number] | undefined;
      for (const snapshot of Object.values(store.getState().kanbanMulti.snapshots)) {
        task = snapshot.tasks.find((t: KanbanState['tasks'][number]) => t.id === taskId);
        if (task) break;
      }

      // If task has primarySessionId, switch immediately (instant UX)
      if (task?.primarySessionId) {
        setActiveSession(taskId, task.primarySessionId);
        performLayoutSwitch(oldSessionId, task.primarySessionId);
        updateUrl(task.primarySessionId);
        // Load sessions in background to update cache (non-blocking)
        loadTaskSessionsForTask(taskId);
        return;
      }

      // Fallback: load sessions first if no primarySessionId
      loadTaskSessionsForTask(taskId).then((sessions) => {
        const currentOldSessionId = store.getState().tasks.activeSessionId;
        const sessionId = sessions[0]?.id ?? null;
        if (!sessionId) {
          setActiveTask(taskId);
          return;
        }
        setActiveSession(taskId, sessionId);
        performLayoutSwitch(currentOldSessionId, sessionId);
        updateUrl(sessionId);
      });
    },
    [loadTaskSessionsForTask, setActiveSession, setActiveTask, updateUrl, store]
  );

  const taskSwitcherContent = (
    <>
      <TaskSwitcher
        tasks={tasksWithRepositories}
        steps={allSteps.map((step) => ({ id: step.id, title: step.title, color: step.color }))}
        activeTaskId={activeTaskId}
        selectedTaskId={selectedTaskId}
        onSelectTask={(taskId) => {
          handleSelectTask(taskId);
        }}
        onDeleteTask={handleDeleteTask}
        deletingTaskId={deletingTaskId}
        isLoading={isLoadingWorkflow}
      />
    </>
  );

  return (
    <PanelRoot>
      <PanelBody className="space-y-4 p-0">
        {taskSwitcherContent}
      </PanelBody>
    </PanelRoot>
  );
});
