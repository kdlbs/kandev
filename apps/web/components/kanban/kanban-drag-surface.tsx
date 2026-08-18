"use client";

import type { ComponentProps, ReactNode, RefObject } from "react";
import { DndContext, DragOverlay } from "@dnd-kit/core";
import { KanbanCardPreview } from "@/components/kanban-card-preview";
import type { Task } from "@/components/kanban-card";

type KanbanDragSurfaceProps = Pick<
  ComponentProps<typeof DndContext>,
  "sensors" | "onDragStart" | "onDragEnd" | "onDragCancel"
> & {
  layoutContent: ReactNode;
  activeTask: Task | null;
  boardRef: RefObject<HTMLDivElement | null>;
};

export function KanbanDragSurface({
  layoutContent,
  activeTask,
  boardRef,
  ...dndProps
}: KanbanDragSurfaceProps) {
  return (
    <DndContext {...dndProps}>
      <div ref={boardRef} className="relative h-full min-h-0">
        {layoutContent}
      </div>
      <DragOverlay dropAnimation={null}>
        {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}
