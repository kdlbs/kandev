"use client";

import { cloneElement, isValidElement, useState } from "react";
import {
  IconArchive,
  IconCheck,
  IconCopy,
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
        {onTogglePin && (
          <ContextMenuItem disabled={isDeleting} onSelect={() => onTogglePin(task.id)}>
            {isPinned ? (
              <IconPinFilled className="mr-2 h-4 w-4" />
            ) : (
              <IconPin className="mr-2 h-4 w-4" />
            )}
            {isPinned ? "Unpin" : "Pin"}
          </ContextMenuItem>
        )}
        {onRenameTask && (
          <ContextMenuItem disabled={isDeleting} onSelect={() => onRenameTask(task.id, task.title)}>
            <IconPencil className="mr-2 h-4 w-4" />
            Rename
          </ContextMenuItem>
        )}
        <ContextMenuItem disabled>
          <IconCopy className="mr-2 h-4 w-4" />
          Duplicate
        </ContextMenuItem>
        {onArchiveTask && (
          <ContextMenuItem disabled={isDeleting} onSelect={() => onArchiveTask(task.id)}>
            <IconArchive className="mr-2 h-4 w-4" />
            Archive
          </ContextMenuItem>
        )}
        <TaskColorMenu taskId={task.id} disabled={isDeleting} />
        {task.workflowId && (
          <TaskMoveContextMenuItems
            currentWorkflowId={task.workflowId}
            currentStepId={task.workflowStepId}
            workflows={workflows ?? []}
            stepsByWorkflowId={stepsByWorkflowId ?? (steps ? { [task.workflowId]: steps } : {})}
            disabled={isDeleting || task.isArchived}
            onMoveToStep={
              onMoveToStep
                ? (stepId) => {
                    closeMenu();
                    onMoveToStep(task.id, task.workflowId!, stepId);
                  }
                : undefined
            }
            onSendToWorkflow={(workflowId, stepId) => {
              closeMenu();
              void moveTasks([task.id], workflowId, stepId).catch(() => {
                // useTaskWorkflowMove already shows the failure toast.
              });
            }}
          />
        )}
        {onDeleteTask && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem
              variant="destructive"
              disabled={isDeleting}
              onSelect={() => onDeleteTask(task.id)}
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

function cloneWithMenuOpen(
  children: React.ReactElement<{ menuOpen?: boolean }>,
  menuOpen: boolean,
): React.ReactNode {
  if (isValidElement(children)) return cloneElement(children, { menuOpen });
  return children;
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
