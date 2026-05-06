"use client";

import { memo, useCallback, useMemo } from "react";
import { IconChevronDown } from "@tabler/icons-react";
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
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import { cn } from "@/lib/utils";
import { TaskItem } from "./task-item";
import { TaskItemWithContextMenu, type StepDef } from "./task-switcher-context-menu";
import type { GroupedSidebarList, SidebarGroup } from "@/lib/sidebar/apply-view";
import { type TaskMoveWorkflow } from "@/components/task/task-move-context-menu";

const DRAG_ACTIVATION_DISTANCE = 8;

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
  parentTaskTitle?: string;
  parentTaskId?: string;
  prInfo?: { number: number; state: string };
  isPRReview?: boolean;
  isIssueWatch?: boolean;
  issueInfo?: { url: string; number: number };
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
        updatedAt={task.updatedAt}
        repositories={task.repositories}
        prInfo={task.prInfo}
        issueInfo={task.issueInfo}
        isSubTask={isSubTask}
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

function SortableTaskBlock({
  taskId,
  parent,
  subTasks,
}: {
  taskId: string;
  parent: React.ReactNode;
  subTasks?: React.ReactNode;
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
  // setNodeRef stays on the outer wrapper so the CSS transform applies to
  // the parent row + its subtasks together. Drag listeners only attach to
  // the parent row's wrapper, so a pointer-down on a subtask row does NOT
  // trigger a drag of the parent block.
  return (
    <div
      ref={setNodeRef}
      style={style}
      data-testid="sortable-task-block"
      data-task-id={taskId}
      className={cn(isDragging && "z-50")}
    >
      <div
        {...sortableAttributes}
        {...listeners}
        // Strip dnd-kit's default tabIndex={0}: only PointerSensor is wired,
        // so keyboard tab stops here lead nowhere. If KeyboardSensor is
        // added later, drop this override.
        tabIndex={undefined}
        data-testid="sortable-task-handle"
        className="cursor-grab active:cursor-grabbing"
      >
        {parent}
      </div>
      {subTasks}
    </div>
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
  pinnedSet: Set<string>;
  deletingTaskId?: string | null;
};

function useGroupDnd(
  groupTasks: TaskSwitcherItem[],
  onReorderGroup?: (groupTaskIds: string[]) => void,
) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: DRAG_ACTIVATION_DISTANCE } }),
  );
  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      if (!onReorderGroup) return;
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const ids = groupTasks.map((t) => t.id);
      const oldIndex = ids.indexOf(String(active.id));
      const newIndex = ids.indexOf(String(over.id));
      if (oldIndex < 0 || newIndex < 0) return;
      onReorderGroup(arrayMove(ids, oldIndex, newIndex));
    },
    [groupTasks, onReorderGroup],
  );
  const sortableIds = useMemo(() => groupTasks.map((t) => t.id), [groupTasks]);
  return { sensors, handleDragEnd, sortableIds };
}

function GroupTaskList({
  group,
  subTasksByParentId,
  collapsedSubs,
  onToggleSubtasks,
  pinnedSet,
  rowProps,
}: {
  group: SidebarGroup;
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  collapsedSubs: Set<string>;
  onToggleSubtasks?: (parentTaskId: string) => void;
  pinnedSet: Set<string>;
  rowProps: Omit<TaskRowProps, "task" | "subtaskToggle" | "isPinned" | "isSubTask">;
}) {
  return (
    <>
      {group.tasks.map((task) => {
        const subs = subTasksByParentId.get(task.id);
        const hasSubs = !!subs?.length;
        const subsHidden = hasSubs && !!onToggleSubtasks && collapsedSubs.has(task.id);
        const toggleInfo: SubtaskToggleInfo | undefined =
          hasSubs && onToggleSubtasks
            ? {
                subtaskCount: subs!.length,
                subtasksCollapsed: subsHidden,
                onToggleSubtasks: () => onToggleSubtasks(task.id),
              }
            : undefined;
        return (
          <SortableTaskBlock
            key={task.id}
            taskId={task.id}
            parent={
              <TaskRow
                task={task}
                subtaskToggle={toggleInfo}
                isPinned={pinnedSet.has(task.id)}
                {...rowProps}
              />
            }
            subTasks={
              !subsHidden &&
              subs?.map((sub) => (
                // Subtasks aren't independently sortable or pinnable — pinning
                // would show an icon but `floatPinnedToTop` only operates on
                // root tasks, so the row wouldn't move. Drop both props to
                // avoid the misleading no-op menu item.
                <TaskRow key={sub.id} task={sub} isSubTask {...rowProps} onTogglePin={undefined} />
              ))
            }
          />
        );
      })}
    </>
  );
}

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
  pinnedSet,
  deletingTaskId,
}: GroupSectionProps) {
  const totalCount = group.tasks.reduce(
    (sum, t) => sum + 1 + (subTasksByParentId.get(t.id)?.length ?? 0),
    0,
  );
  const collapsedSubs = new Set(collapsedSubtaskParentIds ?? []);
  const rowProps = {
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
  };
  const { sensors, handleDragEnd, sortableIds } = useGroupDnd(group.tasks, onReorderGroup);

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
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={sortableIds} strategy={verticalListSortingStrategy}>
            <GroupTaskList
              group={group}
              subTasksByParentId={subTasksByParentId}
              collapsedSubs={collapsedSubs}
              onToggleSubtasks={onToggleSubtasks}
              pinnedSet={pinnedSet}
              rowProps={rowProps}
            />
          </SortableContext>
        </DndContext>
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
          pinnedSet={pinnedSet}
          deletingTaskId={deletingTaskId}
        />
      ))}
    </div>
  );
});
