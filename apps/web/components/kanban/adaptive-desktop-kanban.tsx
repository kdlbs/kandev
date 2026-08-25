"use client";

import type { ReactNode } from "react";
import type { WorkflowStep } from "@/components/kanban-column";
import { getKanbanColumnGridTemplate, KANBAN_COLUMN_MIN_PX } from "./kanban-grid-template";

type AdaptiveDesktopKanbanProps = {
  steps: WorkflowStep[];
  isDragging?: boolean;
  renderColumn: (step: WorkflowStep) => ReactNode;
};

export const KANBAN_DRAG_END_PADDING = `max(0px, calc(100% - ${KANBAN_COLUMN_MIN_PX}px))`;

export function AdaptiveDesktopKanban({
  steps,
  isDragging = false,
  renderColumn,
}: AdaptiveDesktopKanbanProps) {
  return (
    <div data-testid="desktop-kanban-layout" className="h-full min-h-0 min-w-0">
      <div
        data-testid="desktop-kanban-scroll-window"
        className="h-full min-h-0 min-w-0 overflow-x-auto snap-x snap-mandatory"
      >
        <div
          data-testid="desktop-kanban-lane-grid"
          className="grid h-full min-h-0 min-w-full gap-0"
          style={{
            gridTemplateColumns: getKanbanColumnGridTemplate(steps.length),
            paddingInlineEnd: isDragging ? KANBAN_DRAG_END_PADDING : undefined,
          }}
        >
          {steps.map((step) => (
            <div key={step.id} data-kanban-step-id={step.id} className="min-h-0 min-w-0 snap-start">
              {renderColumn(step)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
