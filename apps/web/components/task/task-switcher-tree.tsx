"use client";

import { memo, useCallback, useMemo } from "react";
import { countGroupTasks, type SidebarGroup } from "@/lib/sidebar/apply-view";
import {
  SortableTaskLevel,
  SortableTaskNode,
  TaskTreeDndGroup,
  NestDropZone,
} from "./task-switcher-subtask-dnd";
import { GroupHeader } from "./task-switcher-group";
import { TaskRow, type SubtaskToggleInfo, type TaskRowProps } from "./task-switcher-row";
import type { TaskSwitcherItem } from "./task-switcher-types";

export type TaskRowBaseProps = Omit<
  TaskRowProps,
  "task" | "subtaskToggle" | "isPinned" | "isSubTask" | "depth"
>;

type TaskTreeContext = {
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  collapsedSubs: Set<string>;
  onToggleSubtasks?: (parentTaskId: string) => void;
  pinnedSet: Set<string>;
  rowProps: TaskRowBaseProps;
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  onNestTask?: (taskId: string, parentTaskId: string) => void;
  // Rows in nestTargetIds render a nest drop zone while a drag is active.
  nestTargetIds: Set<string>;
  // The group renders one DndContext spanning every level (TaskTreeDndGroup),
  // so levels must not create their own.
  externalDragContext: boolean;
};

type TaskTreeNodeProps = {
  task: TaskSwitcherItem;
  depth: number;
  ctx: TaskTreeContext;
  isDraggable: boolean;
};

function nodeSubtreeEqual(
  taskId: string,
  previous: TaskTreeContext,
  next: TaskTreeContext,
  visited: Set<string>,
): boolean {
  if (visited.has(taskId)) return true;
  visited.add(taskId);
  if (
    previous.collapsedSubs.has(taskId) !== next.collapsedSubs.has(taskId) ||
    previous.nestTargetIds.has(taskId) !== next.nestTargetIds.has(taskId)
  ) {
    return false;
  }
  const previousChildren = previous.subTasksByParentId.get(taskId);
  const nextChildren = next.subTasksByParentId.get(taskId);
  if (!previousChildren || !nextChildren) return previousChildren === nextChildren;
  if (
    previousChildren.length !== nextChildren.length ||
    previousChildren.some((task, index) => task !== nextChildren[index])
  ) {
    return false;
  }
  return previousChildren.every((task) => nodeSubtreeEqual(task.id, previous, next, visited));
}

function taskTreeNodeEqual(previous: TaskTreeNodeProps, next: TaskTreeNodeProps): boolean {
  // A node owns its rendered descendants, so its equality includes only that
  // branch's collapse, nest-target, and task references from the shared maps.
  if (
    previous.task !== next.task ||
    previous.depth !== next.depth ||
    previous.isDraggable !== next.isDraggable ||
    previous.ctx.rowProps !== next.ctx.rowProps ||
    previous.ctx.onToggleSubtasks !== next.ctx.onToggleSubtasks ||
    previous.ctx.onReorderGroup !== next.ctx.onReorderGroup ||
    previous.ctx.onReorderSubtasks !== next.ctx.onReorderSubtasks ||
    previous.ctx.onNestTask !== next.ctx.onNestTask ||
    previous.ctx.externalDragContext !== next.ctx.externalDragContext ||
    (previous.depth === 0 &&
      previous.ctx.pinnedSet.has(previous.task.id) !== next.ctx.pinnedSet.has(next.task.id))
  ) {
    return false;
  }
  return nodeSubtreeEqual(previous.task.id, previous.ctx, next.ctx, new Set());
}

const TaskTreeNode = memo(function TaskTreeNode({
  task,
  depth,
  ctx,
  isDraggable,
}: TaskTreeNodeProps) {
  const subs = ctx.subTasksByParentId.get(task.id);
  const hasSubs = !!subs?.length;
  const subsHidden = hasSubs && !!ctx.onToggleSubtasks && ctx.collapsedSubs.has(task.id);
  const subtaskCount = hasSubs ? countGroupTasks(subs, ctx.subTasksByParentId) : 0;
  const handleToggleSubtasks = useCallback(
    () => ctx.onToggleSubtasks?.(task.id),
    [ctx.onToggleSubtasks, task.id],
  );
  const toggleInfo: SubtaskToggleInfo | undefined = useMemo(
    () =>
      hasSubs && ctx.onToggleSubtasks
        ? {
            subtaskCount,
            subtasksCollapsed: subsHidden,
            onToggleSubtasks: handleToggleSubtasks,
          }
        : undefined,
    [handleToggleSubtasks, hasSubs, ctx.onToggleSubtasks, subsHidden, subtaskCount],
  );
  const isRoot = depth === 0;
  const isNestTarget = ctx.nestTargetIds.has(task.id);
  const handle = (
    // The relative wrapper keeps the nest drop zone pinned to this row's
    // left edge rather than spanning the nested subtree below it.
    <div className="relative">
      <TaskRow
        task={task}
        depth={depth}
        isSubTask={!isRoot}
        subtaskToggle={toggleInfo}
        isPinned={isRoot && ctx.pinnedSet.has(task.id)}
        {...ctx.rowProps}
        onTogglePin={isRoot ? ctx.rowProps.onTogglePin : undefined}
      />
      {isNestTarget && <NestDropZone taskId={task.id} title={task.title} />}
    </div>
  );
  const nested =
    !subsHidden && hasSubs ? (
      <TaskTreeLevel parentTaskId={task.id} tasks={subs} depth={depth + 1} ctx={ctx} />
    ) : undefined;
  return (
    <SortableTaskNode
      taskId={task.id}
      depth={depth}
      handle={handle}
      nested={nested}
      isDraggable={isDraggable}
    />
  );
}, taskTreeNodeEqual);

function TaskTreeLevel({
  parentTaskId,
  tasks,
  depth,
  ctx,
}: {
  parentTaskId: string | null;
  tasks: TaskSwitcherItem[];
  depth: number;
  ctx: TaskTreeContext;
}) {
  const onReorder = getReorderHandler(parentTaskId, ctx);
  return (
    <SortableTaskLevel
      tasks={tasks}
      onReorder={onReorder}
      externalDragContext={ctx.externalDragContext}
      renderNode={(task, levelDraggable) => (
        <TaskTreeNode
          key={task.id}
          task={task}
          depth={depth}
          ctx={ctx}
          isDraggable={levelDraggable}
        />
      )}
    />
  );
}

function getReorderHandler(parentTaskId: string | null, ctx: TaskTreeContext) {
  if (parentTaskId === null) return ctx.onReorderGroup;
  if (!ctx.onReorderSubtasks) return undefined;
  return (ids: string[]) => ctx.onReorderSubtasks!(parentTaskId, ids);
}

export type GroupSectionProps = {
  group: SidebarGroup;
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  rowProps: TaskRowBaseProps;
  pinnedSet: Set<string>;
  isCollapsed: boolean;
  onToggleGroup?: (groupKey: string) => void;
  collapsedSubtaskParentIds?: string[];
  onToggleSubtasks?: (parentTaskId: string) => void;
  showHeader: boolean;
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  onNestTask?: (taskId: string, parentTaskId: string) => void;
};

function sameTaskSubtree(
  previousTasks: TaskSwitcherItem[],
  nextTasks: TaskSwitcherItem[],
  previousMap: Map<string, TaskSwitcherItem[]>,
  nextMap: Map<string, TaskSwitcherItem[]>,
  visited = new Set<string>(),
): boolean {
  if (
    previousTasks.length !== nextTasks.length ||
    previousTasks.some((task, index) => task !== nextTasks[index])
  ) {
    return false;
  }
  for (const task of previousTasks) {
    if (visited.has(task.id)) continue;
    visited.add(task.id);
    const previousChildren = previousMap.get(task.id);
    const nextChildren = nextMap.get(task.id);
    if (!previousChildren && !nextChildren) continue;
    if (
      !previousChildren ||
      !nextChildren ||
      !sameTaskSubtree(previousChildren, nextChildren, previousMap, nextMap, visited)
    ) {
      return false;
    }
  }
  return true;
}

function groupSectionEqual(previous: GroupSectionProps, next: GroupSectionProps): boolean {
  if (
    previous.group !== next.group ||
    previous.rowProps !== next.rowProps ||
    previous.pinnedSet !== next.pinnedSet ||
    previous.isCollapsed !== next.isCollapsed ||
    previous.onToggleGroup !== next.onToggleGroup ||
    previous.collapsedSubtaskParentIds !== next.collapsedSubtaskParentIds ||
    previous.onToggleSubtasks !== next.onToggleSubtasks ||
    previous.showHeader !== next.showHeader ||
    previous.onReorderGroup !== next.onReorderGroup ||
    previous.onReorderSubtasks !== next.onReorderSubtasks ||
    previous.onNestTask !== next.onNestTask
  ) {
    return false;
  }
  return (
    previous.subTasksByParentId === next.subTasksByParentId ||
    sameTaskSubtree(
      previous.group.tasks,
      next.group.tasks,
      previous.subTasksByParentId,
      next.subTasksByParentId,
    )
  );
}

/**
 * Flattens one group's subtree (roots + every descendant, in rendered order)
 * so the group-spanning DndContext can resolve reorder levels and nest
 * targets without consulting other groups' children. Tracks visited ids so a
 * corrupted cycle in `subTasksByParentId` cannot recurse forever (mirrors
 * countGroupTasks' protection).
 */
function flattenGroupTasks(
  roots: TaskSwitcherItem[],
  subTasksByParentId: Map<string, TaskSwitcherItem[]>,
): TaskSwitcherItem[] {
  const out: TaskSwitcherItem[] = [];
  const visited = new Set<string>();
  const visit = (task: TaskSwitcherItem) => {
    if (visited.has(task.id)) return;
    visited.add(task.id);
    out.push(task);
    const subs = subTasksByParentId.get(task.id);
    if (subs) {
      for (const sub of subs) visit(sub);
    }
  };
  for (const root of roots) visit(root);
  return out;
}

/**
 * Renders one sidebar group's task tree (header + the recursive tree wrapped
 * in the group-spanning DndContext that enables nest-by-drag).
 */
export const GroupSection = memo(function GroupSection({
  group,
  subTasksByParentId,
  rowProps,
  pinnedSet,
  isCollapsed,
  onToggleGroup,
  collapsedSubtaskParentIds,
  onToggleSubtasks,
  showHeader,
  onReorderGroup,
  onReorderSubtasks,
  onNestTask,
}: GroupSectionProps) {
  const totalCount = countGroupTasks(group.tasks, subTasksByParentId);
  const groupTasks = useMemo(
    () => flattenGroupTasks(group.tasks, subTasksByParentId),
    [group.tasks, subTasksByParentId],
  );

  const renderTree = (nestTargetIds: Set<string>) => {
    if (isCollapsed) return null;
    const ctx: TaskTreeContext = {
      subTasksByParentId,
      collapsedSubs: new Set(collapsedSubtaskParentIds ?? []),
      onToggleSubtasks,
      pinnedSet,
      rowProps,
      onReorderGroup,
      onReorderSubtasks,
      onNestTask,
      nestTargetIds,
      externalDragContext: true,
    };
    return <TaskTreeLevel parentTaskId={null} tasks={group.tasks} depth={0} ctx={ctx} />;
  };

  return (
    <div>
      {showHeader && (
        <GroupHeader
          label={group.label}
          groupKey={group.key}
          count={totalCount}
          isCollapsed={isCollapsed}
          onToggle={() => onToggleGroup?.(group.key)}
        />
      )}
      <TaskTreeDndGroup
        groupTasks={groupTasks}
        onReorderGroup={onReorderGroup}
        onReorderSubtasks={onReorderSubtasks}
        onNestTask={onNestTask}
      >
        {renderTree}
      </TaskTreeDndGroup>
    </div>
  );
}, groupSectionEqual);
