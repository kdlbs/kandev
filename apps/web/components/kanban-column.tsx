'use client';

import { useDroppable } from '@dnd-kit/core';
import { KanbanCard, Task } from './kanban-card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { IconPlus } from '@tabler/icons-react';
import { useState, KeyboardEvent } from 'react';

export interface Column {
  id: string;
  title: string;
  color: string;
}

interface KanbanColumnProps {
  column: Column;
  tasks: Task[];
  onAddTask: (columnId: string, title: string) => void;
}

export function KanbanColumn({ column, tasks, onAddTask }: KanbanColumnProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: column.id,
  });

  const [isAdding, setIsAdding] = useState(false);
  const [newTaskTitle, setNewTaskTitle] = useState('');

  const handleAddClick = () => {
    setIsAdding(true);
  };

  const handleSubmit = () => {
    if (newTaskTitle.trim()) {
      onAddTask(column.id, newTaskTitle.trim());
      setNewTaskTitle('');
      setIsAdding(false);
    }
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      handleSubmit();
    } else if (e.key === 'Escape') {
      setNewTaskTitle('');
      setIsAdding(false);
    }
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
          onClick={handleAddClick}
        >
          <IconPlus className="h-4 w-4" />
        </Button>
      </div>

      {/* Add Task Input */}
      {isAdding && (
        <div className="mb-3">
          <Input
            autoFocus
            placeholder="Enter task title..."
            value={newTaskTitle}
            onChange={(e) => setNewTaskTitle(e.target.value)}
            onKeyDown={handleKeyDown}
            onBlur={handleSubmit}
            className="text-sm"
          />
        </div>
      )}

      {/* Tasks */}
      <div className="flex-1 overflow-y-auto space-y-0">
        {tasks.length === 0 && !isAdding ? (
          <p className="text-sm text-muted-foreground text-center mt-8">No tasks yet</p>
        ) : (
          tasks.map((task) => <KanbanCard key={task.id} task={task} />)
        )}
      </div>
    </div>
  );
}
