'use client';

import { useMemo } from 'react';
import { useDroppable } from '@dnd-kit/core';
import { KanbanCard, Task } from './kanban-card';
import { Badge } from '@kandev/ui/badge';
import { cn, getRepositoryDisplayName } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import type { Repository } from '@/lib/types/http';

export interface WorkflowStep {
  id: string;
  title: string;
  color: string;
  autoStartAgent?: boolean;
}

interface KanbanColumnProps {
  column: WorkflowStep;
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
}

export function KanbanColumn({ column, tasks, onPreviewTask, onOpenTask, onEditTask, onDeleteTask, showMaximizeButton, deletingTaskId }: KanbanColumnProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: column.id,
  });

  // Access repositories from store to pass repository names to cards
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspace).flat() as Repository[],
    [repositoriesByWorkspace]
  );

  // Helper function to get repository name for a task
  const getRepositoryName = (repositoryId?: string): string | null => {
    if (!repositoryId) return null;
    const repository = repositories.find((repo) => repo.id === repositoryId);
    return repository ? getRepositoryDisplayName(repository.local_path) : null;
  };

  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex flex-col h-full min-w-0  p-4 min-h-[600px] rounded-sm border border-border',
        isOver && 'ring-2 ring-primary'
      )}
    >
      {/* Column Header */}
      <div className="flex items-center justify-between border-b border-border/70 pb-3 mb-4 px-1">
        <div className="flex items-center gap-2">
          <div className={cn('w-3 h-3 rounded-full', column.color)} />
          <h2 className="font-semibold text-sm">{column.title}</h2>
          <Badge variant="secondary" className="text-xs">
            {tasks.length}
          </Badge>
        </div>
      </div>

      {/* Tasks */}
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden space-y-2 px-1 pt-4">
        {tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center mt-8">No tasks yet</p>
        ) : (
          tasks.map((task) => (
            <KanbanCard
              key={task.id}
              task={task}
              repositoryName={getRepositoryName(task.repositoryId)}
              onClick={onPreviewTask}
              onOpenFullPage={onOpenTask}
              onEdit={onEditTask}
              onDelete={onDeleteTask}
              showMaximizeButton={showMaximizeButton}
              isDeleting={deletingTaskId === task.id}
            />
          ))
        )}
      </div>
    </div>
  );
}
