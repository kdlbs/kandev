"use client";

import { cloneElement, isValidElement, useState } from "react";
import {
  IconArchive,
  IconCheck,
  IconCopy,
  IconCircleDot,
  IconGitPullRequest,
  IconLink,
  IconLoader,
  IconPalette,
  IconPencil,
  IconPin,
  IconPinFilled,
  IconTrash,
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
import {
  TaskMoveContextMenuItems,
  type TaskMoveWorkflow,
} from "@/components/task/task-move-context-menu";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useSetTaskColor, useTaskColor } from "@/hooks/use-task-color";
import {
  TASK_COLORS,
  TASK_COLOR_BAR_CLASS,
  TASK_COLOR_LABEL,
  type TaskColor,
} from "@/lib/task-colors";
import { cn } from "@/lib/utils";
import type { TaskSwitcherItem } from "./task-switcher";

export type StepDef = {
  id: string;
  title: string;
  color?: string;
  events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }> };
};

type ContextMenuProps = {
  task: TaskSwitcherItem;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  steps?: StepDef[];
  children: React.ReactElement<{ menuOpen?: boolean }>;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onLinkPullRequest?: (taskId: string) => void;
  onLinkIssue?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  isPinned?: boolean;
  isDeleting?: boolean;
};

export function TaskItemWithContextMenu({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  children,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onLinkPullRequest,
  onLinkIssue,
  onMoveToStep,
  onTogglePin,
  isPinned,
  isDeleting,
}: ContextMenuProps) {
  const [contextOpen, setContextOpen] = useState(false);
  const [menuKey, setMenuKey] = useState(0);
  const moveTasks = useTaskWorkflowMove();
  const menuOpen = contextOpen || isDeleting === true;
  const closeMenu = () => {
    setContextOpen(false);
    setMenuKey((k) => k + 1);
  };

  return (
    <ContextMenu key={menuKey} onOpenChange={setContextOpen}>
      <ContextMenuTrigger asChild>
        <div>{cloneWithMenuOpen(children, menuOpen)}</div>
      </ContextMenuTrigger>
      <ContextMenuContent className="w-48">
        <TaskContextMenuItems
          task={task}
          workflows={workflows}
          stepsByWorkflowId={stepsByWorkflowId}
          steps={steps}
          onRenameTask={onRenameTask}
          onArchiveTask={onArchiveTask}
          onDeleteTask={onDeleteTask}
          onLinkPullRequest={onLinkPullRequest}
          onLinkIssue={onLinkIssue}
          onMoveToStep={onMoveToStep}
          onTogglePin={onTogglePin}
          isPinned={isPinned}
          isDeleting={isDeleting}
          closeMenu={closeMenu}
          moveTasks={moveTasks}
        />
      </ContextMenuContent>
    </ContextMenu>
  );
}

type TaskContextMenuItemsProps = Omit<ContextMenuProps, "children"> & {
  closeMenu: () => void;
  moveTasks: ReturnType<typeof useTaskWorkflowMove>;
};

function TaskContextMenuItems({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onLinkPullRequest,
  onLinkIssue,
  onMoveToStep,
  onTogglePin,
  isPinned,
  isDeleting,
  closeMenu,
  moveTasks,
}: TaskContextMenuItemsProps) {
  return (
    <>
      <TaskPinItem
        taskId={task.id}
        isPinned={isPinned}
        disabled={isDeleting}
        onTogglePin={onTogglePin}
      />
      <TaskRenameItem task={task} disabled={isDeleting} onRenameTask={onRenameTask} />
      <ContextMenuItem disabled>
        <IconCopy className="mr-2 h-4 w-4" />
        Duplicate
      </ContextMenuItem>
      <TaskArchiveItem taskId={task.id} disabled={isDeleting} onArchiveTask={onArchiveTask} />
      <TaskColorMenu taskId={task.id} disabled={isDeleting} />
      <TaskLinkMenu
        disabled={isDeleting}
        onLinkPullRequest={selectTaskAction(task.id, onLinkPullRequest, closeMenu)}
        onLinkIssue={selectTaskAction(task.id, onLinkIssue, closeMenu)}
      />
      <TaskMoveItems
        task={task}
        workflows={workflows}
        stepsByWorkflowId={stepsByWorkflowId}
        steps={steps}
        isDeleting={isDeleting}
        onMoveToStep={onMoveToStep}
        closeMenu={closeMenu}
        moveTasks={moveTasks}
      />
      <TaskDeleteItem taskId={task.id} isDeleting={isDeleting} onDeleteTask={onDeleteTask} />
    </>
  );
}

function selectTaskAction(
  taskId: string,
  handler: ((taskId: string) => void) | undefined,
  closeMenu: () => void,
) {
  if (!handler) return undefined;
  return () => {
    closeMenu();
    handler(taskId);
  };
}

function cloneWithMenuOpen(
  children: React.ReactElement<{ menuOpen?: boolean }>,
  menuOpen: boolean,
): React.ReactNode {
  if (isValidElement(children)) return cloneElement(children, { menuOpen });
  return children;
}

function TaskPinItem({
  taskId,
  isPinned,
  disabled,
  onTogglePin,
}: {
  taskId: string;
  isPinned?: boolean;
  disabled?: boolean;
  onTogglePin?: (taskId: string) => void;
}) {
  if (!onTogglePin) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onTogglePin(taskId)}>
      {isPinned ? <IconPinFilled className="mr-2 h-4 w-4" /> : <IconPin className="mr-2 h-4 w-4" />}
      {isPinned ? "Unpin" : "Pin"}
    </ContextMenuItem>
  );
}

function TaskRenameItem({
  task,
  disabled,
  onRenameTask,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
}) {
  if (!onRenameTask) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onRenameTask(task.id, task.title)}>
      <IconPencil className="mr-2 h-4 w-4" />
      Rename
    </ContextMenuItem>
  );
}

function TaskArchiveItem({
  taskId,
  disabled,
  onArchiveTask,
}: {
  taskId: string;
  disabled?: boolean;
  onArchiveTask?: (taskId: string) => void;
}) {
  if (!onArchiveTask) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onArchiveTask(taskId)}>
      <IconArchive className="mr-2 h-4 w-4" />
      Archive
    </ContextMenuItem>
  );
}

function TaskMoveItems({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  isDeleting,
  onMoveToStep,
  closeMenu,
  moveTasks,
}: Omit<TaskContextMenuItemsProps, "onRenameTask" | "onArchiveTask" | "onDeleteTask">) {
  if (!task.workflowId) return null;
  return (
    <TaskMoveContextMenuItems
      currentWorkflowId={task.workflowId}
      currentStepId={task.workflowStepId}
      workflows={workflows ?? []}
      stepsByWorkflowId={stepsByWorkflowId ?? (steps ? { [task.workflowId]: steps } : {})}
      disabled={isDeleting || task.isArchived}
      onMoveToStep={selectMoveAction(task.id, task.workflowId, onMoveToStep, closeMenu)}
      onSendToWorkflow={(workflowId, stepId) => {
        closeMenu();
        void moveTasks([task.id], workflowId, stepId).catch(() => {
          // useTaskWorkflowMove already shows the failure toast.
        });
      }}
    />
  );
}

function selectMoveAction(
  taskId: string,
  workflowId: string,
  handler: ((taskId: string, workflowId: string, targetStepId: string) => void) | undefined,
  closeMenu: () => void,
) {
  if (!handler) return undefined;
  return (stepId: string) => {
    closeMenu();
    handler(taskId, workflowId, stepId);
  };
}

function TaskDeleteItem({
  taskId,
  isDeleting,
  onDeleteTask,
}: {
  taskId: string;
  isDeleting?: boolean;
  onDeleteTask?: (taskId: string) => void;
}) {
  if (!onDeleteTask) return null;
  return (
    <>
      <ContextMenuSeparator />
      <ContextMenuItem
        variant="destructive"
        disabled={isDeleting}
        onSelect={() => onDeleteTask(taskId)}
      >
        {isDeleting ? (
          <IconLoader className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <IconTrash className="mr-2 h-4 w-4" />
        )}
        Delete
      </ContextMenuItem>
    </>
  );
}

function TaskLinkMenu({
  disabled,
  onLinkPullRequest,
  onLinkIssue,
}: {
  disabled?: boolean;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
}) {
  if (!onLinkPullRequest && !onLinkIssue) return null;
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled}>
        <IconLink className="mr-2 h-4 w-4" />
        Link
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-56">
        <ContextMenuItem disabled={disabled || !onLinkPullRequest} onSelect={onLinkPullRequest}>
          <IconGitPullRequest className="mr-2 h-4 w-4" />
          GitHub Pull Request
        </ContextMenuItem>
        <ContextMenuItem disabled={disabled || !onLinkIssue} onSelect={onLinkIssue}>
          <IconCircleDot className="mr-2 h-4 w-4" />
          GitHub Issue
        </ContextMenuItem>
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function TaskColorMenu({ taskId, disabled }: { taskId: string; disabled?: boolean }) {
  const currentColor = useTaskColor(taskId);
  const setColor = useSetTaskColor();
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled}>
        <IconPalette className="mr-2 h-4 w-4" />
        Color
        {currentColor && (
          <span
            className={cn(
              "ml-2 inline-block h-2 w-2 rounded-full",
              TASK_COLOR_BAR_CLASS[currentColor],
            )}
          />
        )}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-40">
        {TASK_COLORS.map((color) => (
          <TaskColorMenuItem
            key={color}
            color={color}
            selected={currentColor === color}
            onSelect={() => setColor(taskId, color)}
          />
        ))}
        <ContextMenuSeparator />
        <ContextMenuItem disabled={!currentColor} onSelect={() => setColor(taskId, null)}>
          <span className="mr-2 inline-block h-2 w-2 rounded-full border border-muted-foreground/40" />
          None
        </ContextMenuItem>
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

function TaskColorMenuItem({
  color,
  selected,
  onSelect,
}: {
  color: TaskColor;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <ContextMenuItem onSelect={onSelect}>
      <span className={cn("mr-2 inline-block h-2 w-2 rounded-full", TASK_COLOR_BAR_CLASS[color])} />
      {TASK_COLOR_LABEL[color]}
      {selected && <IconCheck className="ml-auto h-3.5 w-3.5" />}
    </ContextMenuItem>
  );
}
