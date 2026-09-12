import { TaskPriorityContextMenu } from "./task-priority-context-menu";
import {
  TaskMoveContextMenuItems,
  type TaskMoveStep,
  type TaskMoveWorkflow,
} from "./task-move-context-menu";
import { TaskArchiveItem, TaskDeleteItem } from "./task-switcher-action-items";
import { TaskPluginLinkMenu, type selectTaskLinkActions } from "./task-switcher-link-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";
import type { TaskPriority } from "@/lib/types/http";

export type TaskManagementMenuProps = {
  task: TaskSwitcherItem;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  disabled?: boolean;
  onPriority: (priority: TaskPriority) => void;
  onMove: (workflowId: string, stepId: string) => void;
  onArchive: () => void;
  onDelete: () => void;
  closeMenu: () => void;
  linkActions: Partial<ReturnType<typeof selectTaskLinkActions>>;
};

/** Focused task management composition; the full switcher retains its additional actions. */
export function TaskManagementMenu({
  task,
  workflows,
  stepsByWorkflowId,
  disabled,
  onPriority,
  onMove,
  onArchive,
  onDelete,
  closeMenu,
  linkActions,
}: TaskManagementMenuProps) {
  return (
    <>
      <TaskPriorityContextMenu
        currentPriority={task.priority}
        disabled={disabled}
        onSelect={onPriority}
      />
      <TaskMoveContextMenuItems
        currentWorkflowId={task.workflowId}
        currentStepId={task.workflowStepId}
        workflows={workflows}
        stepsByWorkflowId={stepsByWorkflowId}
        disabled={disabled}
        showSeparator={false}
        onMoveToStep={(stepId) => {
          if (task.workflowId) onMove(task.workflowId, stepId);
        }}
        onSendToWorkflow={onMove}
      />
      <TaskPluginLinkMenu
        task={task}
        disabled={disabled}
        closeMenu={closeMenu}
        linkActions={linkActions}
      />
      <TaskArchiveItem
        taskId={task.id}
        actingIds={[task.id]}
        actingOnSelection={false}
        disabled={disabled}
        onArchiveTask={onArchive}
      />
      <TaskDeleteItem taskId={task.id} isDeleting={disabled} onDeleteTask={onDelete} />
    </>
  );
}
