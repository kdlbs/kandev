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
import { BoardSwitcher } from '../board-switcher';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { linkToSession } from '@/lib/links';
import { listTaskSessions, fetchBoardSnapshot, listBoards } from '@/lib/api';
import { useTasks } from '@/hooks/use-tasks';
import { useTaskActions } from '@/hooks/use-task-actions';
import type { TaskState, Workspace, Repository, TaskSession, BoardSnapshot, Task } from '@/lib/types/http';
import type { KanbanState } from '@/lib/state/slices';

type SessionTaskSwitcherSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
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
  boardId,
}: SessionTaskSwitcherSheetProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);

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
  const store = useAppStoreApi();

  const selectedTaskId = useMemo(() => {
    if (activeSessionId) {
      return sessionsById[activeSessionId]?.task_id ?? activeTaskId;
    }
    return activeTaskId;
  }, [activeSessionId, activeTaskId, sessionsById]);

  const kanbanIsLoading = useAppStore((state) => state.kanban.isLoading ?? false);
  const { deleteTaskById } = useTaskActions();

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

  const handleDeleteTask = useCallback(
    async (taskId: string) => {
      setDeletingTaskId(taskId);
      try {
        await deleteTaskById(taskId);

        const currentTasks = store.getState().kanban.tasks;
        const remainingTasks = currentTasks.filter(
          (item: KanbanState['tasks'][number]) => item.id !== taskId
        );

        store.setState((state) => ({
          ...state,
          kanban: {
            ...state.kanban,
            tasks: remainingTasks,
          },
        }));

        const currentActiveTaskId = store.getState().tasks.activeTaskId;
        if (currentActiveTaskId === taskId) {
          if (remainingTasks.length > 0) {
            const nextTask = remainingTasks[0];
            if (nextTask.primarySessionId) {
              setActiveSession(nextTask.id, nextTask.primarySessionId);
              window.history.replaceState({}, '', linkToSession(nextTask.primarySessionId));
            } else {
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
            window.location.href = '/';
          }
        }
      } finally {
        setDeletingTaskId(null);
      }
    },
    [deleteTaskById, store, setActiveSession, setActiveTask, loadTaskSessionsForTask]
  );

  const handleBoardChange = useCallback(
    async (newBoardId: string) => {
      if (newBoardId === boardId) return;

      store.setState((state) => ({
        ...state,
        kanban: { ...state.kanban, isLoading: true },
      }));

      try {
        const snapshot = await fetchBoardSnapshot(newBoardId);

        store.setState((state) => ({
          ...state,
          kanban: mapSnapshotToKanban(snapshot, newBoardId),
          boards: { ...state.boards, activeId: newBoardId },
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
        console.error('Failed to switch board:', error);
        store.setState((state) => ({
          ...state,
          kanban: { ...state.kanban, isLoading: false },
        }));
      }
    },
    [boardId, store, loadTaskSessionsForTask, setActiveSession, setActiveTask, updateUrl, onOpenChange]
  );

  const handleWorkspaceChange = useCallback(
    async (newWorkspaceId: string) => {
      if (newWorkspaceId === workspaceId) return;

      store.setState((state) => ({
        ...state,
        kanban: { ...state.kanban, isLoading: true },
      }));

      try {
        const boardsResponse = await listBoards(newWorkspaceId, { cache: 'no-store' });
        const newWorkspaceBoards = boardsResponse.boards ?? [];

        const firstBoard = newWorkspaceBoards[0];
        if (!firstBoard) {
          store.setState((state) => ({
            ...state,
            kanban: { ...state.kanban, isLoading: false },
          }));
          return;
        }

        const snapshot = await fetchBoardSnapshot(firstBoard.id);

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

  const dialogColumns = useMemo(
    () => columns.map((step: KanbanState['steps'][number]) => ({
      id: step.id,
      title: step.title,
      color: step.color,
      autoStartAgent: step.autoStartAgent,
    })),
    [columns]
  );

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
            columns={columns.map((step: KanbanState['steps'][number]) => ({ id: step.id, title: step.title, color: step.color }))}
            activeTaskId={activeTaskId}
            selectedTaskId={selectedTaskId}
            onSelectTask={handleSelectTask}
            onDeleteTask={handleDeleteTask}
            deletingTaskId={deletingTaskId}
            isLoading={kanbanIsLoading}
          />
        </div>

        {/* Board Switcher at bottom */}
        {workspaceBoards.length > 1 && (
          <div className="p-2 border-t border-border">
            <BoardSwitcher
              boards={workspaceBoards.map((b: { id: string; name: string }) => ({ id: b.id, name: b.name }))}
              activeBoardId={boardId}
              onSelect={handleBoardChange}
            />
          </div>
        )}
      </SheetContent>

      <TaskCreateDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        mode="create"
        workspaceId={workspaceId}
        boardId={boardId}
        defaultColumnId={dialogColumns[0]?.id ?? null}
        columns={dialogColumns}
        onSuccess={handleTaskCreated}
      />
    </Sheet>
  );
});
