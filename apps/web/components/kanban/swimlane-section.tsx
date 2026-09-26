"use client";

import type { HTMLAttributes, ReactNode } from "react";
import { SwimlaneHeader } from "./swimlane-header";

export type SwimlaneSectionProps = {
  workflowId: string;
  workflowName: string;
  taskCount: number;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  dragHandleProps?: HTMLAttributes<HTMLDivElement>;
  onToggleMultiSelect?: () => void;
  isMultiSelectMode?: boolean;
  fillHeight?: boolean;
  columnsMenu?: ReactNode;
  children: ReactNode;
};

export function SwimlaneSection({
  workflowName,
  taskCount,
  isCollapsed,
  onToggleCollapse,
  dragHandleProps,
  onToggleMultiSelect,
  isMultiSelectMode,
  fillHeight = false,
  columnsMenu,
  children,
}: SwimlaneSectionProps) {
  return (
    <div className={fillHeight ? "flex h-full min-h-0 flex-col" : undefined}>
      <SwimlaneHeader
        workflowName={workflowName}
        taskCount={taskCount}
        isCollapsed={isCollapsed}
        onToggleCollapse={onToggleCollapse}
        dragHandleProps={dragHandleProps}
        onToggleMultiSelect={onToggleMultiSelect}
        isMultiSelectMode={isMultiSelectMode}
        columnsMenu={columnsMenu}
      />
      {!isCollapsed && (fillHeight ? <div className="min-h-0 flex-1">{children}</div> : children)}
    </div>
  );
}
