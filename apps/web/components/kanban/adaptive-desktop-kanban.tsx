"use client";

import type { ReactNode } from "react";
import type { WorkflowStep } from "@/components/kanban-column";
import { getKanbanColumnGridTemplate } from "./kanban-grid-template";

type AdaptiveDesktopKanbanProps = {
  steps: WorkflowStep[];
  renderColumn: (step: WorkflowStep) => ReactNode;
};

export function AdaptiveDesktopKanban({ steps, renderColumn }: AdaptiveDesktopKanbanProps) {
  return (
    <div data-testid="desktop-kanban-layout" className="h-full min-w-0">
      <div
        data-testid="desktop-kanban-scroll-window"
        className="h-full min-w-0 overflow-x-auto snap-x snap-mandatory"
      >
        <div
          data-testid="desktop-kanban-lane-grid"
          className="grid h-full min-w-full gap-0"
          style={{ gridTemplateColumns: getKanbanColumnGridTemplate(steps.length) }}
        >
          {steps.map((step) => (
            <div key={step.id} className="min-w-0 snap-start">
              {renderColumn(step)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
