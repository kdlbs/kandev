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
  children: ReactNode;
};

export function SwimlaneSection({
  workflowName,
  taskCount,
  isCollapsed,
  onToggleCollapse,
  dragHandleProps,
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
      />
      {!isCollapsed && children}
    </div>
  );
}
