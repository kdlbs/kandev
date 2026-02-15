'use client';

import { Badge } from '@kandev/ui/badge';
import { IconChevronRight } from '@tabler/icons-react';
import { cn } from '@kandev/ui/lib/utils';

export type SwimlaneHeaderProps = {
  workflowName: string;
  taskCount: number;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
};

export function SwimlaneHeader({
  workflowName,
  taskCount,
  isCollapsed,
  onToggleCollapse,
}: SwimlaneHeaderProps) {
  return (
    <button
      type="button"
      onClick={onToggleCollapse}
      className="flex items-center gap-2 py-1.5 w-full text-left cursor-pointer group"
    >
      <div className="flex-1 border-t border-dashed border-border/50" />
      <Badge variant="secondary" className="text-xs shrink-0 gap-1.5 px-2.5 py-0.5">
        <IconChevronRight
          className={cn(
            'h-3 w-3 text-muted-foreground transition-transform shrink-0',
            !isCollapsed && 'rotate-90'
          )}
        />
        {workflowName}
        <span className="text-muted-foreground/60">{taskCount}</span>
      </Badge>
      <div className="flex-1 border-t border-dashed border-border/50" />
    </button>
  );
}
