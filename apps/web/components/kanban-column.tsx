'use client';

import { useDroppable } from '@dnd-kit/core';
import { KanbanCard, Task } from './kanban-card';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

export interface Column {
  id: string;
  title: string;
  color: string;
}

interface KanbanColumnProps {
  column: Column;
  tasks: Task[];
}

export function KanbanColumn({ column, tasks }: KanbanColumnProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: column.id,
  });

  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex flex-col h-full min-w-0 bg-card rounded-lg border border-border p-4 min-h-[600px]',
        isOver && 'ring-2 ring-primary'
      )}
    >
      {/* Column Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <div className={cn('w-3 h-3 rounded-full', column.color)} />
          <h2 className="font-semibold text-base">{column.title}</h2>
          <Badge variant="secondary" className="text-xs">
            {tasks.length}
          </Badge>
        </div>
      </div>

      {/* Tasks */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden space-y-0">
        {tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center mt-8">No tasks yet</p>
        ) : (
          tasks.map((task) => <KanbanCard key={task.id} task={task} />)
        )}
      </div>
    </div>
  );
}
