'use client';

import { useDraggable } from '@dnd-kit/core';
import { CSS } from '@dnd-kit/utilities';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';

export interface Task {
  id: string;
  title: string;
  status: string;
  description?: string;
}

interface KanbanCardProps {
  task: Task;
}

function KanbanCardLayout({ task, className }: KanbanCardProps & { className?: string }) {
  return (
    <Card size="sm" className={cn('w-full py-0', className)}>
      <CardContent className="px-3 py-2">
        <p className="text-sm font-medium">{task.title}</p>
        {task.description && (
          <p className="text-xs text-muted-foreground mt-1">{task.description}</p>
        )}
      </CardContent>
    </Card>
  );
}

export function KanbanCard({ task }: KanbanCardProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id,
  });

  const style = {
    transform: CSS.Translate.toString(transform),
    transition: 'none',
    willChange: isDragging ? 'transform' : undefined,
  };

  return (
    <Card
      size="sm"
      ref={setNodeRef}
      style={style}
      className={cn(
        'cursor-grab active:cursor-grabbing mb-2 w-full py-0',
        isDragging && 'opacity-50 z-50'
      )}
      {...listeners}
      {...attributes}
    >
      <CardContent className="px-3 py-2">
        <p className="text-sm font-medium">{task.title}</p>
        {task.description && (
          <p className="text-xs text-muted-foreground mt-1">{task.description}</p>
        )}
      </CardContent>
    </Card>
  );
}

export function KanbanCardPreview({ task }: KanbanCardProps) {
  return (
    <KanbanCardLayout
      task={task}
      className="cursor-grabbing shadow-lg ring-1 ring-primary/30 pointer-events-none"
    />
  );
}
