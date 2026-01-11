'use client';

import { useMemo, useState, useSyncExternalStore } from 'react';
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
  const [activeViewId, setActiveViewId] = useState('team');
  const router = useRouter();

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

  const activeTask = useMemo(
    () => tasks.find((task) => task.id === activeTaskId) ?? null,
    [tasks, activeTaskId]
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
    return tasks.filter((task) => task.status === columnId);
  };

  if (!isMounted) {
    return <div className="h-screen w-full bg-background" />;
  }

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <header className="flex items-center justify-between p-4 pb-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold">KanDev.ai</h1>
          <ConnectionStatus />
        </div>
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
        </div>
        <DragOverlay dropAnimation={null}>
          {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
        </DragOverlay>
      </DndContext>
    </div>
  );
}
