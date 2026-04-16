"use client";

import type { HTMLAttributes } from "react";
import { Badge } from "@kandev/ui/badge";
import { Checkbox } from "@kandev/ui/checkbox";
import { IconChevronRight, IconGripVertical } from "@tabler/icons-react";
import { cn } from "@kandev/ui/lib/utils";

export type SwimlaneHeaderProps = {
  workflowName: string;
  taskCount: number;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  dragHandleProps?: HTMLAttributes<HTMLDivElement>;
  onToggleMultiSelect?: () => void;
  isMultiSelectMode?: boolean;
};

export function SwimlaneHeader({
  workflowName,
  taskCount,
  isCollapsed,
  onToggleCollapse,
  dragHandleProps,
  onToggleMultiSelect,
  isMultiSelectMode,
}: SwimlaneHeaderProps) {
  return (
    <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-1 py-1.5 w-full" data-testid="swimlane-header">
      <div className="flex items-center gap-1">
        {dragHandleProps && (
          <div
            className="cursor-grab active:cursor-grabbing shrink-0"
            data-testid="swimlane-drag-handle"
            {...dragHandleProps}
          >
            <IconGripVertical className="h-3.5 w-3.5 text-muted-foreground" />
          </div>
        )}
        {onToggleMultiSelect && (
          <button
            type="button"
            onClick={onToggleMultiSelect}
            data-testid="multi-select-toggle"
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors shrink-0 cursor-pointer"
          >
            <Checkbox
              checked={!!isMultiSelectMode}
              className="pointer-events-none h-3.5 w-3.5"
              tabIndex={-1}
            />
            Multi-select
          </button>
        )}
        <div className="flex-1 border-t border-dashed border-border/50" />
      </div>
      <button
        type="button"
        onClick={onToggleCollapse}
        className="cursor-pointer group"
      >
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
      </button>
      <div className="border-t border-dashed border-border/50" />
    </div>
  );
}
