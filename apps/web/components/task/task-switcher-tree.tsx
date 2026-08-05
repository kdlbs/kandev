"use client";

import { countGroupTasks, type SidebarGroup } from "@/lib/sidebar/apply-view";
import { SortableTaskLevel, SortableTaskNode } from "./task-switcher-subtask-dnd";
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
  const toggleInfo: SubtaskToggleInfo | undefined =
    hasSubs && ctx.onToggleSubtasks
      ? {
          subtaskCount: countGroupTasks(subs, ctx.subTasksByParentId),
          subtasksCollapsed: subsHidden,
          onToggleSubtasks: () => ctx.onToggleSubtasks!(task.id),
        }
      : undefined;
  const isRoot = depth === 0;
  const handle = (
    <TaskRow
      task={task}
      depth={depth}
      isSubTask={!isRoot}
      subtaskToggle={toggleInfo}
      isPinned={isRoot && ctx.pinnedSet.has(task.id)}
      {...ctx.rowProps}
      onTogglePin={isRoot ? ctx.rowProps.onTogglePin : undefined}
    />
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
};

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
}: GroupSectionProps) {
  const totalCount = countGroupTasks(group.tasks, subTasksByParentId);
  const ctx: TaskTreeContext = {
    subTasksByParentId,
    collapsedSubs: new Set(collapsedSubtaskParentIds ?? []),
    onToggleSubtasks,
    pinnedSet,
    rowProps,
    onReorderGroup,
    onReorderSubtasks,
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
      {!isCollapsed && (
        <TaskTreeLevel parentTaskId={null} tasks={group.tasks} depth={0} ctx={ctx} />
      )}
    </div>
  );
}
