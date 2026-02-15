'use client';

import type { ReactNode } from 'react';
import { SwimlaneHeader } from './swimlane-header';

export type SwimlaneSectionProps = {
  workflowId: string;
  workflowName: string;
  taskCount: number;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  children: ReactNode;
};

export function SwimlaneSection({
  workflowName,
  taskCount,
  isCollapsed,
  onToggleCollapse,
  children,
}: SwimlaneSectionProps) {
  return (
    <div>
      <SwimlaneHeader
        workflowName={workflowName}
        taskCount={taskCount}
        isCollapsed={isCollapsed}
        onToggleCollapse={onToggleCollapse}
      />
      {!isCollapsed && children}
    </div>
  );
}
