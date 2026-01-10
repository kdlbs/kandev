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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

const VIEWS: Record<
  string,
  {
    label: string;
    columns: Column[];
  }
> = {
  team: {
    label: 'Team view',
    columns: [
      { id: 'backlog', title: 'Backlog', color: 'bg-neutral-400' },
      { id: 'solution-design', title: 'Solution Design', color: 'bg-sky-500' },
      { id: 'ready-for-dev', title: 'Ready for Dev', color: 'bg-indigo-500' },
      { id: 'in-progress', title: 'In Progress', color: 'bg-blue-500' },
      { id: 'review', title: 'Review', color: 'bg-yellow-500' },
      { id: 'done', title: 'Done', color: 'bg-green-500' },
    ],
  },
  user: {
    label: 'User view',
    columns: [
      { id: 'todo', title: 'To Do', color: 'bg-neutral-400' },
      { id: 'in-progress', title: 'In Progress', color: 'bg-blue-500' },
      { id: 'review', title: 'Review', color: 'bg-yellow-500' },
      { id: 'done', title: 'Done', color: 'bg-green-500' },
    ],
  },
  architect: {
    label: 'Architect view',
    columns: [
      { id: 'backlog', title: 'Backlog', color: 'bg-neutral-400' },
      { id: 'high-level-design', title: 'High Level Design', color: 'bg-cyan-500' },
      { id: 'low-level-design', title: 'Low Level Design', color: 'bg-violet-500' },
      { id: 'review', title: 'Review', color: 'bg-yellow-500' },
      { id: 'done', title: 'Done', color: 'bg-green-500' },
    ],
  },
};

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
  const [activeViewId, setActiveViewId] = useState('team');
  const [editingTaskId, setEditingTaskId] = useState<string | null>(null);

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
  const editingTask = useMemo(
    () => tasks.find((task) => task.id === editingTaskId) ?? null,
    [tasks, editingTaskId]
  );
  const activeView = VIEWS[activeViewId] ?? VIEWS.team;
  const activeColumns = activeView.columns;

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

  const handleDialogSubmit = (title: string, description?: string) => {
    if (editingTaskId) {
      setTasks((tasks) =>
        tasks.map((task) =>
          task.id === editingTaskId ? { ...task, title, description } : task
        )
      );
      setEditingTaskId(null);
      return;
    }

    const columnId = activeColumns[0]?.id ?? 'todo';
    const newTask: Task = {
      id: crypto.randomUUID(),
      title,
      status: columnId,
      description,
    };
    setTasks((tasks) => [...tasks, newTask]);
  };

  const handleEditTask = (task: Task) => {
    setEditingTaskId(task.id);
    setIsDialogOpen(true);
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
        <h1 className="text-2xl font-bold">KanDev.ai</h1>
        <div className="flex items-center gap-3">
          <Select value={activeViewId} onValueChange={setActiveViewId}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Select view" />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(VIEWS).map(([id, view]) => (
                <SelectItem key={id} value={id}>
                  {view.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            onClick={() => {
              setEditingTaskId(null);
              setIsDialogOpen(true);
            }}
          >
            <IconPlus className="h-4 w-4" />
            Add task
          </Button>
          <ThemeToggle />
        </div>
      </header>
      <TaskCreateDialog
        open={isDialogOpen}
        onOpenChange={(open) => {
          setIsDialogOpen(open);
          if (!open) {
            setEditingTaskId(null);
          }
        }}
        onSubmit={(title, description) => handleDialogSubmit(title, description)}
        initialValues={
          editingTask
            ? { title: editingTask.title, description: editingTask.description }
            : undefined
        }
        submitLabel={editingTask ? 'Save' : 'Create'}
      />
      <DndContext
        sensors={sensors}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
      >
        <div className="flex-1 min-h-0 px-6 pb-6">
          <div
            className="grid gap-px bg-border rounded-lg overflow-hidden h-full"
            style={{ gridTemplateColumns: `repeat(${activeColumns.length}, minmax(0, 1fr))` }}
          >
            {activeColumns.map((column) => (
              <KanbanColumn
                key={column.id}
                column={column}
                tasks={getTasksForColumn(column.id)}
                onEditTask={handleEditTask}
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
