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
  onOpenTask: (task: Task) => void;
}

export function KanbanColumn({ column, tasks, onOpenTask }: KanbanColumnProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: column.id,
  });

  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex flex-col h-full min-w-0 bg-card p-5 min-h-[600px] rounded-lg',
        isOver && 'ring-2 ring-primary'
      )}
    >
      {/* Column Header */}
      <div className="flex items-center justify-between border-b border-border/70 pb-3 mb-4 px-3">
        <div className="flex items-center gap-2">
          <div className={cn('w-3 h-3 rounded-full', column.color)} />
          <h2 className="font-semibold text-base">{column.title}</h2>
          <Badge variant="secondary" className="text-xs">
            {tasks.length}
          </Badge>
        </div>
      </div>

      {/* Tasks */}
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden space-y-2 px-3 pt-4">
        {tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center mt-8">No tasks yet</p>
        ) : (
          tasks.map((task) => (
            <KanbanCard key={task.id} task={task} onClick={onOpenTask} />
          ))
        )}
      </div>
    </div>
  );
}
