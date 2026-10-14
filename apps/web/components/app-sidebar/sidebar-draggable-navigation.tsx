"use client";
import { useEffect, useState, type ReactNode } from "react";
import {
  DndContext,
  PointerSensor,
  useSensor,
  useSensors,
  closestCenter,
  type DragEndEvent,
} from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import { moveSection } from "@/lib/sidebar/layout-operations";
import { useSidebarCustomization } from "@/hooks/domains/sidebar/use-sidebar-customization";
import { useAppStore } from "@/components/state-provider";

class NavigationPointerSensor extends PointerSensor {
  static activators = [
    {
      eventName: "onPointerDown" as const,
      handler: ({ nativeEvent }: React.PointerEvent) => {
        if (nativeEvent.button !== 0 || !nativeEvent.isPrimary) return false;
        const target = nativeEvent.target as Element;
        const row = target.closest("[data-sidebar-node-id]");
        if (!row || target.closest("[data-sidebar-drag-exclude]")) return false;
        const control = target.closest("a,button,input");
        return !control || control === row.querySelector("a,button");
      },
    },
  ];
}

function DraggableRow({
  id,
  children,
  disabled,
  move,
}: {
  id: string;
  children: ReactNode;
  disabled: boolean;
  move: (delta: number) => void;
}) {
  const { setNodeRef, listeners, transform, transition, isDragging, over } = useSortable({
    id,
    disabled,
  });
  return (
    <div
      ref={setNodeRef}
      {...listeners}
      data-sidebar-node-id={id}
      data-testid={`sidebar-node-${id}`}
      className={over?.id === id && !isDragging ? "relative border-t-2 border-primary" : "relative"}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
      }}
      onKeyDown={(event) => {
        if (disabled || !event.altKey || !["ArrowUp", "ArrowDown"].includes(event.key)) return;
        event.preventDefault();
        move(event.key === "ArrowUp" ? -1 : 1);
      }}
    >
      {children}
    </div>
  );
}

export function SidebarDraggableNavigation({
  rows,
  catalog,
  disabled = false,
}: {
  rows: { id: string; content: ReactNode }[];
  catalog: ShortcutCatalogEntry[];
  disabled?: boolean;
}) {
  const { mutate, status, enabled } = useSidebarCustomization(catalog);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [dragging, setDragging] = useState(false);
  const sensors = useSensors(
    useSensor(NavigationPointerSensor, { activationConstraint: { distance: 8 } }),
  );
  useEffect(() => {
    if (!dragging) return;
    const previous = document.documentElement.style.cursor;
    document.documentElement.style.cursor = "grabbing";
    const sheet = document.createElement("style");
    sheet.textContent = "* { cursor: grabbing !important; }";
    document.head.appendChild(sheet);
    return () => {
      document.documentElement.style.cursor = previous;
      sheet.remove();
    };
  }, [dragging]);
  useEffect(() => setDragging(false), [workspaceId]);
  const end = ({ active, over }: DragEndEvent) => {
    setDragging(false);
    if (!over || active.id === over.id) return;
    void mutate((layout) =>
      moveSection(
        layout,
        String(active.id),
        layout.nodes.findIndex((node) => node.id === over.id),
      ),
    );
  };
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={() => setDragging(true)}
      onDragCancel={() => setDragging(false)}
      onDragEnd={end}
    >
      <SortableContext items={rows.map((row) => row.id)} strategy={verticalListSortingStrategy}>
        <div className="flex flex-col gap-1">
          {rows.map((row, index) => (
            <DraggableRow
              key={row.id}
              id={row.id}
              disabled={disabled || !enabled || status === "saving"}
              move={(delta) => {
                const neighbour = rows[index + delta];
                if (neighbour)
                  void mutate((layout) =>
                    moveSection(
                      layout,
                      row.id,
                      layout.nodes.findIndex((node) => node.id === neighbour.id),
                    ),
                  );
              }}
            >
              {row.content}
            </DraggableRow>
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}
