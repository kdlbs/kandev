"use client";

import { cloneElement, isValidElement, useState } from "react";
import {
  IconArchive,
  IconCopy,
  IconLoader,
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
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import {
  TaskMoveContextMenuItems,
  type TaskMoveWorkflow,
} from "@/components/task/task-move-context-menu";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
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
