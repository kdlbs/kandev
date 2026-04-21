"use client";

import { cloneElement, isValidElement, memo, useState } from "react";
import {
  IconChevronDown,
  IconArrowRight,
  IconPencil,
  IconCopy,
  IconArchive,
  IconTrash,
  IconLoader,
} from "@tabler/icons-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import { cn } from "@/lib/utils";
import { TaskItem } from "./task-item";
import type { GroupedSidebarList, SidebarGroup } from "@/lib/sidebar/apply-view";

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
  parentTaskTitle?: string;
  parentTaskId?: string;
  prInfo?: { number: number; state: string };
  isPRReview?: boolean;
  isIssueWatch?: boolean;
  issueInfo?: { url: string; number: number };
};

type StepDef = { id: string; title: string; color?: string };

type TaskSwitcherProps = {
  grouped: GroupedSidebarList;
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  collapsedGroupKeys?: string[];
  onToggleGroup?: (groupKey: string) => void;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  deletingTaskId?: string | null;
  isLoading?: boolean;
  totalTaskCount?: number;
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

function GroupSection({
  group,
  subTasksByParentId,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  isCollapsed,
  onToggleCollapsed,
  showHeader,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  deletingTaskId,
}: {
  group: SidebarGroup;
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  isCollapsed: boolean;
  onToggleCollapsed: () => void;
  showHeader: boolean;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  deletingTaskId?: string | null;
}) {
  const totalCount = group.tasks.reduce(
    (sum, t) => sum + 1 + (subTasksByParentId.get(t.id)?.length ?? 0),
    0,
  );

  function renderItem(task: TaskSwitcherItem, isSubTask?: boolean) {
    const isSelected = task.id === selectedTaskId || task.id === activeTaskId;
    const taskSteps = task.workflowId ? stepsByWorkflowId?.[task.workflowId] : undefined;
    return (
      <TaskItemWithContextMenu
        key={task.id}
        task={task}
        steps={taskSteps}
        onRenameTask={onRenameTask}
        onArchiveTask={onArchiveTask}
        onDeleteTask={onDeleteTask}
        onMoveToStep={onMoveToStep}
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
          updatedAt={task.updatedAt}
          repositories={task.repositories}
          prInfo={task.prInfo}
          issueInfo={task.issueInfo}
          isSubTask={isSubTask}
          onClick={() => onSelectTask(task.id)}
          isDeleting={deletingTaskId === task.id}
        />
      </TaskItemWithContextMenu>
    );
  }

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
      {!isCollapsed &&
        group.tasks.map((task) => (
          <div key={task.id}>
            {renderItem(task)}
            {subTasksByParentId.get(task.id)?.map((sub) => renderItem(sub, true))}
          </div>
        ))}
    </div>
  );
}

function TaskItemWithContextMenu({
  task,
  steps,
  children,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  isDeleting,
}: {
  task: TaskSwitcherItem;
  steps?: StepDef[];
  children: React.ReactElement<{ menuOpen?: boolean }>;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  isDeleting?: boolean;
}) {
  const [contextOpen, setContextOpen] = useState(false);
  const [menuKey, setMenuKey] = useState(0);
  const menuOpen = contextOpen || isDeleting === true;

  return (
    <ContextMenu key={menuKey} onOpenChange={setContextOpen}>
      <ContextMenuTrigger asChild>
        <div>{cloneWithMenuOpen(children, menuOpen)}</div>
      </ContextMenuTrigger>
      <ContextMenuContent className="w-48">
        {onRenameTask && (
          <ContextMenuItem disabled={isDeleting} onClick={() => onRenameTask(task.id, task.title)}>
            <IconPencil className="mr-2 h-4 w-4" />
            Rename
          </ContextMenuItem>
        )}
        <ContextMenuItem disabled>
          <IconCopy className="mr-2 h-4 w-4" />
          Duplicate
        </ContextMenuItem>
        {onArchiveTask && (
          <ContextMenuItem disabled={isDeleting} onClick={() => onArchiveTask(task.id)}>
            <IconArchive className="mr-2 h-4 w-4" />
            Archive
          </ContextMenuItem>
        )}
        {onMoveToStep && task.workflowId && steps && steps.length > 0 && (
          <MoveToStepSubmenu
            steps={steps}
            currentStepId={task.workflowStepId}
            disabled={isDeleting}
            onSelect={(stepId) => {
              setContextOpen(false);
              setMenuKey((k) => k + 1);
              onMoveToStep(task.id, task.workflowId!, stepId);
            }}
          />
        )}
        {onDeleteTask && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem
              variant="destructive"
              disabled={isDeleting}
              onClick={() => onDeleteTask(task.id)}
            >
              {isDeleting ? (
                <IconLoader className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <IconTrash className="mr-2 h-4 w-4" />
              )}
              Delete
            </ContextMenuItem>
          </>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}

function MoveToStepSubmenu({
  steps,
  currentStepId,
  disabled,
  onSelect,
}: {
  steps: StepDef[];
  currentStepId?: string;
  disabled?: boolean;
  onSelect: (stepId: string) => void;
}) {
  return (
    <>
      <ContextMenuSeparator />
      <ContextMenuSub>
        <ContextMenuSubTrigger disabled={disabled}>
          <IconArrowRight className="mr-2 h-4 w-4" />
          Move to
        </ContextMenuSubTrigger>
        <ContextMenuSubContent className="w-44">
          {steps.map((step) => (
            <ContextMenuItem
              key={step.id}
              disabled={step.id === currentStepId}
              onSelect={(e) => {
                e.preventDefault();
                onSelect(step.id);
              }}
            >
              <span className={cn("block h-2 w-2 rounded-full shrink-0", step.color)} />
              <span className="flex-1 truncate">{step.title}</span>
              {step.id === currentStepId && (
                <span className="ml-auto text-[10px] text-muted-foreground">Current</span>
              )}
            </ContextMenuItem>
          ))}
        </ContextMenuSubContent>
      </ContextMenuSub>
    </>
  );
}

function cloneWithMenuOpen(
  children: React.ReactElement<{ menuOpen?: boolean }>,
  menuOpen: boolean,
): React.ReactNode {
  if (isValidElement(children)) return cloneElement(children, { menuOpen });
  return children;
}

export const TaskSwitcher = memo(function TaskSwitcher({
  grouped,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  collapsedGroupKeys = [],
  onToggleGroup,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  deletingTaskId,
  isLoading = false,
  totalTaskCount,
}: TaskSwitcherProps) {
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
          stepsByWorkflowId={stepsByWorkflowId}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          isCollapsed={collapsedSet.has(group.key)}
          onToggleCollapsed={() => onToggleGroup?.(group.key)}
          showHeader={showHeaders}
          onSelectTask={onSelectTask}
          onRenameTask={onRenameTask}
          onArchiveTask={onArchiveTask}
          onDeleteTask={onDeleteTask}
          onMoveToStep={onMoveToStep}
          deletingTaskId={deletingTaskId}
        />
      ))}
    </div>
  );
});
