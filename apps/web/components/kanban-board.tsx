'use client';

import { useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import { DragEndEvent, DragStartEvent } from '@dnd-kit/core';
import { Column } from './kanban-column';
import { Task } from './kanban-card';
import { TaskCreateDialog } from './task-create-dialog';
import { useRouter } from 'next/navigation';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { Task as BackendTask } from '@/lib/types/http';
import { filterTasksByRepositories } from '@/lib/kanban/filters';
import { useUserDisplaySettings } from '@/hooks/use-user-display-settings';
import { useBoards } from '@/hooks/use-boards';
import { useBoardSnapshot } from '@/hooks/use-board-snapshot';
import { useTaskActions } from '@/hooks/use-task-actions';
import { KanbanBoardHeader } from './kanban-board-header';
import { KanbanBoardGrid } from './kanban-board-grid';
import { getWebSocketClient } from '@/lib/ws/connection';
import { linkToSession } from '@/lib/links';

interface KanbanBoardProps {
  onPreviewTask?: (task: Task) => void;
  onOpenTask?: (task: Task, sessionId: string) => void;
}

export function KanbanBoard({ onPreviewTask, onOpenTask }: KanbanBoardProps = {}) {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const [isMovingTask, setIsMovingTask] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
  const [taskSessionAvailability, setTaskSessionAvailability] = useState<Record<string, boolean>>({});
  const router = useRouter();
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  const boardsState = useAppStore((state) => state.boards);
  const setActiveBoard = useAppStore((state) => state.setActiveBoard);
  const setBoards = useAppStore((state) => state.setBoards);
  const store = useAppStoreApi();
  const { moveTaskById, deleteTaskById } = useTaskActions();
  useBoards(workspaceState.activeId, true);
  useBoardSnapshot(boardsState.activeId);
  const {
    settings: userSettings,
    commitSettings,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryIds,
  } = useUserDisplaySettings({
    workspaceId: workspaceState.activeId,
    boardId: boardsState.activeId,
    onWorkspaceChange: (nextWorkspaceId) => {
      setActiveWorkspace(nextWorkspaceId);
      if (nextWorkspaceId) {
        router.push(`/?workspaceId=${nextWorkspaceId}`);
      } else {
        router.push('/');
      }
    },
    onBoardChange: (nextBoardId) => {
      setActiveBoard(nextBoardId);
      if (nextBoardId) {
        const workspaceId = boardsState.items.find((board) => board.id === nextBoardId)?.workspaceId;
        const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : '';
        router.push(`/?boardId=${nextBoardId}${workspaceParam}`);
      }
    },
  });

  const isMounted = useSyncExternalStore(
    () => () => { },
    () => true,
    () => false
  );

  const backendColumns = useMemo<Column[]>(
    () =>
      [...kanban.columns]
        .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
        .map((column) => ({
          id: column.id,
          title: column.title,
          color: column.color || 'bg-neutral-400',
        })),
    [kanban.columns]
  );
  const backendTasks = useMemo<Task[]>(
    () =>
      kanban.tasks.map((task) => ({
        id: task.id,
        title: task.title,
        columnId: task.columnId,
        state: task.state,
        description: task.description,
        position: task.position,
        repositoryId: task.repositoryId,
      })),
    [kanban.tasks]
  );
  const activeColumns = kanban.boardId ? backendColumns : [];
  const visibleTasks = useMemo(
    () => filterTasksByRepositories(backendTasks, selectedRepositoryIds),
    [backendTasks, selectedRepositoryIds]
  );
  const visibleTasksWithSessions = useMemo(
    () => visibleTasks.map((task) => ({
      ...task,
      hasSession: taskSessionAvailability[task.id],
    })),
    [visibleTasks, taskSessionAvailability]
  );
  const activeTask = useMemo(
    () => visibleTasksWithSessions.find((task) => task.id === activeTaskId) ?? null,
    [visibleTasksWithSessions, activeTaskId]
  );

  const handleDragStart = (event: DragStartEvent) => {
    setActiveTaskId(event.active.id as string);
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;

    setActiveTaskId(null);
    if (!over) return;

    const taskId = active.id as string;
    const newStatus = over.id as string;

    if (!kanban.boardId || isMovingTask) {
      return;
    }

    const targetTasks = kanban.tasks
      .filter((task) => task.columnId === newStatus && task.id !== taskId)
      .sort((a, b) => a.position - b.position);
    const nextPosition = targetTasks.length;
    store.getState().hydrate({
      kanban: {
        ...kanban,
        tasks: kanban.tasks.map((task) =>
          task.id === taskId
            ? { ...task, columnId: newStatus, position: nextPosition }
            : task
        ),
      },
    });

    try {
      setIsMovingTask(true);
      await moveTaskById(taskId, {
        board_id: kanban.boardId,
        column_id: newStatus,
        position: nextPosition,
      });
    } catch {
      // Ignore move errors for now; WS updates or next snapshot will correct.
    } finally {
      setIsMovingTask(false);
    }
  };

  const handleDragCancel = () => {
    setActiveTaskId(null);
  };

  const handleDialogSuccess = (task: BackendTask, mode: 'create' | 'edit') => {
    if (mode === 'create') {
      store.getState().hydrate({
        kanban: {
          ...kanban,
          tasks: [
            ...kanban.tasks,
            {
              id: task.id,
              columnId: task.column_id,
              title: task.title,
              description: task.description ?? undefined,
              position: task.position ?? 0,
              state: task.state,
              repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
            },
          ],
        },
      });
      return;
    }
    store.getState().hydrate({
      kanban: {
        ...kanban,
        tasks: kanban.tasks.map((item) =>
          item.id === task.id
            ? {
                ...item,
                title: task.title,
                description: task.description ?? undefined,
                columnId: task.column_id ?? item.columnId,
                position: task.position ?? item.position,
                state: task.state ?? item.state,
                repositoryId: task.repositories?.[0]?.repository_id ?? item.repositoryId,
              }
            : item
        ),
      },
    });
  };

  const fetchLatestSessionId = async (taskId: string) => {
    const client = getWebSocketClient();
    if (!client) return null;
    try {
      const response = await client.request<{ sessions: Array<{ id: string }> }>(
        'task.session.list',
        { task_id: taskId },
        10000
      );
      setTaskSessionAvailability((prev) => ({
        ...prev,
        [taskId]: response.sessions.length > 0,
      }));
      return response.sessions[0]?.id ?? null;
    } catch (error) {
      console.error('Failed to load task sessions:', error);
      return null;
    }
  };

  const handleOpenTask = async (task: Task) => {
    const latestSessionId = await fetchLatestSessionId(task.id);
    if (!latestSessionId) {
      setEditingTask(task);
      setIsDialogOpen(true);
      return;
    }
    if (onOpenTask) {
      onOpenTask(task, latestSessionId);
    } else {
      router.push(linkToSession(latestSessionId));
    }
  };

  const handlePreviewTask = async (task: Task) => {
    const latestSessionId = await fetchLatestSessionId(task.id);
    if (!latestSessionId) {
      setEditingTask(task);
      setIsDialogOpen(true);
      return;
    }
    if (onPreviewTask) {
      onPreviewTask(task);
    } else {
      // Fallback to opening full page if no preview handler
      handleOpenTask(task);
    }
  };

  const handleEditTask = (task: Task) => {
    setEditingTask(task);
    setIsDialogOpen(true);
  };

  const handleDeleteTask = async (task: Task) => {
    if (!kanban.boardId) return;
    store.getState().hydrate({
      kanban: {
        ...kanban,
        tasks: kanban.tasks.filter((item) => item.id !== task.id),
      },
    });
    try {
      await deleteTaskById(task.id);
    } catch {
      // Ignore delete errors for now.
    }
  };


  useEffect(() => {
    const workspaceId = workspaceState.activeId;
    if (!workspaceId) {
      if (boardsState.items.length || boardsState.activeId) {
        setBoards([]);
        setActiveBoard(null);
      }
      return;
    }
    const workspaceBoards = boardsState.items.filter((board) => board.workspaceId === workspaceId);
    const desiredBoardId =
      (userSettings.boardId && workspaceBoards.some((board) => board.id === userSettings.boardId)
        ? userSettings.boardId
        : workspaceBoards[0]?.id) ?? null;
    setActiveBoard(desiredBoardId);
    if (userSettings.loaded && desiredBoardId !== userSettings.boardId) {
      commitSettings({
        workspaceId,
        boardId: desiredBoardId,
        repositoryIds: userSettings.repositoryIds,
      });
    }
    if (!desiredBoardId) {
      store.getState().hydrate({
        kanban: { boardId: null, columns: [], tasks: [] },
      });
    }
  }, [
    boardsState.activeId,
    boardsState.items,
    commitSettings,
    setActiveBoard,
    setBoards,
    store,
    userSettings.boardId,
    userSettings.loaded,
    userSettings.repositoryIds,
    workspaceState.activeId,
  ]);



  if (!isMounted) {
    return <div className="h-screen w-full bg-background" />;
  }

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <KanbanBoardHeader
        workspaces={workspaceState.items}
        boards={boardsState.items}
        activeWorkspaceId={workspaceState.activeId}
        activeBoardId={boardsState.activeId}
        repositories={repositories}
        repositoriesLoading={repositoriesLoading}
        allRepositoriesSelected={allRepositoriesSelected}
        selectedRepositoryId={userSettings.repositoryIds[0] ?? null}
        onWorkspaceChange={(nextWorkspaceId) => {
          setActiveWorkspace(nextWorkspaceId);
          if (nextWorkspaceId) {
            router.push(`/?workspaceId=${nextWorkspaceId}`);
          } else {
            router.push('/');
          }
          commitSettings({
            workspaceId: nextWorkspaceId,
            boardId: null,
            repositoryIds: [],
          });
        }}
        onBoardChange={(nextBoardId) => {
          setActiveBoard(nextBoardId);
          if (nextBoardId) {
            const workspaceId = boardsState.items.find((board) => board.id === nextBoardId)?.workspaceId;
            const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : '';
            router.push(`/?boardId=${nextBoardId}${workspaceParam}`);
          }
          commitSettings({
            workspaceId: userSettings.workspaceId,
            boardId: nextBoardId,
            repositoryIds: userSettings.repositoryIds,
          });
        }}
        onRepositoryChange={(value) => {
          if (value === 'all') {
            commitSettings({
              workspaceId: userSettings.workspaceId,
              boardId: userSettings.boardId,
              repositoryIds: [],
            });
            return;
          }
          commitSettings({
            workspaceId: userSettings.workspaceId,
            boardId: userSettings.boardId,
            repositoryIds: [value],
          });
        }}
        onAddTask={() => {
          setEditingTask(null);
          setIsDialogOpen(true);
        }}
      />
      <TaskCreateDialog
        key={isDialogOpen ? 'open' : 'closed'}
        open={isDialogOpen}
        onOpenChange={(open) => {
          setIsDialogOpen(open);
          if (!open) {
            setEditingTask(null);
          }
        }}
        workspaceId={workspaceState.activeId}
        boardId={kanban.boardId}
        defaultColumnId={activeColumns[0]?.id ?? null}
        columns={activeColumns.map((column) => ({ id: column.id, title: column.title }))}
        editingTask={
          editingTask
            ? {
                id: editingTask.id,
                title: editingTask.title,
                description: editingTask.description,
                columnId: editingTask.columnId,
                state: editingTask.state as BackendTask['state'],
              }
            : null
        }
        onSuccess={handleDialogSuccess}
        initialValues={
          editingTask
            ? {
                title: editingTask.title,
                description: editingTask.description,
                state: editingTask.state as BackendTask['state'],
              }
            : undefined
        }
        submitLabel={editingTask ? 'Update' : 'Create'}
      />
      <KanbanBoardGrid
        columns={activeColumns}
        tasks={visibleTasksWithSessions}
        onPreviewTask={handlePreviewTask}
        onOpenTask={handleOpenTask}
        onEditTask={handleEditTask}
        onDeleteTask={handleDeleteTask}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
        activeTask={activeTask}
      />
    </div>
  );
}
