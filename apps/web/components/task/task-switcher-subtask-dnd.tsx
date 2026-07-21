"use client";

import { useCallback, useMemo } from "react";
import {
  DndContext,
  MouseSensor,
  TouchSensor,
  closestCenter,
  type DragEndEvent,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { cn } from "@/lib/utils";
import type { TaskSwitcherItem } from "./task-switcher";

export const DRAG_ACTIVATION_DISTANCE = 8;
export const TOUCH_DRAG_ACTIVATION_DELAY_MS = 250;
export const TOUCH_DRAG_ACTIVATION_TOLERANCE_PX = 5;

const TASK_SWITCHER_DRAG_ACTIVATION_CONSTRAINTS = {
  pointer: { distance: DRAG_ACTIVATION_DISTANCE },
  touch: {
    delay: TOUCH_DRAG_ACTIVATION_DELAY_MS,
    tolerance: TOUCH_DRAG_ACTIVATION_TOLERANCE_PX,
  },
};

export function taskSwitcherDragActivationConstraints() {
  return TASK_SWITCHER_DRAG_ACTIVATION_CONSTRAINTS;
}

// Skip layout/paint for rows outside the sidebar scrollport so long task lists
// stay cheap to render and scroll. `auto 52px` seeds the height estimate before
// first paint; afterwards the browser remembers each row's real size, so
// scrollbar/anchor behavior stays stable and dnd-kit's drag-start rect
// measurements remain accurate for previously rendered rows.
const OFFSCREEN_SKIP = "[content-visibility:auto] [contain-intrinsic-block-size:auto_52px]";

/** Sortable implementation — isolated so `useSortable` is never called conditionally. */
function DraggableSortableTaskNode({
  taskId,
  depth,
  handle,
  nested,
}: {
  taskId: string;
  depth: number;
  handle: React.ReactNode;
  nested?: React.ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: taskId,
  });
  const sortableAttributes = {
    ...attributes,
    role: undefined,
    "aria-roledescription": undefined,
  };
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : undefined,
  };
  return (
    <div
      ref={setNodeRef}
      style={style}
      data-testid="sortable-task-block"
      data-task-id={taskId}
      data-depth={depth}
      className={cn(OFFSCREEN_SKIP, isDragging && "z-50")}
    >
      <div
        {...sortableAttributes}
        {...listeners}
        // Strip dnd-kit's default tabIndex={0}: only pointer-based sensors are
        // wired, so keyboard tab stops here lead nowhere. If KeyboardSensor is
        // added later, drop this override.
        tabIndex={undefined}
        data-testid="sortable-task-handle"
        className="cursor-grab active:cursor-grabbing"
      >
        {handle}
      </div>
      {nested}
    </div>
  );
}

/**
 * A single sortable node in the (arbitrarily deep) task tree.
 *
 * `setNodeRef` and the CSS transform live on the outer wrapper so the row and
 * its nested subtree move together while dragging. Drag listeners attach ONLY
 * to the `handle` (the task row) — `nested` (the child subtree) renders as a
 * sibling outside the handle so a pointer-down on a descendant row drags that
 * descendant, not this node. This is the same handle/children split the root
 * level has always used; generalizing it is what makes nesting safe at depth.
 *
 * When `isDraggable` is false, renders a plain wrapper with no DnD wiring.
 */
export function SortableTaskNode({
  taskId,
  depth,
  handle,
  nested,
  isDraggable = true,
}: {
  taskId: string;
  depth: number;
  handle: React.ReactNode;
  nested?: React.ReactNode;
  isDraggable?: boolean;
}) {
  if (!isDraggable) {
    return (
      <div
        data-testid="sortable-task-block"
        data-task-id={taskId}
        data-depth={depth}
        className={OFFSCREEN_SKIP}
      >
        <div data-testid="sortable-task-handle">{handle}</div>
        {nested}
      </div>
    );
  }

  return (
    <DraggableSortableTaskNode taskId={taskId} depth={depth} handle={handle} nested={nested} />
  );
}

/**
 * DnD wiring for one level of sibling tasks. Reordering is scoped to the
 * siblings sharing a parent (cross-parent drag is intentionally unsupported).
 */
function useLevelDnd(tasks: TaskSwitcherItem[], onReorder?: (orderedTaskIds: string[]) => void) {
  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: TASK_SWITCHER_DRAG_ACTIVATION_CONSTRAINTS.pointer,
    }),
    useSensor(TouchSensor, {
      activationConstraint: TASK_SWITCHER_DRAG_ACTIVATION_CONSTRAINTS.touch,
    }),
  );
  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      if (!onReorder) return;
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const ids = tasks.map((t) => t.id);
      const oldIndex = ids.indexOf(String(active.id));
      const newIndex = ids.indexOf(String(over.id));
      if (oldIndex < 0 || newIndex < 0) return;
      onReorder(arrayMove(ids, oldIndex, newIndex));
    },
    [tasks, onReorder],
  );
  const sortableIds = useMemo(() => tasks.map((t) => t.id), [tasks]);
  return { sensors, handleDragEnd, sortableIds };
}

/**
 * Renders one level of sibling task nodes. When `onReorder` is omitted the DnD
 * scaffolding (and the misleading grab cursor) is skipped, so non-reorderable
 * callers get a plain list with no wasted setup.
 */
export function SortableTaskLevel({
  tasks,
  onReorder,
  renderNode,
}: {
  tasks: TaskSwitcherItem[];
  onReorder?: (orderedTaskIds: string[]) => void;
  renderNode: (task: TaskSwitcherItem, isDraggable: boolean) => React.ReactNode;
}) {
  const { sensors, handleDragEnd, sortableIds } = useLevelDnd(tasks, onReorder);
  const isDraggable = !!onReorder;
  const body = tasks.map((task) => renderNode(task, isDraggable));
  if (!onReorder) return <>{body}</>;
  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={sortableIds} strategy={verticalListSortingStrategy}>
        {body}
      </SortableContext>
    </DndContext>
  );
}
