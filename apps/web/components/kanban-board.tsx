'use client';

import { useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import { KanbanColumn, Column } from './kanban-column';
import { KanbanCardPreview, Task } from './kanban-card';
import { ConnectionStatus } from './connection-status';
import { Button } from '@/components/ui/button';
import { IconPlus, IconSettings } from '@tabler/icons-react';
import { TaskCreateDialog } from './task-create-dialog';
import Link from 'next/link';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useRouter } from 'next/navigation';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { ListBoardsResponse } from '@/lib/types/http';

const initialTasks: Task[] = [
  { id: '1', title: 'Design database schema', status: 'todo' },
  { id: '2', title: 'Setup agent container environment', status: 'in-progress' },
  { id: '3', title: 'Implement WebSocket connection', status: 'todo' },
];

export function KanbanBoard() {
  const [tasks, setTasks] = useState<Task[]>(initialTasks);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const router = useRouter();
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  const boardsState = useAppStore((state) => state.boards);
  const setActiveBoard = useAppStore((state) => state.setActiveBoard);
  const setBoards = useAppStore((state) => state.setBoards);
  const store = useAppStoreApi();

  const isMounted = useSyncExternalStore(
    () => () => { },
    () => true,
    () => false
  );

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
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
        status: task.columnId,
      })),
    [kanban.tasks]
  );
  const activeColumns = kanban.boardId ? backendColumns : [];
  const visibleTasks = kanban.boardId ? backendTasks : tasks;
  const activeTask = useMemo(
    () => visibleTasks.find((task) => task.id === activeTaskId) ?? null,
    [visibleTasks, activeTaskId]
  );

  const handleDragStart = (event: DragStartEvent) => {
    setActiveTaskId(event.active.id as string);
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;

    setActiveTaskId(null);
    if (!over) return;

    const taskId = active.id as string;
    const newStatus = over.id as string;

    if (kanban.boardId) {
      store.getState().hydrate({
        kanban: {
          ...kanban,
          tasks: kanban.tasks.map((task) =>
            task.id === taskId ? { ...task, columnId: newStatus } : task
          ),
        },
      });
      return;
    }

    setTasks((tasks) =>
      tasks.map((task) =>
        task.id === taskId ? { ...task, status: newStatus } : task
      )
    );
  };

  const handleDragCancel = () => {
    setActiveTaskId(null);
  };

  const handleDialogSubmit = (title: string, description?: string) => {
    const columnId = activeColumns[0]?.id ?? 'todo';
    const newTask: Task = {
      id: crypto.randomUUID(),
      title,
      status: columnId,
      description,
    };
    setTasks((tasks) => [...tasks, newTask]);
  };

  const handleOpenTask = (task: Task) => {
    router.push(`/task/${task.id}`);
  };

  const getTasksForColumn = (columnId: string) => {
    return visibleTasks.filter((task) => task.status === columnId);
  };

  useEffect(() => {
    const workspaceId = workspaceState.activeId;
    if (!workspaceId) return;
    const client = getWebSocketClient();
    if (!client) return;
    client
      .request<ListBoardsResponse>('board.list', { workspace_id: workspaceId })
      .then((response) => {
        const boards = response.boards.map((board) => ({
          id: board.id,
          workspaceId: board.workspace_id,
          name: board.name,
        }));
        setBoards(boards);
        const nextBoardId = boards[0]?.id ?? null;
        setActiveBoard(nextBoardId);
        if (!nextBoardId) {
          store.getState().hydrate({
            kanban: { boardId: null, columns: [], tasks: [] },
          });
        }
      })
      .catch(() => {
        // Ignore board list errors for now.
      });
  }, [setActiveBoard, setBoards, store, workspaceState.activeId]);

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
          <Select
            value={workspaceState.activeId ?? ''}
            onValueChange={(value) => {
              setActiveWorkspace(value || null);
              if (value) {
                router.push(`/?workspaceId=${value}`);
              }
            }}
          >
            <SelectTrigger className="w-[200px]">
              <SelectValue placeholder="Select workspace" />
            </SelectTrigger>
            <SelectContent>
              {workspaceState.items.map((workspace) => (
                <SelectItem key={workspace.id} value={workspace.id}>
                  {workspace.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-3">
          <Select
            value={boardsState.activeId ?? ''}
            onValueChange={(value) => {
              setActiveBoard(value || null);
              if (value) {
                const workspaceId = boardsState.items.find((board) => board.id === value)?.workspaceId;
                const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : '';
                router.push(`/?boardId=${value}${workspaceParam}`);
              }
            }}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Select board" />
            </SelectTrigger>
            <SelectContent>
              {boardsState.items.map((board) => (
                <SelectItem key={board.id} value={board.id}>
                  {board.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button onClick={() => setIsDialogOpen(true)} className="cursor-pointer">
            <IconPlus className="h-4 w-4" />
            Add task
          </Button>
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
        onOpenChange={(open) => {
          setIsDialogOpen(open);
        }}
        onSubmit={(title, description) => handleDialogSubmit(title, description)}
      />
      <DndContext
        sensors={sensors}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
      >
        <div className="flex-1 min-h-0 px-4 pb-4">
          {activeColumns.length === 0 ? (
            <div className="h-full rounded-lg border border-dashed border-border/60 flex items-center justify-center text-sm text-muted-foreground">
              No boards available yet.
            </div>
          ) : (
            <div
              className="grid gap-[3px] rounded-lg overflow-hidden h-full p-[2px]"
              style={{ gridTemplateColumns: `repeat(${activeColumns.length}, minmax(0, 1fr))` }}
            >
              {activeColumns.map((column) => (
                <KanbanColumn
                  key={column.id}
                  column={column}
                  tasks={getTasksForColumn(column.id)}
                  onOpenTask={handleOpenTask}
                />
              ))}
            </div>
          )}
        </div>
        <DragOverlay dropAnimation={null}>
          {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
        </DragOverlay>
      </DndContext>
    </div>
  );
}
