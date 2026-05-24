"use client";

import { Fragment, useCallback, useMemo } from "react";
import {
  DndContext,
  PointerSensor,
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

function SortableSubtaskRow({ taskId, children }: { taskId: string; children: React.ReactNode }) {
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
      {...sortableAttributes}
      {...listeners}
      tabIndex={undefined}
      data-testid="sortable-subtask-row"
      data-task-id={taskId}
      className={cn("cursor-grab active:cursor-grabbing", isDragging && "z-50")}
    >
      {children}
    </div>
  );
}

function useSubtaskDnd(
  parentTaskId: string,
  subtasks: TaskSwitcherItem[],
  onReorderSubtasks: (parentTaskId: string, orderedSubtaskIds: string[]) => void,
) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: DRAG_ACTIVATION_DISTANCE } }),
  );
  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const ids = subtasks.map((t) => t.id);
      const oldIndex = ids.indexOf(String(active.id));
      const newIndex = ids.indexOf(String(over.id));
      if (oldIndex < 0 || newIndex < 0) return;
      onReorderSubtasks(parentTaskId, arrayMove(ids, oldIndex, newIndex));
    },
    [parentTaskId, subtasks, onReorderSubtasks],
  );
  const sortableIds = useMemo(() => subtasks.map((t) => t.id), [subtasks]);
  return { sensors, handleDragEnd, sortableIds };
}

/**
 * Per-parent sortable list. When `onReorderSubtasks` is omitted the DnD
 * scaffolding (and the misleading grab cursor) is skipped — callers that
 * don't reorder get a plain list with no wasted setup.
 */
export function SortableSubtaskList({
  parentTaskId,
  subtasks,
  onReorderSubtasks,
  renderRow,
}: {
  parentTaskId: string;
  subtasks: TaskSwitcherItem[];
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  renderRow: (sub: TaskSwitcherItem) => React.ReactNode;
}) {
  if (!onReorderSubtasks) {
    return (
      <>
        {subtasks.map((sub) => (
          <Fragment key={sub.id}>{renderRow(sub)}</Fragment>
        ))}
      </>
    );
  }
  return (
    <SortableSubtaskListInner
      parentTaskId={parentTaskId}
      subtasks={subtasks}
      onReorderSubtasks={onReorderSubtasks}
      renderRow={renderRow}
    />
  );
}

function SortableSubtaskListInner({
  parentTaskId,
  subtasks,
  onReorderSubtasks,
  renderRow,
}: {
  parentTaskId: string;
  subtasks: TaskSwitcherItem[];
  onReorderSubtasks: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  renderRow: (sub: TaskSwitcherItem) => React.ReactNode;
}) {
  const { sensors, handleDragEnd, sortableIds } = useSubtaskDnd(
    parentTaskId,
    subtasks,
    onReorderSubtasks,
  );
  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={sortableIds} strategy={verticalListSortingStrategy}>
        {subtasks.map((sub) => (
          <SortableSubtaskRow key={sub.id} taskId={sub.id}>
            {renderRow(sub)}
          </SortableSubtaskRow>
        ))}
      </SortableContext>
    </DndContext>
  );
}
