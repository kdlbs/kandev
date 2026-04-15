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
  children,
}: SwimlaneSectionProps) {
  return (
    <div>
      <SwimlaneHeader
        workflowName={workflowName}
        taskCount={taskCount}
        isCollapsed={isCollapsed}
        onToggleCollapse={onToggleCollapse}
        dragHandleProps={dragHandleProps}
        onToggleMultiSelect={onToggleMultiSelect}
        isMultiSelectMode={isMultiSelectMode}
      />
      {!isCollapsed && children}
    </div>
  );
}
