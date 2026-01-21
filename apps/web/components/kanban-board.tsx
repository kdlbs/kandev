'use client';

import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from 'react';
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
import { useDragAndDrop } from '@/hooks/use-drag-and-drop';
import { useTaskCRUD } from '@/hooks/use-task-crud';
import { KanbanBoardGrid } from './kanban-board-grid';
import { getWebSocketClient } from '@/lib/ws/connection';
import { linkToSession } from '@/lib/links';
import Link from 'next/link';
import { Button } from '@kandev/ui/button';
import { IconPlus, IconSettings } from '@tabler/icons-react';
import { ConnectionStatus } from './connection-status';
import { KanbanDisplayDropdown } from './kanban-display-dropdown';

interface KanbanBoardProps {
  onPreviewTask?: (task: Task) => void;
  onOpenTask?: (task: Task, sessionId: string) => void;
}

export function KanbanBoard({ onPreviewTask, onOpenTask }: KanbanBoardProps = {}) {
  const [taskSessionAvailability, setTaskSessionAvailability] = useState<Record<string, boolean>>({});
  const router = useRouter();
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  const boardsState = useAppStore((state) => state.boards);
  const setActiveBoard = useAppStore((state) => state.setActiveBoard);
  const setBoards = useAppStore((state) => state.setBoards);
  const store = useAppStoreApi();

  // Get preview setting from store
  const enablePreviewOnClick = useAppStore((state) => state.userSettings.enablePreviewOnClick);

  useBoards(workspaceState.activeId, true);
  useBoardSnapshot(boardsState.activeId);

  const handleWorkspaceChange = useCallback(
    (nextWorkspaceId: string | null) => {
      if (nextWorkspaceId === workspaceState.activeId) {
        return;
      }
      setActiveWorkspace(nextWorkspaceId);
      if (nextWorkspaceId) {
        router.push(`/?workspaceId=${nextWorkspaceId}`);
      } else {
        router.push('/');
      }
    },
    [router, setActiveWorkspace, workspaceState.activeId]
  );

  const handleBoardChange = useCallback(
    (nextBoardId: string | null) => {
      if (nextBoardId === boardsState.activeId) {
        return;
      }
      setActiveBoard(nextBoardId);
      if (nextBoardId) {
        const workspaceId = boardsState.items.find((board) => board.id === nextBoardId)?.workspaceId;
        const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : '';
        router.push(`/?boardId=${nextBoardId}${workspaceParam}`);
      }
    },
    [boardsState.activeId, boardsState.items, router, setActiveBoard]
  );

  const stableWorkspaceId = workspaceState.activeId;
  const stableBoardId = boardsState.activeId;

  const {
    settings: userSettings,
    commitSettings,
    selectedRepositoryIds,
  } = useUserDisplaySettings({
    workspaceId: stableWorkspaceId,
    boardId: stableBoardId,
    onWorkspaceChange: handleWorkspaceChange,
    onBoardChange: handleBoardChange,
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

  // Use custom hooks for drag-and-drop and CRUD operations
  const { activeTask, handleDragStart, handleDragEnd, handleDragCancel } = useDragAndDrop(visibleTasksWithSessions);
  const { isDialogOpen, editingTask, handleCreate, handleEdit, handleDelete, handleDialogOpenChange, setIsDialogOpen, setEditingTask } = useTaskCRUD();

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

  const handleCardClick = async (task: Task) => {
    // Capture the current mode at the start to avoid race conditions
    const shouldOpenPreview = enablePreviewOnClick;

    if (shouldOpenPreview) {
      // Preview mode - just call the preview handler without fetching session
      if (onPreviewTask) {
        onPreviewTask(task);
      }
    } else {
      // Navigate mode - fetch session and navigate
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
      <header className="flex items-center justify-between p-4 pb-3">
        <div className="flex items-center gap-3">
          <Link href="/" className="text-2xl font-bold hover:opacity-80">
            KanDev.ai
          </Link>
          <ConnectionStatus />
        </div>
        <div className="flex items-center gap-3">
          <Button onClick={handleCreate} className="cursor-pointer">
            <IconPlus className="h-4 w-4" />
            Add task
          </Button>
          <KanbanDisplayDropdown />
          <Link href="/settings" className="cursor-pointer">
            <Button variant="outline" className="cursor-pointer">
              <IconSettings className="h-4 w-4 mr-2" />
              Settings
            </Button>
          </Link>
        </div>
      </header>
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
