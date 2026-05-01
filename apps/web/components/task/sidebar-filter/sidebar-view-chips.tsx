"use client";

import { useCallback, useRef, type PointerEvent } from "react";
import {
  DndContext,
  PointerSensor,
  closestCenter,
  type DragEndEvent,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { SortableContext, horizontalListSortingStrategy, useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useAppStore } from "@/components/state-provider";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { cn } from "@/lib/utils";

const DRAG_ACTIVATION_DISTANCE = 8;

export function SidebarViewChips() {
  const views = useAppStore((s) => s.sidebarViews.views);
  const activeViewId = useAppStore((s) => s.sidebarViews.activeViewId);
  const setActive = useAppStore((s) => s.setSidebarActiveView);
  const reorderViews = useAppStore((s) => s.reorderSidebarViews);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: DRAG_ACTIVATION_DISTANCE } }),
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      reorderViews(String(active.id), String(over.id));
    },
    [reorderViews],
  );

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext
        items={views.map((view) => view.id)}
        strategy={horizontalListSortingStrategy}
      >
        <div
          className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto"
          data-testid="sidebar-view-chip-row"
        >
          {views.map((view) => (
            <SidebarViewChip
              key={view.id}
              view={view}
              active={view.id === activeViewId}
              onSelect={() => setActive(view.id)}
            />
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}

function SidebarViewChip({
  view,
  active,
  onSelect,
}: {
  view: SidebarView;
  active: boolean;
  onSelect: () => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: view.id,
  });
  const sortableAttributes = {
    ...attributes,
    role: undefined,
    "aria-roledescription": undefined,
  };
  const pointerStartRef = useRef<{ x: number; y: number } | null>(null);
  const pointerMovedRef = useRef(false);
  const skipNextClickRef = useRef(false);
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : undefined,
  };
  const resetPointerState = useCallback(() => {
    pointerStartRef.current = null;
    pointerMovedRef.current = false;
  }, []);
  const handlePointerDownCapture = useCallback((event: PointerEvent<HTMLButtonElement>) => {
    pointerStartRef.current = { x: event.clientX, y: event.clientY };
    pointerMovedRef.current = false;
  }, []);
  const handlePointerMoveCapture = useCallback((event: PointerEvent<HTMLButtonElement>) => {
    const start = pointerStartRef.current;
    if (!start) return;
    const distance = Math.hypot(event.clientX - start.x, event.clientY - start.y);
    if (distance >= DRAG_ACTIVATION_DISTANCE) pointerMovedRef.current = true;
  }, []);
  const handlePointerUpCapture = useCallback(() => {
    const shouldSelect = pointerStartRef.current && !pointerMovedRef.current;
    resetPointerState();
    if (!shouldSelect) return;
    skipNextClickRef.current = true;
    window.setTimeout(() => {
      skipNextClickRef.current = false;
    }, 0);
    onSelect();
  }, [onSelect, resetPointerState]);
  const handleClick = useCallback(() => {
    if (skipNextClickRef.current) {
      skipNextClickRef.current = false;
      return;
    }
    onSelect();
  }, [onSelect]);

  return (
    <button
      ref={setNodeRef}
      style={style}
      type="button"
      {...sortableAttributes}
      {...listeners}
      onPointerDownCapture={handlePointerDownCapture}
      onPointerMoveCapture={handlePointerMoveCapture}
      onPointerUpCapture={handlePointerUpCapture}
      onPointerCancelCapture={resetPointerState}
      data-testid="sidebar-view-chip"
      data-view-id={view.id}
      data-active={active}
      aria-pressed={active}
      className={cn(
        "flex h-6 shrink-0 cursor-pointer items-center rounded-md border px-2 text-left text-[11px] transition-colors active:cursor-grabbing",
        active
          ? "border-primary/40 bg-primary/10 text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground",
        isDragging && "z-50 cursor-grabbing",
      )}
      title={view.name}
      onClick={handleClick}
    >
      <span className="block max-w-[120px] truncate">{view.name}</span>
    </button>
  );
}
