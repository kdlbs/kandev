'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { TaskState } from '@/lib/types/http';
import { TaskSwitcher } from './task-switcher';
import { Button } from '@kandev/ui/button';
import { IconPlus } from '@tabler/icons-react';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { linkToSession } from '@/lib/links';
import { listTaskSessions } from '@/lib/http';
import { useTasks } from '@/hooks/use-tasks';

type TaskSummary = {
  id: string;
  title: string;
  state?: TaskState;
  description?: string;
  columnId?: string;
  repositoryPath?: string;
};

type TaskSessionSidebarProps = {
  workspaceId: string | null;
  boardId: string | null;
};

export function TaskSessionSidebar({ workspaceId, boardId }: TaskSessionSidebarProps) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionsById = useAppStore((state) => state.taskSessions.items);
  const { tasks } = useTasks(boardId);
  const columns = useAppStore((state) => state.kanban.columns);
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
  const [taskDialogOpen, setTaskDialogOpen] = useState(false);
  const store = useAppStoreApi();

  const workspaceName = useMemo(() => {
    if (!workspaceId) return 'Workspace';
    return workspaces.find((workspace) => workspace.id === workspaceId)?.name ?? 'Workspace';
  }, [workspaceId, workspaces]);

  const tasksWithRepositories = useMemo(() => {
    const repositories = workspaceId ? repositoriesByWorkspace[workspaceId] ?? [] : [];
    const repositoryPathsById = new Map(repositories.map((repo) => [repo.id, repo.local_path]));
    return tasks.map((task) => ({
      id: task.id,
      title: task.title,
      state: task.state as TaskState | undefined,
      description: task.description,
      columnId: task.columnId,
      repositoryPath: task.repositoryId ? repositoryPathsById.get(task.repositoryId) : undefined,
    }));
  }, [repositoriesByWorkspace, tasks, workspaceId]);

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
    async (taskId: string) => {
      const sessions = await loadTaskSessionsForTask(taskId);
      const sessionId = sessions[0]?.id ?? null;
      if (!sessionId) {
        setActiveTask(taskId);
        return;
      }
      setActiveSession(taskId, sessionId);
      updateUrl(sessionId);
    },
    [loadTaskSessionsForTask, setActiveSession, setActiveTask, updateUrl]
  );


  return (
    <>
      <div className="h-full min-h-0 min-w-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-r-0 mr-[5px]">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium truncate text-muted-foreground">{workspaceName || 'Workspace'}</span>
          <Button
            size="sm"
            variant="outline"
            className="h-6 gap-1 cursor-pointer"
            onClick={() => setTaskDialogOpen(true)}
          >
            <IconPlus className="h-4 w-4" />
            Task
          </Button>
        </div>
        <div className="flex-1 min-h-0 overflow-y-auto space-y-4 pt-3">
          <TaskSwitcher
            tasks={tasksWithRepositories}
            columns={columns.map((column) => ({ id: column.id, title: column.title }))}
            activeTaskId={activeTaskId}
            selectedTaskId={selectedTaskId}
            onSelectTask={(taskId) => {
              handleSelectTask(taskId);
            }}
          />
        </div>
      </div>
      <TaskCreateDialog
        open={taskDialogOpen}
        onOpenChange={setTaskDialogOpen}
        mode="task"
        workspaceId={workspaceId}
        boardId={boardId}
        defaultColumnId={columns[0]?.id ?? null}
        columns={columns.map((column) => ({ id: column.id, title: column.title }))}
        onSuccess={(task, _mode, meta) => {
          store.setState((state) => {
            if (state.kanban.boardId !== task.board_id) return state;
            const nextTask = {
              id: task.id,
              columnId: task.column_id,
              title: task.title,
              description: task.description,
              position: task.position ?? 0,
              state: task.state,
              repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
            };
            return {
              ...state,
              kanban: {
                ...state.kanban,
                tasks: state.kanban.tasks.some((item) => item.id === task.id)
                  ? state.kanban.tasks.map((item) => (item.id === task.id ? nextTask : item))
                  : [...state.kanban.tasks, nextTask],
              },
            };
          });
          setActiveTask(task.id);
          if (meta?.taskSessionId) {
            setActiveSession(task.id, meta.taskSessionId);
            updateUrl(meta.taskSessionId);
          }
        }}
      />
    </>
  );
}
