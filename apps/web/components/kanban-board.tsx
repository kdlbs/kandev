'use client';

import { useEffect } from 'react';
import { Task } from './kanban-card';
import { TaskCreateDialog } from './task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import type { Task as BackendTask } from '@/lib/types/http';
import type { BoardState } from '@/lib/state/slices';
import { useDragAndDrop } from '@/hooks/use-drag-and-drop';
import { KanbanBoardGrid } from './kanban-board-grid';
import { KanbanHeader } from './kanban/kanban-header';
import { useKanbanData, useKanbanActions, useKanbanNavigation } from '@/hooks/domains/kanban';

interface KanbanBoardProps {
  onPreviewTask?: (task: Task) => void;
  onOpenTask?: (task: Task, sessionId: string) => void;
}

export function KanbanBoard({ onPreviewTask, onOpenTask }: KanbanBoardProps = {}) {
  // Store access
  const store = useAppStoreApi();

  // Get initial state for actions hook
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const boardsState = useAppStore((state) => state.boards);
  const setActiveBoard = useAppStore((state) => state.setActiveBoard);
  const setBoards = useAppStore((state) => state.setBoards);

  // Consolidated actions hook
  const {
    isDialogOpen,
    editingTask,
    setIsDialogOpen,
    setEditingTask,
    handleCreate,
    handleEdit,
    handleDelete,
    handleDialogOpenChange,
    handleDialogSuccess,
    handleWorkspaceChange,
    handleBoardChange,
  } = useKanbanActions({ kanban, workspaceState, boardsState });

  // Data fetching and derived state
  const {
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    activeColumns,
    visibleTasksWithSessions,
    isMounted,
    setTaskSessionAvailability,
  } = useKanbanData({
    onWorkspaceChange: handleWorkspaceChange,
    onBoardChange: handleBoardChange,
  });

  // Navigation hook
  const { handleOpenTask, handleCardClick } = useKanbanNavigation({
    enablePreviewOnClick,
    onPreviewTask,
    onOpenTask,
    setEditingTask,
    setIsDialogOpen,
    setTaskSessionAvailability,
  });

  // Drag and drop
  const { activeTask, handleDragStart, handleDragEnd, handleDragCancel } = useDragAndDrop(visibleTasksWithSessions);

  // Board selection effect
  useEffect(() => {
    const workspaceId = workspaceState.activeId;
    if (!workspaceId) {
      if (boardsState.items.length || boardsState.activeId) {
        setBoards([]);
        setActiveBoard(null);
      }
      return;
    }
    const workspaceBoards = boardsState.items.filter((board: BoardState['items'][number]) => board.workspaceId === workspaceId);
    const desiredBoardId =
      (userSettings.boardId && workspaceBoards.some((board: BoardState['items'][number]) => board.id === userSettings.boardId)
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
      <KanbanHeader onCreateTask={handleCreate} />
      <TaskCreateDialog
        key={isDialogOpen ? 'open' : 'closed'}
        open={isDialogOpen}
        onOpenChange={handleDialogOpenChange}
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
                repositoryId: editingTask.repositoryId,
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
                repositoryId: editingTask.repositoryId,
              }
            : undefined
        }
        submitLabel={editingTask ? 'Update' : 'Create'}
      />
      <KanbanBoardGrid
        columns={activeColumns}
        tasks={visibleTasksWithSessions}
        onPreviewTask={handleCardClick}
        onOpenTask={handleOpenTask}
        onEditTask={handleEdit}
        onDeleteTask={handleDelete}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
        activeTask={activeTask}
        showMaximizeButton={enablePreviewOnClick}
      />
    </div>
  );
}
