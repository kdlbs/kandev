'use client';

import { useEffect, useMemo, useState } from 'react';
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
import { ThemeToggle } from './theme-toggle';
import { Button } from '@/components/ui/button';
import { IconPlus } from '@tabler/icons-react';
import { TaskCreateDialog } from './task-create-dialog';

type ColumnId = 'todo' | 'in-progress' | 'in-review' | 'done';

const COLUMNS: Column[] = [
  { id: 'todo', title: 'To Do', color: 'bg-neutral-400' },
  { id: 'in-progress', title: 'In Progress', color: 'bg-blue-500' },
  { id: 'in-review', title: 'In Review', color: 'bg-yellow-500' },
  { id: 'done', title: 'Done', color: 'bg-green-500' },
];

const initialTasks: Task[] = [
  { id: '1', title: 'Design database schema', status: 'todo' },
  { id: '2', title: 'Setup agent container environment', status: 'in-progress' },
  { id: '3', title: 'Implement WebSocket connection', status: 'todo' },
];

export function KanbanBoard() {
  const [tasks, setTasks] = useState<Task[]>(initialTasks);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);
  const [isMounted, setIsMounted] = useState(false);

  useEffect(() => {
    setIsMounted(true);
  }, []);

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  );

  const activeTask = useMemo(
    () => tasks.find((task) => task.id === activeTaskId) ?? null,
    [tasks, activeTaskId]
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

    setTasks((tasks) =>
      tasks.map((task) =>
        task.id === taskId ? { ...task, status: newStatus } : task
      )
    );
  };

  const handleDragCancel = () => {
    setActiveTaskId(null);
  };

  const handleAddTask = (columnId: string, title: string, description?: string) => {
    const newTask: Task = {
      id: crypto.randomUUID(),
      title,
      status: columnId,
      description,
    };
    setTasks((tasks) => [...tasks, newTask]);
  };

  const getTasksForColumn = (columnId: string) => {
    return tasks.filter((task) => task.status === columnId);
  };

  if (!isMounted) {
    return <div className="h-screen w-full bg-background" />;
  }

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <header className="flex items-center justify-between p-6 pb-4">
        <h1 className="text-2xl font-bold">Kanban Board</h1>
        <div className="flex items-center gap-2">
          <Button onClick={() => setIsDialogOpen(true)}>
            <IconPlus className="h-4 w-4" />
            Add task
          </Button>
          <ThemeToggle />
        </div>
      </header>
      <TaskCreateDialog
        open={isDialogOpen}
        onOpenChange={setIsDialogOpen}
        onSubmit={(title, description) => handleAddTask('todo', title, description)}
      />
      <DndContext
        sensors={sensors}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
      >
        <div className="flex-1 overflow-x-auto px-6 pb-6">
          <div className="inline-grid grid-flow-col auto-cols-[minmax(280px,360px)] gap-4">
            {COLUMNS.map((column) => (
              <KanbanColumn
                key={column.id}
                column={column}
                tasks={getTasksForColumn(column.id)}
              />
            ))}
          </div>
        </div>
        <DragOverlay dropAnimation={null}>
          {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
        </DragOverlay>
      </DndContext>
    </div>
  );
}
