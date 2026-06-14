"use client";

import { memo, useMemo } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import { cn } from "@/lib/utils";
import { TaskItem } from "./task-item";
import { TaskItemWithContextMenu, type StepDef } from "./task-switcher-context-menu";
import {
  countGroupTasks,
  type GroupedSidebarList,
  type SidebarGroup,
} from "@/lib/sidebar/apply-view";
import { type TaskMoveWorkflow } from "@/components/task/task-move-context-menu";
import { SortableTaskLevel, SortableTaskNode } from "./task-switcher-subtask-dnd";

export type TaskSwitcherItem = {
  id: string;
  title: string;
  state?: TaskState;
  sessionState?: TaskSessionState;
  description?: string;
  workflowId?: string;
  workflowName?: string;
  workflowStepId?: string;
  workflowStepTitle?: string;
  repositoryPath?: string;
  repositories?: string[];
  diffStats?: { additions: number; deletions: number };
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  createdAt?: string;
  isArchived?: boolean;
  primarySessionId?: string | null;
  hasPendingClarification?: boolean;
  hasPendingPermission?: boolean;
  parentTaskTitle?: string;
  parentTaskId?: string;
  prInfo?: { number: number; state: string };
  isPRReview?: boolean;
  isIssueWatch?: boolean;
  issueInfo?: { url: string; number: number };
  agentErrorMessage?: string | null;
};

type TaskSwitcherProps = {
  grouped: GroupedSidebarList;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  collapsedGroupKeys?: string[];
  onToggleGroup?: (groupKey: string) => void;
  collapsedSubtaskParentIds?: string[];
  onToggleSubtasks?: (parentTaskId: string) => void;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  pinnedTaskIds?: string[];
  deletingTaskId?: string | null;
  isLoading?: boolean;
  totalTaskCount?: number;
};

type SubtaskToggleInfo = {
  subtaskCount: number;
  subtasksCollapsed: boolean;
  onToggleSubtasks: () => void;
};

function TaskSwitcherSkeleton() {
  return (
    <div className="animate-pulse">
      <div className="h-10 bg-foreground/5" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
    </div>
  );
}

function GroupHeader({
  label,
  groupKey,
  count,
  isCollapsed,
  onToggle,
}: {
  label: string;
  groupKey: string;
  count: number;
  isCollapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      data-testid="sidebar-group-header"
      data-group-key={groupKey}
      data-group-label={label}
      className="flex w-full items-center gap-2 bg-background px-3 py-1.5 cursor-pointer hover:bg-foreground/[0.03]"
    >
      <span className="flex-1 truncate text-left text-[12px] font-medium text-foreground/80">
        {label}
      </span>
      <span className="text-[11px] text-muted-foreground/50">{count}</span>
      <IconChevronDown
        className={cn(
          "h-3 w-3 text-muted-foreground/40 transition-transform",
          isCollapsed && "-rotate-90",
        )}
      />
    </button>
  );
}

type TaskRowProps = {
  task: TaskSwitcherItem;
  isSubTask?: boolean;
  depth?: number;
  subtaskToggle?: SubtaskToggleInfo;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  isPinned?: boolean;
  deletingTaskId?: string | null;
};

function TaskRow({
  task,
  isSubTask,
  depth,
  subtaskToggle,
  workflows,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  onTogglePin,
  isPinned,
  deletingTaskId,
}: TaskRowProps) {
  const isSelected = task.id === selectedTaskId || task.id === activeTaskId;
  const taskSteps = task.workflowId ? stepsByWorkflowId?.[task.workflowId] : undefined;
  return (
    <TaskItemWithContextMenu
      task={task}
      workflows={workflows}
      stepsByWorkflowId={stepsByWorkflowId}
      steps={taskSteps}
      onRenameTask={onRenameTask}
      onArchiveTask={onArchiveTask}
      onDeleteTask={onDeleteTask}
      onMoveToStep={onMoveToStep}
      onTogglePin={onTogglePin}
      isPinned={isPinned}
      isDeleting={deletingTaskId === task.id}
    >
      <TaskItem
        title={task.title}
        state={task.state}
        sessionState={task.sessionState}
        isArchived={task.isArchived}
        isSelected={isSelected}
        diffStats={task.diffStats}
        isRemoteExecutor={task.isRemoteExecutor}
        remoteExecutorType={task.remoteExecutorType}
        remoteExecutorName={task.remoteExecutorName}
        taskId={task.id}
        primarySessionId={task.primarySessionId ?? null}
        hasPendingClarification={task.hasPendingClarification}
        hasPendingPermission={task.hasPendingPermission}
        updatedAt={task.updatedAt}
        repositories={task.repositories}
        prInfo={task.prInfo}
        issueInfo={task.issueInfo}
        agentErrorMessage={task.agentErrorMessage}
        isSubTask={isSubTask}
        depth={depth}
        subtaskCount={subtaskToggle?.subtaskCount}
        subtasksCollapsed={subtaskToggle?.subtasksCollapsed}
        onToggleSubtasks={subtaskToggle?.onToggleSubtasks}
        onClick={() => onSelectTask(task.id)}
        isDeleting={deletingTaskId === task.id}
        isPinned={isPinned}
      />
    </TaskItemWithContextMenu>
  );
}

// Shared, per-render context threaded through the recursive task tree so each
// node can look up its children, collapse state, and reorder callbacks without
// drilling a dozen props through every level.
type TaskTreeContext = {
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  collapsedSubs: Set<string>;
  onToggleSubtasks?: (parentTaskId: string) => void;
  pinnedSet: Set<string>;
  rowProps: Omit<TaskRowProps, "task" | "subtaskToggle" | "isPinned" | "isSubTask" | "depth">;
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
};

// One task row plus — when expanded — its nested subtree. Mutually recursive
// with TaskTreeLevel, so it renders arbitrarily deep hierarchies.
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
          subtaskCount: countGroupTasks(subs!, ctx.subTasksByParentId),
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
      // Only root tasks are pinnable — `floatPinnedToTop` reorders root tasks
      // only, so a pin on a nested row would show an icon but never move it.
      onTogglePin={isRoot ? ctx.rowProps.onTogglePin : undefined}
    />
  );
  const nested =
    !subsHidden && hasSubs ? (
      <TaskTreeLevel parentTaskId={task.id} tasks={subs!} depth={depth + 1} ctx={ctx} />
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

// One level of sibling tasks. `parentTaskId === null` is the group root (whose
// reorder maps to onReorderGroup); deeper levels reorder via onReorderSubtasks
// scoped to that parent's children.
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
  let onReorder: ((orderedTaskIds: string[]) => void) | undefined;
  if (parentTaskId === null) {
    onReorder = ctx.onReorderGroup;
  } else if (ctx.onReorderSubtasks) {
    const pid = parentTaskId;
    onReorder = (ids: string[]) => ctx.onReorderSubtasks!(pid, ids);
  }
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

type GroupSectionProps = {
  group: SidebarGroup;
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  isCollapsed: boolean;
  onToggleCollapsed: () => void;
  collapsedSubtaskParentIds?: string[];
  onToggleSubtasks?: (parentTaskId: string) => void;
  showHeader: boolean;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  onReorderGroup?: (groupTaskIds: string[]) => void;
  onReorderSubtasks?: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
  pinnedSet: Set<string>;
  deletingTaskId?: string | null;
};

function GroupSection({
  group,
  subTasksByParentId,
  workflows,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  isCollapsed,
  onToggleCollapsed,
  collapsedSubtaskParentIds,
  onToggleSubtasks,
  showHeader,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  onTogglePin,
  onReorderGroup,
  onReorderSubtasks,
  pinnedSet,
  deletingTaskId,
}: GroupSectionProps) {
  const totalCount = countGroupTasks(group.tasks, subTasksByParentId);
  const ctx: TaskTreeContext = {
    subTasksByParentId,
    collapsedSubs: new Set(collapsedSubtaskParentIds ?? []),
    onToggleSubtasks,
    pinnedSet,
    rowProps: {
      workflows,
      stepsByWorkflowId,
      activeTaskId,
      selectedTaskId,
      onSelectTask,
      onRenameTask,
      onArchiveTask,
      onDeleteTask,
      onMoveToStep,
      onTogglePin,
      deletingTaskId,
    },
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

export const TaskSwitcher = memo(function TaskSwitcher({
  grouped,
  workflows,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  collapsedGroupKeys = [],
  onToggleGroup,
  collapsedSubtaskParentIds,
  onToggleSubtasks,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  onTogglePin,
  onReorderGroup,
  onReorderSubtasks,
  pinnedTaskIds,
  deletingTaskId,
  isLoading = false,
  totalTaskCount,
}: TaskSwitcherProps) {
  const pinnedSet = useMemo(() => new Set(pinnedTaskIds ?? []), [pinnedTaskIds]);
  if (isLoading) return <TaskSwitcherSkeleton />;
  const totalTasks = totalTaskCount ?? grouped.groups.reduce((sum, g) => sum + g.tasks.length, 0);
  if (totalTasks === 0) {
    return <div className="px-3 py-3 text-xs text-muted-foreground">No tasks yet.</div>;
  }

  const collapsedSet = new Set(collapsedGroupKeys);
  const showHeaders =
    grouped.groups.length > 1 ||
    (grouped.groups.length === 1 && grouped.groups[0].key !== "__all__");

  return (
    <div>
      {grouped.groups.map((group) => (
        <GroupSection
          key={group.key}
          group={group}
          subTasksByParentId={grouped.subTasksByParentId}
          workflows={workflows}
          stepsByWorkflowId={stepsByWorkflowId}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          isCollapsed={collapsedSet.has(group.key)}
          onToggleCollapsed={() => onToggleGroup?.(group.key)}
          collapsedSubtaskParentIds={collapsedSubtaskParentIds}
          onToggleSubtasks={onToggleSubtasks}
          showHeader={showHeaders}
          onSelectTask={onSelectTask}
          onRenameTask={onRenameTask}
          onArchiveTask={onArchiveTask}
          onDeleteTask={onDeleteTask}
          onMoveToStep={onMoveToStep}
          onTogglePin={onTogglePin}
          onReorderGroup={onReorderGroup}
          onReorderSubtasks={onReorderSubtasks}
          pinnedSet={pinnedSet}
          deletingTaskId={deletingTaskId}
        />
      ))}
    </div>
  );
});
