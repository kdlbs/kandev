'use client';

import { useDroppable } from '@dnd-kit/core';
import { cn } from '@/lib/utils';
import type { WorkflowStep } from '../kanban-column';

type MobileDropTargetProps = {
  column: WorkflowStep;
  isCurrentColumn: boolean;
};

function MobileDropTarget({ column, isCurrentColumn }: MobileDropTargetProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: column.id,
  });

  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex items-center justify-center gap-2 px-3 py-3 rounded-lg border-2 border-dashed transition-all min-w-[100px]',
        isOver
          ? 'border-primary bg-primary/10 scale-105'
          : isCurrentColumn
            ? 'border-muted-foreground/30 bg-muted/50 opacity-50'
            : 'border-muted-foreground/40 bg-background hover:border-muted-foreground/60'
      )}
    >
      <div className={cn('w-3 h-3 rounded-full flex-shrink-0', column.color)} />
      <span className="text-sm font-medium truncate max-w-[80px]">{column.title}</span>
    </div>
  );
}

type MobileDropTargetsProps = {
  columns: WorkflowStep[];
  currentColumnId: string | null;
  isDragging: boolean;
};

export function MobileDropTargets({
  columns,
  currentColumnId,
  isDragging,
}: MobileDropTargetsProps) {
  if (!isDragging) return null;

  return (
    <div className="fixed bottom-0 left-0 right-0 z-50 p-4 bg-gradient-to-t from-background via-background to-transparent">
      <div className="flex gap-2 overflow-x-auto pb-safe scrollbar-hide">
        {columns.map((column) => (
          <MobileDropTarget
            key={column.id}
            column={column}
            isCurrentColumn={column.id === currentColumnId}
          />
        ))}
      </div>
      <p className="text-xs text-muted-foreground text-center mt-2">
        Drop on a column to move task
      </p>
    </div>
  );
}
