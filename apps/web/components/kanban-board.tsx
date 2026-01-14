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

export function KanbanBoard() {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const [isMovingTask, setIsMovingTask] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
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
    selectedRepositoryPaths,
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
        repositoryUrl: task.repositoryUrl,
      })),
    [kanban.tasks]
  );
  const activeColumns = kanban.boardId ? backendColumns : [];
  const visibleTasks = useMemo(
    () => filterTasksByRepositories(backendTasks, selectedRepositoryPaths),
    [backendTasks, selectedRepositoryPaths]
  );
  const activeTask = useMemo(
    () => visibleTasks.find((task) => task.id === activeTaskId) ?? null,
    [visibleTasks, activeTaskId]
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
              repositoryUrl: task.repository_url ?? undefined,
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
                repositoryUrl: task.repository_url ?? item.repositoryUrl,
              }
            : item
        ),
      },
    });
  };

  const handleOpenTask = (task: Task) => {
    router.push(`/task/${task.id}`);
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
      setBoards([]);
      setActiveBoard(null);
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
        tasks={visibleTasks}
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
