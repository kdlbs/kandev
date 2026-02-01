'use client';

import { useCallback, useMemo, useState, memo } from 'react';
import type { TaskState, Workspace, Repository, TaskSession, BoardSnapshot, Task } from '@/lib/types/http';
import type { KanbanState } from '@/lib/state/slices';
import { TaskSwitcher } from './task-switcher';
import { BoardSwitcher } from './board-switcher';
import { WorkspaceSwitcher } from './workspace-switcher';
import { Button } from '@kandev/ui/button';
import { SessionPanel } from '@kandev/ui/pannel-session';
import { IconPlus } from '@tabler/icons-react';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { linkToSession } from '@/lib/links';
import { listTaskSessions, fetchBoardSnapshot, listBoards } from '@/lib/api';
import { useTasks } from '@/hooks/use-tasks';
import { useTaskActions } from '@/hooks/use-task-actions';

// Extracted component to isolate dialog state from sidebar
type NewTaskButtonProps = {
  workspaceId: string | null;
  boardId: string | null;
  columns: Array<{ id: string; title: string; color?: string; autoStartAgent?: boolean }>;
  onSuccess: (task: Task, mode: 'create' | 'edit', meta?: { taskSessionId?: string | null }) => void;
};

const NewTaskButton = memo(function NewTaskButton({
  workspaceId,
  boardId,
  columns,
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
        mode="task"
        workspaceId={workspaceId}
        boardId={boardId}
        defaultColumnId={columns[0]?.id ?? null}
        columns={columns}
        onSuccess={onSuccess}
      />
    </>
  );
});

type TaskSessionSidebarProps = {
  workspaceId: string | null;
  boardId: string | null;
};

// Helper to map board snapshot to kanban state
function mapSnapshotToKanban(snapshot: BoardSnapshot, newBoardId: string) {
  return {
    boardId: newBoardId,
    isLoading: false,
    steps: snapshot.steps.map((step) => ({
      id: step.id,
      title: step.name,
      color: step.color,
      position: step.position,
      autoStartAgent: step.auto_start_agent,
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

// Helper to sort by updated_at descending (most recent first)
function sortByUpdatedAtDesc<T extends { updated_at?: string | null }>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const aDate = a.updated_at ? new Date(a.updated_at).getTime() : 0;
    const bDate = b.updated_at ? new Date(b.updated_at).getTime() : 0;
    return bDate - aDate;
  });
}

export const TaskSessionSidebar = memo(function TaskSessionSidebar({ workspaceId, boardId }: TaskSessionSidebarProps) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const sessionsByTaskId = useAppStore((state) => state.taskSessionsByTask.itemsByTaskId);
  const gitStatusBySessionId = useAppStore((state) => state.gitStatus.bySessionId);
  const { tasks } = useTasks(boardId);
  const columns = useAppStore((state) => state.kanban.steps);
  const boards = useAppStore((state) => state.boards.items);
  const workspaces = useAppStore((state) => state.workspaces.items);
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
  const kanbanBoardId = useAppStore((state) => state.kanban.boardId);
  const kanbanIsLoading = useAppStore((state) => state.kanban.isLoading ?? false);
  // Consider loading if explicitly loading OR if board IDs mismatch (mid-switch)
  const isLoadingBoard = kanbanIsLoading || (boardId !== null && kanbanBoardId !== boardId);
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

  // Get boards for the current workspace
  const workspaceBoards = useMemo(() => {
    if (!workspaceId) return [];
    return boards.filter((board: { id: string; workspaceId: string; name: string }) => board.workspaceId === workspaceId);
  }, [boards, workspaceId]);

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
        // Use session's updatedAt if available, otherwise fall back to task's updatedAt
        updatedAt: sessionInfo.updatedAt ?? task.updatedAt,
      };
    });
  }, [repositoriesByWorkspace, tasks, workspaceId, getSessionInfoForTask]);

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

        // Get remaining tasks after filtering out deleted one
        const currentTasks = store.getState().kanban.tasks;
        const remainingTasks = currentTasks.filter(
          (item: KanbanState['tasks'][number]) => item.id !== taskId
        );

        // Update UI after successful delete
        store.setState((state) => ({
          ...state,
          kanban: {
            ...state.kanban,
            tasks: remainingTasks,
          },
        }));

        // If deleted task was active, switch to another task or go home
        const currentActiveTaskId = store.getState().tasks.activeTaskId;
        if (currentActiveTaskId === taskId) {
          if (remainingTasks.length > 0) {
            // Switch to the first remaining task
            const nextTask = remainingTasks[0];
            // Use handleSelectTask pattern - check for primarySessionId first
            if (nextTask.primarySessionId) {
              setActiveSession(nextTask.id, nextTask.primarySessionId);
              window.history.replaceState({}, '', linkToSession(nextTask.primarySessionId));
            } else {
              // Load sessions to find one to navigate to
              const sessions = await loadTaskSessionsForTask(nextTask.id);
              const sessionId = sessions[0]?.id ?? null;
              if (sessionId) {
                setActiveSession(nextTask.id, sessionId);
                window.history.replaceState({}, '', linkToSession(sessionId));
              } else {
                setActiveTask(nextTask.id);
              }
            }
          } else {
            // No tasks left, go to homepage
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
      // Get task from kanban state to check for primarySessionId
      const kanbanTasks = store.getState().kanban.tasks;
      const task = kanbanTasks.find((t) => t.id === taskId);

      // If task has primarySessionId, switch immediately (instant UX)
      if (task?.primarySessionId) {
        setActiveSession(taskId, task.primarySessionId);
        updateUrl(task.primarySessionId);
        // Load sessions in background to update cache (non-blocking)
        loadTaskSessionsForTask(taskId);
        return;
      }

      // Fallback: load sessions first if no primarySessionId
      loadTaskSessionsForTask(taskId).then((sessions) => {
        const sessionId = sessions[0]?.id ?? null;
        if (!sessionId) {
          setActiveTask(taskId);
          return;
        }
        setActiveSession(taskId, sessionId);
        updateUrl(sessionId);
      });
    },
    [loadTaskSessionsForTask, setActiveSession, setActiveTask, updateUrl, store]
  );

  // Handle board change - fetch new board data and navigate to most recent task's session
  const handleBoardChange = useCallback(
    async (newBoardId: string) => {
      if (newBoardId === boardId) return;

      // Set loading state in store (synchronous)
      store.setState((state) => ({
        ...state,
        kanban: { ...state.kanban, isLoading: true },
      }));

      try {
        const snapshot = await fetchBoardSnapshot(newBoardId);

        // Update the kanban state with the new board data
        store.setState((state) => ({
          ...state,
          kanban: mapSnapshotToKanban(snapshot, newBoardId),
          boards: { ...state.boards, activeId: newBoardId },
        }));

        // Navigate to the most recent task's most recent session
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
      } catch (error) {
        console.error('Failed to switch board:', error);
        store.setState((state) => ({
          ...state,
          kanban: { ...state.kanban, isLoading: false },
        }));
      }
    },
    [boardId, store, loadTaskSessionsForTask, setActiveSession, setActiveTask, updateUrl]
  );

  // Handle workspace change - find first board and navigate to most recent task's session
  // Memoized columns for NewTaskButton to prevent re-renders
  const dialogColumns = useMemo(
    () => columns.map((step: KanbanState['steps'][number]) => ({
      id: step.id,
      title: step.title,
      color: step.color,
      autoStartAgent: step.autoStartAgent,
    })),
    [columns]
  );

  // Memoized success handler for NewTaskButton
  const handleTaskCreated = useCallback(
    (task: Task, _mode: 'create' | 'edit', meta?: { taskSessionId?: string | null }) => {
      store.setState((state) => {
        if (state.kanban.boardId !== task.board_id) return state;
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
    },
    [store, setActiveTask, setActiveSession, updateUrl]
  );

  const handleWorkspaceChange = useCallback(
    async (newWorkspaceId: string) => {
      if (newWorkspaceId === workspaceId) return;

      // Set loading state in store (synchronous)
      store.setState((state) => ({
        ...state,
        kanban: { ...state.kanban, isLoading: true },
      }));

      try {
        // Fetch boards for the new workspace
        const boardsResponse = await listBoards(newWorkspaceId, { cache: 'no-store' });
        const newWorkspaceBoards = boardsResponse.boards ?? [];

        const firstBoard = newWorkspaceBoards[0];
        if (!firstBoard) {
          // No boards in workspace - reset loading state
          store.setState((state) => ({
            ...state,
            kanban: { ...state.kanban, isLoading: false },
          }));
          return;
        }

        const snapshot = await fetchBoardSnapshot(firstBoard.id);

        // Update boards and kanban state in a single setState call
        store.setState((state) => ({
          ...state,
          boards: {
            ...state.boards,
            items: [
              ...state.boards.items.filter((b: { workspaceId: string }) => b.workspaceId !== newWorkspaceId),
              ...newWorkspaceBoards.map((b) => ({ id: b.id, workspaceId: b.workspace_id, name: b.name })),
            ],
            activeId: firstBoard.id,
          },
          kanban: mapSnapshotToKanban(snapshot, firstBoard.id),
        }));

        // Navigate to the most recent task's most recent session
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
      } catch (error) {
        console.error('Failed to switch workspace:', error);
        store.setState((state) => ({
          ...state,
          kanban: { ...state.kanban, isLoading: false },
        }));
      }
    },
    [workspaceId, store, loadTaskSessionsForTask, setActiveSession, setActiveTask, updateUrl]
  );

  return (
    <>
      <SessionPanel borderSide="right" margin="right" className="min-w-0 bg-transparent border-0 rounded-none">
        <div className="flex items-center justify-between">
          <div className="pl-2">
            <WorkspaceSwitcher
              workspaces={workspaces.map((w: Workspace) => ({ id: w.id, name: w.name }))}
              activeWorkspaceId={workspaceId}
              onSelect={handleWorkspaceChange}
            />
          </div>
          <NewTaskButton
            workspaceId={workspaceId}
            boardId={boardId}
            columns={dialogColumns}
            onSuccess={handleTaskCreated}
          />
        </div>
        <div className="flex-1 min-h-0 overflow-y-auto space-y-4 pt-3">
          <TaskSwitcher
            tasks={tasksWithRepositories}
            columns={columns.map((step: KanbanState['steps'][number]) => ({ id: step.id, title: step.title, color: step.color }))}
            activeTaskId={activeTaskId}
            selectedTaskId={selectedTaskId}
            onSelectTask={(taskId) => {
              handleSelectTask(taskId);
            }}
            onDeleteTask={handleDeleteTask}
            deletingTaskId={deletingTaskId}
            isLoading={isLoadingBoard}
          />
        </div>
        {/* Board Switcher at bottom - only show if more than one board */}
        {workspaceBoards.length > 1 && (
          <div className="mt-auto pt-2 border-t border-border/30">
            <BoardSwitcher
              boards={workspaceBoards.map((b: { id: string; name: string }) => ({ id: b.id, name: b.name }))}
              activeBoardId={boardId}
              onSelect={handleBoardChange}
            />
          </div>
        )}
      </SessionPanel>
    </>
  );
});
