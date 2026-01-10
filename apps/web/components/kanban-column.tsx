'use client';

import { useDroppable } from '@dnd-kit/core';
import { KanbanCard, Task } from './kanban-card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { IconPlus } from '@tabler/icons-react';
import { useState } from 'react';
import { TaskCreateDialog } from './task-create-dialog';

export interface Column {
  id: string;
  title: string;
  color: string;
}

interface KanbanColumnProps {
  column: Column;
  tasks: Task[];
  onAddTask: (columnId: string, title: string, description?: string) => void;
}

export function KanbanColumn({ column, tasks, onAddTask }: KanbanColumnProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: column.id,
  });

  const [isDialogOpen, setIsDialogOpen] = useState(false);

  const handleDialogSubmit = (title: string, description: string) => {
    onAddTask(column.id, title, description);
  };

  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex flex-col h-full bg-card rounded-lg border border-border p-4 min-h-[600px]',
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
        <Button
          variant="ghost"
          size="sm"
          className="h-8 w-8 p-0"
          onClick={() => setIsDialogOpen(true)}
        >
          <IconPlus className="h-4 w-4" />
        </Button>
      </div>

      <TaskCreateDialog
        open={isDialogOpen}
        onOpenChange={setIsDialogOpen}
        onSubmit={handleDialogSubmit}
      />

      {/* Tasks */}
      <div className="flex-1 overflow-y-auto space-y-0">
        {tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center mt-8">No tasks yet</p>
        ) : (
          tasks.map((task) => <KanbanCard key={task.id} task={task} />)
        )}
      </div>
    </div>
  );
}
