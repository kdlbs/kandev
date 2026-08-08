"use client";

import { useCallback, useMemo, useState } from "react";
import {
  DndContext,
  MouseSensor,
  TouchSensor,
  closestCenter,
  pointerWithin,
  useDroppable,
  type CollisionDetection,
  type DragEndEvent,
  type DragStartEvent,
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
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { computeNestCandidates } from "@/lib/sidebar/nest-candidates";
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

/** Shared mouse/touch activation constraints for the sidebar tree drags. */
export function taskSwitcherDragActivationConstraints() {
  return TASK_SWITCHER_DRAG_ACTIVATION_CONSTRAINTS;
}

// Skip layout/paint for rows outside the sidebar scrollport so long task lists
// stay cheap to render and scroll. `auto 52px` seeds the height estimate before
// first paint; afterwards the browser remembers each row's real size, so
// scrollbar/anchor behavior stays stable and dnd-kit's drag-start rect
// measurements remain accurate for previously rendered rows.
const OFFSCREEN_SKIP = "[content-visibility:auto] [contain-intrinsic-block-size:auto_52px]";

// Nest drop zones are registered as `nest:<taskId>` droppables. A drop on one
// re-parents the dragged task under that task; any other drop keeps the
// existing sibling-reorder semantics.
export const NEST_DROPPABLE_PREFIX = "nest:";

/** Droppable id of a row's nest drop zone: `nest:<taskId>`. */
export function nestDroppableId(taskId: string): string {
  return NEST_DROPPABLE_PREFIX + taskId;
}

/** Returns the task id when `id` is a nest droppable id, else null. */
export function parseNestDroppableId(id: string): string | null {
  return id.startsWith(NEST_DROPPABLE_PREFIX) ? id.slice(NEST_DROPPABLE_PREFIX.length) : null;
}

export type SidebarDropDecision =
  | { kind: "nest"; parentTaskId: string }
  | { kind: "reorder-group"; orderedTaskIds: string[] }
  | { kind: "reorder-subtasks"; parentTaskId: string; orderedTaskIds: string[] };

/**
 * Maps a group-level drag end onto the single intended action.
 *
 * A drop on a `nest:` droppable re-parents the dragged task. Any other drop
 * reorders only when both tasks share a sibling level (group root list or one
 * parent's children); cross-level drops are a no-op, matching the previous
 * per-level behavior where a drag could never leave its own level.
 */
export function resolveSidebarDrop(args: {
  activeId: string;
  overId: string | null;
  groupRootIds: string[];
  childrenByParent: Map<string, string[]>;
}): SidebarDropDecision | null {
  const { activeId, overId, groupRootIds, childrenByParent } = args;
  if (!overId || overId === activeId) return null;

  const nestTarget = parseNestDroppableId(overId);
  if (nestTarget !== null) {
    if (nestTarget === activeId) return null;
    return { kind: "nest", parentTaskId: nestTarget };
  }

  const levelOf = (taskId: string): string | null => {
    if (groupRootIds.includes(taskId)) return GROUP_ROOT_LEVEL;
    for (const [parentId, ids] of childrenByParent) {
      if (ids.includes(taskId)) return parentId;
    }
    return null;
  };
  const activeLevel = levelOf(activeId);
  const overLevel = levelOf(overId);
  if (activeLevel === null || activeLevel !== overLevel) return null;

  const levelIds =
    activeLevel === GROUP_ROOT_LEVEL ? groupRootIds : (childrenByParent.get(activeLevel) ?? []);
  const oldIndex = levelIds.indexOf(activeId);
  const newIndex = levelIds.indexOf(overId);
  if (oldIndex < 0 || newIndex < 0) return null;
  const ordered = arrayMove(levelIds, oldIndex, newIndex);
  if (activeLevel === GROUP_ROOT_LEVEL) {
    return { kind: "reorder-group", orderedTaskIds: ordered };
  }
  return { kind: "reorder-subtasks", parentTaskId: activeLevel, orderedTaskIds: ordered };
}

// Sentinel level id for the group root list inside resolveSidebarDrop.
const GROUP_ROOT_LEVEL = "__group_root__";

/**
 * Computes the rows a drag may nest the active task under, mirroring the
 * context menu's candidate rules (computeNestCandidates) scoped to the
 * active task's workflow: same-workflow roots, excluding the task itself,
 * its current parent, and any subtask (which also excludes its descendants).
 */
export function computeNestTargets(
  activeTask: Pick<TaskSwitcherItem, "id" | "workflowId" | "parentTaskId"> | undefined,
  groupTasks: TaskSwitcherItem[],
): Set<string> {
  if (!activeTask) return new Set();
  const sameWorkflow = groupTasks.filter((t) => t.workflowId === activeTask.workflowId);
  return new Set(computeNestCandidates(sameWorkflow, activeTask.id).map((t) => t.id));
}

/**
 * The drop target rendered on a row that the active drag may nest under.
 * Absolute strip at the row's left edge; dropping on it re-parents the
 * dragged task instead of reordering. Rendered only during an active drag
 * with valid targets, so it never intercepts normal row clicks.
 */
export function NestDropZone({ taskId, title }: { taskId: string; title: string }) {
  const { t } = useTranslation();
  const { setNodeRef, isOver } = useDroppable({ id: nestDroppableId(taskId) });
  const label = t("sidebar:nestUnder", { title });
  return (
    <div
      ref={setNodeRef}
      data-testid="nest-drop-zone"
      data-task-id={taskId}
      aria-label={label}
      className={cn(
        "absolute inset-y-0 left-0 z-20 w-6",
        isOver ? "bg-primary/20" : "bg-transparent hover:bg-accent/40",
      )}
    >
      {isOver && (
        <span className="pointer-events-none absolute left-6 top-1/2 -translate-y-1/2 whitespace-nowrap rounded border bg-background px-2 py-0.5 text-xs shadow">
          {label}
        </span>
      )}
    </div>
  );
}

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
 *
 * By default each level owns its own `DndContext`, which confines every drag
 * to its sibling level. When `externalDragContext` is set the caller already
 * rendered a group-spanning `DndContext` (see `TaskTreeDndGroup`) so cross-
 * level drops like nest-under can be resolved — this level only contributes
 * its `SortableContext` and nodes.
 */
export function SortableTaskLevel({
  tasks,
  onReorder,
  renderNode,
  externalDragContext = false,
}: {
  tasks: TaskSwitcherItem[];
  onReorder?: (orderedTaskIds: string[]) => void;
  renderNode: (task: TaskSwitcherItem, isDraggable: boolean) => React.ReactNode;
  externalDragContext?: boolean;
}) {
  const { sensors, handleDragEnd, sortableIds } = useLevelDnd(tasks, onReorder);
  const isDraggable = !!onReorder;
  const body = tasks.map((task) => renderNode(task, isDraggable));
  if (!onReorder) return <>{body}</>;
  const sortable = (
    <SortableContext items={sortableIds} strategy={verticalListSortingStrategy}>
      {body}
    </SortableContext>
  );
  if (externalDragContext) return sortable;
  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      {sortable}
    </DndContext>
  );
}

/**
 * One group-spanning DndContext for the sidebar task tree. Replaces the
 * per-level contexts so a drag can target rows outside its own sibling level:
 * reorder stays scoped to the level the drag started in (resolveSidebarDrop),
 * and dropping on a `nest:` zone re-parents the dragged task.
 *
 * `children` receives the nest-target ids and the active drag id so rows can
 * render their `NestDropZone` affordance only when they are valid targets.
 */
export function TaskTreeDndGroup({
  groupTasks,
  onReorderGroup,
  onReorderSubtasks,
  onNestTask,
  children,
}: {
  groupTasks: TaskSwitcherItem[];
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  onNestTask?: (activeTaskId: string, parentTaskId: string) => void;
  children: (nestTargetIds: Set<string>, activeDragId: string | null) => React.ReactNode;
}) {
  const [activeDragId, setActiveDragId] = useState<string | null>(null);
  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: TASK_SWITCHER_DRAG_ACTIVATION_CONSTRAINTS.pointer,
    }),
    useSensor(TouchSensor, {
      activationConstraint: TASK_SWITCHER_DRAG_ACTIVATION_CONSTRAINTS.touch,
    }),
  );
  const enabled = !!onReorderGroup || !!onReorderSubtasks || !!onNestTask;
  const { tasksById, groupRootIds, childrenByParent } = useMemo(() => {
    const ids = new Set(groupTasks.map((t) => t.id));
    const byParent = new Map<string, string[]>();
    for (const t of groupTasks) {
      if (t.parentTaskId && ids.has(t.parentTaskId)) {
        const arr = byParent.get(t.parentTaskId) ?? [];
        arr.push(t.id);
        byParent.set(t.parentTaskId, arr);
      }
    }
    return {
      tasksById: new Map(groupTasks.map((t) => [t.id, t])),
      groupRootIds: groupTasks
        .filter((t) => !t.parentTaskId || !ids.has(t.parentTaskId))
        .map((t) => t.id),
      childrenByParent: byParent,
    };
  }, [groupTasks]);
  const nestTargetIds = useMemo(
    () => computeNestTargets(activeDragId ? tasksById.get(activeDragId) : undefined, groupTasks),
    [activeDragId, tasksById, groupTasks],
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveDragId(String(event.active.id));
  }, []);
  const handleDragCancel = useCallback(() => setActiveDragId(null), []);
  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      setActiveDragId(null);
      const { active, over } = event;
      if (!over) return;
      const decision = resolveSidebarDrop({
        activeId: String(active.id),
        overId: String(over.id),
        groupRootIds,
        childrenByParent,
      });
      if (!decision) return;
      switch (decision.kind) {
        case "nest":
          onNestTask?.(String(active.id), decision.parentTaskId);
          break;
        case "reorder-group":
          onReorderGroup?.(decision.orderedTaskIds);
          break;
        case "reorder-subtasks":
          onReorderSubtasks?.(decision.parentTaskId, decision.orderedTaskIds);
          break;
      }
    },
    [groupRootIds, childrenByParent, onReorderGroup, onReorderSubtasks, onNestTask],
  );

  const collisionDetection = useCallback<CollisionDetection>((args) => {
    // A pointer resting on a nest zone always means "nest under this row";
    // everywhere else falls back to the nearest-row reorder.
    const within = pointerWithin(args);
    const nest = within.filter((collision) =>
      collision.id.toString().startsWith(NEST_DROPPABLE_PREFIX),
    );
    if (nest.length > 0) return nest;
    return closestCenter(args);
  }, []);

  if (!enabled) return <>{children(new Set(), null)}</>;
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={collisionDetection}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onDragCancel={handleDragCancel}
    >
      {children(nestTargetIds, activeDragId)}
    </DndContext>
  );
}
