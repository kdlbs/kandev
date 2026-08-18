"use client";

import type { ComponentProps, ReactNode } from "react";
import { DndContext, DragOverlay } from "@dnd-kit/core";
import { KanbanCardPreview } from "@/components/kanban-card-preview";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import { DesktopAutoHiddenDropTargets } from "./mobile-drop-targets";

type KanbanDragSurfaceProps = Pick<
  ComponentProps<typeof DndContext>,
  "sensors" | "onDragStart" | "onDragEnd" | "onDragCancel"
> & {
  layoutContent: ReactNode;
  autoHiddenMoveTargets: WorkflowStep[];
  activeTask: Task | null;
  isMobile: boolean;
};

export function KanbanDragSurface({
  layoutContent,
  autoHiddenMoveTargets,
  activeTask,
  isMobile,
  ...dndProps
}: KanbanDragSurfaceProps) {
  return (
    <DndContext {...dndProps}>
      <div className="relative h-full min-h-0">
        {layoutContent}
        <DesktopAutoHiddenDropTargets
          steps={autoHiddenMoveTargets}
          isDragging={!!activeTask && !isMobile}
        />
      </div>
      <DragOverlay dropAnimation={null}>
        {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}
