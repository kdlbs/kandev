"use client";

import { useCallback, useMemo } from "react";
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
  activeDragId: string | null;
  // The group renders one DndContext spanning every level (TaskTreeDndGroup),
  // so levels must not create their own.
  externalDragContext: boolean;
};

function TaskTreeNode({
  task,
  depth,
  ctx,
  isDraggable,
}: {
  task: TaskSwitcherItem;
  depth: number;
  ctx: TaskTreeContext;
  isDraggable: boolean;
}) {
  const subs = ctx.subTasksByParentId.get(task.id);
  const hasSubs = !!subs?.length;
  const subsHidden = hasSubs && !!ctx.onToggleSubtasks && ctx.collapsedSubs.has(task.id);
  const handleToggleSubtasks = useCallback(() => {
    ctx.onToggleSubtasks?.(task.id);
  }, [ctx.onToggleSubtasks, task.id]);
  const toggleInfo = useMemo<SubtaskToggleInfo | undefined>(
    () =>
      hasSubs && ctx.onToggleSubtasks
        ? {
            subtaskCount: countGroupTasks(subs, ctx.subTasksByParentId),
            subtasksCollapsed: subsHidden,
            onToggleSubtasks: handleToggleSubtasks,
          }
        : undefined,
    [ctx.onToggleSubtasks, ctx.subTasksByParentId, handleToggleSubtasks, hasSubs, subs, subsHidden],
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
}

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
  onToggleCollapsed: () => void;
  collapsedSubtaskParentIds?: string[];
  onToggleSubtasks?: (parentTaskId: string) => void;
  showHeader: boolean;
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  onNestTask?: (taskId: string, parentTaskId: string) => void;
};

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
export function GroupSection({
  group,
  subTasksByParentId,
  rowProps,
  pinnedSet,
  isCollapsed,
  onToggleCollapsed,
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

  const renderTree = (nestTargetIds: Set<string>, activeDragId: string | null) => {
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
      activeDragId,
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
          onToggle={onToggleCollapsed}
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
}
