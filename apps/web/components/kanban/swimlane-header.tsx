"use client";

import type { HTMLAttributes } from "react";
import { Badge } from "@kandev/ui/badge";
import { IconChevronRight, IconGripVertical } from "@tabler/icons-react";
import { cn } from "@kandev/ui/lib/utils";

export type SwimlaneHeaderProps = {
  workflowName: string;
  taskCount: number;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  dragHandleProps?: HTMLAttributes<HTMLDivElement>;
};

export function SwimlaneHeader({
  workflowName,
  taskCount,
  isCollapsed,
  onToggleCollapse,
  dragHandleProps,
}: SwimlaneHeaderProps) {
  return (
    <div className="flex items-center gap-1 py-1.5 w-full" data-testid="swimlane-header">
      {dragHandleProps && (
        <div
          className="cursor-grab active:cursor-grabbing shrink-0"
          data-testid="swimlane-drag-handle"
          {...dragHandleProps}
        >
          <IconGripVertical className="h-3.5 w-3.5 text-muted-foreground" />
        </div>
      )}
      <button
        type="button"
        onClick={onToggleCollapse}
        className="flex items-center gap-2 flex-1 text-left cursor-pointer group"
      >
        <div className="flex-1 border-t border-dashed border-border/50" />
        <Badge variant="secondary" className="text-xs shrink-0 gap-1.5 px-2.5 py-0.5">
          <IconChevronRight
            className={cn(
              "h-3 w-3 text-muted-foreground transition-transform shrink-0",
              !isCollapsed && "rotate-90",
            )}
          />
          {workflowName}
          <span className="text-muted-foreground/60">{taskCount}</span>
        </Badge>
        <div className="flex-1 border-t border-dashed border-border/50" />
      </button>
    </div>
  );
}
