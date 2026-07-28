"use client";

import { Children, useMemo, type ReactNode } from "react";
import type { WorkflowStep } from "@/components/kanban-column";
import { getKanbanColumnGridTemplate } from "./kanban-grid-template";

type AdaptiveDesktopKanbanProps = {
  steps: WorkflowStep[];
  children: ReactNode;
};

export function AdaptiveDesktopKanban({ steps, children }: AdaptiveDesktopKanbanProps) {
  const columns = useMemo(() => Children.toArray(children), [children]);

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
          {columns.map((column, index) => (
            <div key={steps[index]?.id ?? index} className="min-w-0 snap-start">
              {column}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
