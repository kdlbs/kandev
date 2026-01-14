'use client';

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

export type KanbanBoardGridProps = {
  columns: Column[];
  tasks: Task[];
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onDragStart: (event: DragStartEvent) => void;
  onDragEnd: (event: DragEndEvent) => void;
  onDragCancel: () => void;
  activeTask: Task | null;
};

export function KanbanBoardGrid({
  columns,
  tasks,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onDragStart,
  onDragEnd,
  onDragCancel,
  activeTask,
}: KanbanBoardGridProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  );

  const getTasksForColumn = (columnId: string) => {
    return tasks
      .filter((task) => task.columnId === columnId)
      .map((task) => ({ ...task, position: task.position ?? 0 }))
      .sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
  };

  return (
    <DndContext
      sensors={sensors}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragCancel={onDragCancel}
    >
      <div className="flex-1 min-h-0 px-4 pb-4">
        {columns.length === 0 ? (
          <div className="h-full rounded-lg border border-dashed border-border/60 flex items-center justify-center text-sm text-muted-foreground">
            No boards available yet.
          </div>
        ) : (
          <div
            className="grid gap-2 rounded-lg overflow-hidden h-full p-[2px]"
            style={{ gridTemplateColumns: `repeat(${columns.length}, minmax(0, 1fr))` }}
          >
            {columns.map((column) => (
              <KanbanColumn
                key={column.id}
                column={column}
                tasks={getTasksForColumn(column.id)}
                onOpenTask={onOpenTask}
                onEditTask={onEditTask}
                onDeleteTask={onDeleteTask}
              />
            ))}
          </div>
        )}
      </div>
      <DragOverlay dropAnimation={null}>
        {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}
