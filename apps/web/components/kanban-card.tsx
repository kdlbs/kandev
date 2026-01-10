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

export function KanbanCard({ task }: KanbanCardProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id,
  });

  const style = {
    transform: CSS.Translate.toString(transform),
  };

  return (
    <Card
      ref={setNodeRef}
      style={style}
      className={cn(
        'cursor-grab active:cursor-grabbing mb-2 transition-opacity',
        isDragging && 'opacity-50 z-50'
      )}
      {...listeners}
      {...attributes}
    >
      <CardContent className="p-3">
        <p className="text-sm font-medium">{task.title}</p>
        {task.description && (
          <p className="text-xs text-muted-foreground mt-1">{task.description}</p>
        )}
      </CardContent>
    </Card>
  );
}
