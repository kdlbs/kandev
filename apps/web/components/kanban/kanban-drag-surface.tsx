"use client";

import { useEffect, useRef, type ComponentProps, type ReactNode, type RefObject } from "react";
import { DndContext, DragOverlay, pointerWithin } from "@dnd-kit/core";
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

export const KANBAN_AUTO_SCROLL_OPTIONS = {
  layoutShiftCompensation: { x: false, y: true },
} as const;

// A drag's pointer release must not activate another workflow's card.
function suppressDragReleaseClick(ownerDocument: Document) {
  const clear = () => {
    ownerDocument.removeEventListener("click", onClick, true);
    ownerDocument.removeEventListener("pointerdown", onPointerDown, true);
  };
  const onClick = (event: MouseEvent) => {
    clear();
    if (event.detail > 0) {
      event.preventDefault();
      event.stopPropagation();
    }
  };
  const onPointerDown = (event: PointerEvent) => {
    if (event.isPrimary !== false) clear();
  };
  ownerDocument.addEventListener("click", onClick, true);
  ownerDocument.addEventListener("pointerdown", onPointerDown, true);
  return clear;
}

export function KanbanDragSurface({
  layoutContent,
  activeTask,
  boardRef,
  onDragStart,
  ...dndProps
}: KanbanDragSurfaceProps) {
  const clearClickGuard = useRef<(() => void) | null>(null);
  useEffect(() => () => clearClickGuard.current?.(), []);

  return (
    <DndContext
      autoScroll={KANBAN_AUTO_SCROLL_OPTIONS}
      collisionDetection={pointerWithin}
      {...dndProps}
      onDragStart={(event) => {
        clearClickGuard.current?.();
        clearClickGuard.current = suppressDragReleaseClick(document);
        onDragStart?.(event);
      }}
    >
      <div ref={boardRef} className="relative h-full min-h-0">
        {layoutContent}
      </div>
      <DragOverlay dropAnimation={null}>
        {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}
