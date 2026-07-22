"use client";

import { cloneElement, isValidElement, useState } from "react";
import {
  IconBrandGitlab,
  IconBrandSentry,
  IconCopy,
  IconCircleDot,
  IconGitPullRequest,
  IconLink,
  IconPencil,
  IconPin,
  IconPinFilled,
  IconTicket,
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
import { TaskColorMenu } from "./task-switcher-color-menu";
import { TaskArchiveItem, TaskDeleteItem, TaskDetachItem } from "./task-switcher-action-items";
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
  onDetachTask?: (taskId: string) => void;
  onLinkPullRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkMergeRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkJiraTicket?: (taskId: string, taskTitle?: string) => void;
  onLinkLinearIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkSentryIssue?: (taskId: string, taskTitle?: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  isPinned?: boolean;
  pinnedTaskIds?: string[];
  isDeleting?: boolean;
  /** Active multi-selection; when this task is part of it, actions apply to the whole set. */
  selectedTaskIds?: Set<string>;
  onBulkArchive?: (taskIds: string[]) => void;
  onBulkDelete?: (taskIds: string[]) => void;
  onBulkPin?: (taskIds: string[]) => void;
  onBulkMove?: (taskIds: string[], targetWorkflowId: string, targetStepId: string) => void;
  onClearSelection?: () => void;
  /** True when the selection spans more than one workflow (disables bulk "Move to step"). */
  isMixedWorkflowSelection?: boolean;
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
  onDetachTask,
  onLinkPullRequest,
  onLinkIssue,
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  onMoveToStep,
  onTogglePin,
  isPinned,
  pinnedTaskIds,
  isDeleting,
  selectedTaskIds,
  onBulkArchive,
  onBulkDelete,
  onBulkPin,
  onBulkMove,
  onClearSelection,
  isMixedWorkflowSelection,
}: ContextMenuProps) {
  const [contextOpen, setContextOpen] = useState(false);
  const [menuKey, setMenuKey] = useState(0);
  const moveTasks = useTaskWorkflowMove();
  const closeMenu = () => {
    setContextOpen(false);
    setMenuKey((k) => k + 1);
  };

  return (
    <ContextMenu key={menuKey} onOpenChange={setContextOpen}>
      <ContextMenuTrigger asChild>
        <div>{cloneWithMenuOpen(children, contextOpen)}</div>
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
          onDetachTask={onDetachTask}
          onLinkPullRequest={onLinkPullRequest}
          onLinkIssue={onLinkIssue}
          onLinkMergeRequest={onLinkMergeRequest}
          onLinkJiraTicket={onLinkJiraTicket}
          onLinkLinearIssue={onLinkLinearIssue}
          onLinkSentryIssue={onLinkSentryIssue}
          onMoveToStep={onMoveToStep}
          onTogglePin={onTogglePin}
          isPinned={isPinned}
          pinnedTaskIds={pinnedTaskIds}
          isDeleting={isDeleting}
          selectedTaskIds={selectedTaskIds}
          onBulkArchive={onBulkArchive}
          onBulkDelete={onBulkDelete}
          onBulkPin={onBulkPin}
          onBulkMove={onBulkMove}
          onClearSelection={onClearSelection}
          isMixedWorkflowSelection={isMixedWorkflowSelection}
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

function TaskContextMenuItems(props: TaskContextMenuItemsProps) {
  const {
    task,
    workflows,
    stepsByWorkflowId,
    steps,
    onRenameTask,
    onArchiveTask,
    onDeleteTask,
    onDetachTask,
    onMoveToStep,
    onTogglePin,
    isPinned,
    pinnedTaskIds,
    isDeleting,
    selectedTaskIds,
    onBulkArchive,
    onBulkDelete,
    onBulkPin,
    onBulkMove,
    onClearSelection,
    isMixedWorkflowSelection,
    closeMenu,
    moveTasks,
  } = props;
  // Right-clicking any row that's part of the active selection acts on the whole
  // selection (even a one-row selection, so the action clears it); right-clicking
  // a non-selected row acts on just that task and leaves the selection intact.
  const actingOnSelection = !!selectedTaskIds?.has(task.id);
  const actingIds = actingOnSelection ? [...selectedTaskIds!] : [task.id];

  // With several tasks selected, only actions that make sense for all of them
  // are offered (Pin / Move / Archive / Delete) — the single-task actions
  // (Rename, Color, Link, Duplicate) are hidden.
  if (actingOnSelection && actingIds.length > 1) {
    return (
      <BulkSelectionMenuItems
        task={task}
        actingIds={actingIds}
        workflows={workflows}
        stepsByWorkflowId={stepsByWorkflowId}
        steps={steps}
        isMixedWorkflowSelection={isMixedWorkflowSelection}
        pinnedTaskIds={pinnedTaskIds}
        onBulkPin={onBulkPin}
        onBulkArchive={onBulkArchive}
        onBulkDelete={onBulkDelete}
        onBulkMove={onBulkMove}
        closeMenu={closeMenu}
        moveTasks={moveTasks}
      />
    );
  }

  // Acting on a lone selected row (Pin / Delete) must drop it from the selection
  // so later plain clicks navigate instead of toggling.
  const withClear = (handler?: (id: string) => void) =>
    actingOnSelection && onClearSelection && handler
      ? (id: string) => {
          onClearSelection();
          handler(id);
        }
      : handler;
  const onDelete = withClear(onDeleteTask);
  const onDetach = withClear(onDetachTask);
  return (
    <>
      <TaskPinItem
        taskId={task.id}
        isPinned={isPinned}
        disabled={isDeleting}
        onTogglePin={withClear(onTogglePin)}
      />
      <TaskRenameItem task={task} disabled={isDeleting} onRenameTask={onRenameTask} />
      <ContextMenuItem disabled>
        <IconCopy className="mr-2 h-4 w-4" />
        Duplicate
      </ContextMenuItem>
      <TaskArchiveItem
        taskId={task.id}
        actingIds={actingIds}
        actingOnSelection={actingOnSelection}
        disabled={isDeleting}
        onArchiveTask={onArchiveTask}
        onBulkArchive={onBulkArchive}
      />
      <TaskColorMenu taskId={task.id} disabled={isDeleting} />
      <TaskLinkMenu disabled={isDeleting} {...selectTaskLinkActions(task, closeMenu, props)} />
      <TaskMoveItems
        task={task}
        workflows={workflows}
        stepsByWorkflowId={stepsByWorkflowId}
        steps={steps}
        isDeleting={isDeleting}
        onMoveToStep={onMoveToStep}
        actingIds={actingIds}
        actingOnSelection={actingOnSelection}
        onBulkMove={onBulkMove}
        isMixedWorkflowSelection={isMixedWorkflowSelection}
        closeMenu={closeMenu}
        moveTasks={moveTasks}
      />
      <TaskDetachItem task={task} disabled={isDeleting} onDetachTask={onDetach} />
      <TaskDeleteItem taskId={task.id} isDeleting={isDeleting} onDeleteTask={onDelete} />
    </>
  );
}

/** Reduced menu shown when 2+ tasks are selected — only bulk-valid actions. */
function BulkSelectionMenuItems({
  task,
  actingIds,
  workflows,
  stepsByWorkflowId,
  steps,
  isMixedWorkflowSelection,
  pinnedTaskIds,
  onBulkPin,
  onBulkArchive,
  onBulkDelete,
  onBulkMove,
  closeMenu,
  moveTasks,
}: {
  task: TaskSwitcherItem;
  actingIds: string[];
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  steps?: StepDef[];
  isMixedWorkflowSelection?: boolean;
  pinnedTaskIds?: string[];
  onBulkPin?: (taskIds: string[]) => void;
  onBulkArchive?: (taskIds: string[]) => void;
  onBulkDelete?: (taskIds: string[]) => void;
  onBulkMove?: (taskIds: string[], targetWorkflowId: string, targetStepId: string) => void;
  closeMenu: () => void;
  moveTasks: ReturnType<typeof useTaskWorkflowMove>;
}) {
  const n = actingIds.length;
  const allPinned =
    actingIds.length > 0 && actingIds.every((id) => pinnedTaskIds?.includes(id) ?? false);
  const pinLabel = `${allPinned ? "Unpin" : "Pin"} ${n} ${n === 1 ? "task" : "tasks"}`;
  return (
    <>
      {onBulkPin && (
        <ContextMenuItem onSelect={() => onBulkPin(actingIds)}>
          {allPinned ? (
            <IconPinFilled className="mr-2 h-4 w-4" />
          ) : (
            <IconPin className="mr-2 h-4 w-4" />
          )}
          {pinLabel}
        </ContextMenuItem>
      )}
      <TaskArchiveItem
        taskId={task.id}
        actingIds={actingIds}
        actingOnSelection
        onArchiveTask={undefined}
        onBulkArchive={onBulkArchive}
      />
      <TaskMoveItems
        task={task}
        workflows={workflows}
        stepsByWorkflowId={stepsByWorkflowId}
        steps={steps}
        onMoveToStep={undefined}
        actingIds={actingIds}
        actingOnSelection
        onBulkMove={onBulkMove}
        isMixedWorkflowSelection={isMixedWorkflowSelection}
        closeMenu={closeMenu}
        moveTasks={moveTasks}
      />
      {onBulkDelete && (
        <>
          <ContextMenuSeparator />
          <ContextMenuItem variant="destructive" onSelect={() => onBulkDelete(actingIds)}>
            <IconTrash className="mr-2 h-4 w-4" />
            Delete {n} tasks
          </ContextMenuItem>
        </>
      )}
    </>
  );
}

export function createTaskLinkSelectAction(
  task: Pick<TaskSwitcherItem, "id" | "title">,
  handler: ((taskId: string, taskTitle?: string) => void) | undefined,
  closeMenu: () => void,
) {
  if (!handler) return undefined;
  return () => {
    closeMenu();
    handler(task.id, task.title);
  };
}

function selectTaskLinkActions(
  task: Pick<TaskSwitcherItem, "id" | "title">,
  closeMenu: () => void,
  handlers: Pick<
    ContextMenuProps,
    | "onLinkPullRequest"
    | "onLinkIssue"
    | "onLinkMergeRequest"
    | "onLinkJiraTicket"
    | "onLinkLinearIssue"
    | "onLinkSentryIssue"
  >,
) {
  return {
    onLinkPullRequest: createTaskLinkSelectAction(task, handlers.onLinkPullRequest, closeMenu),
    onLinkIssue: createTaskLinkSelectAction(task, handlers.onLinkIssue, closeMenu),
    onLinkMergeRequest: createTaskLinkSelectAction(task, handlers.onLinkMergeRequest, closeMenu),
    onLinkJiraTicket: createTaskLinkSelectAction(task, handlers.onLinkJiraTicket, closeMenu),
    onLinkLinearIssue: createTaskLinkSelectAction(task, handlers.onLinkLinearIssue, closeMenu),
    onLinkSentryIssue: createTaskLinkSelectAction(task, handlers.onLinkSentryIssue, closeMenu),
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

function TaskMoveItems({
  task,
  workflows,
  stepsByWorkflowId,
  steps,
  isDeleting,
  onMoveToStep,
  actingIds,
  actingOnSelection,
  onBulkMove,
  isMixedWorkflowSelection,
  closeMenu,
  moveTasks,
}: Omit<TaskContextMenuItemsProps, "onRenameTask" | "onArchiveTask" | "onDeleteTask"> & {
  actingIds: string[];
  actingOnSelection: boolean;
}) {
  if (!task.workflowId) return null;
  const workflowId = task.workflowId;
  // Moving a selection routes through the sidebar hook's bulkMove, which clears
  // the selection afterwards. Fall back to a raw move when no bulk handler is
  // wired (e.g. the kanban-less callers that don't manage a selection).
  const runSelectionMove = (
    targetWorkflowId: string,
    stepId: string,
    destination: "step" | "workflow",
  ) => {
    closeMenu();
    if (onBulkMove) {
      onBulkMove(actingIds, targetWorkflowId, stepId);
      return;
    }
    void moveTasks(actingIds, targetWorkflowId, stepId, destination).catch(() => {
      // useTaskWorkflowMove already shows the failure toast.
    });
  };

  // Single-task right-click keeps the optimistic same-workflow move. A selection
  // spanning workflows makes "Move to step" of one workflow ambiguous, so disable
  // it there (Send to workflow remains the explicit path).
  let moveToStep: ((stepId: string) => void) | undefined;
  if (actingOnSelection) {
    moveToStep = isMixedWorkflowSelection
      ? undefined
      : (stepId) => runSelectionMove(workflowId, stepId, "step");
  } else {
    moveToStep = (stepId) => {
      closeMenu();
      if (onMoveToStep) {
        onMoveToStep(task.id, workflowId, stepId);
        return;
      }
      void moveTasks([task.id], workflowId, stepId, "step").catch(() => {
        // useTaskWorkflowMove already shows the failure toast.
      });
    };
  }

  return (
    <TaskMoveContextMenuItems
      currentWorkflowId={workflowId}
      // For a selection spanning several steps, don't disable the clicked row's
      // step — the backend bulk move skips tasks already there, and the other
      // selected rows still need it as a target.
      currentStepId={actingOnSelection ? undefined : task.workflowStepId}
      workflows={workflows ?? []}
      stepsByWorkflowId={stepsByWorkflowId ?? (steps ? { [workflowId]: steps } : {})}
      disabled={isDeleting || task.isArchived}
      onMoveToStep={moveToStep}
      onSendToWorkflow={(targetWorkflowId, stepId) => {
        if (actingOnSelection) {
          runSelectionMove(targetWorkflowId, stepId, "workflow");
          return;
        }
        closeMenu();
        void moveTasks([task.id], targetWorkflowId, stepId, "workflow").catch(() => {
          // useTaskWorkflowMove already shows the failure toast.
        });
      }}
    />
  );
}

function TaskLinkMenu({
  disabled,
  onLinkPullRequest,
  onLinkIssue,
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
}: {
  disabled?: boolean;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
  onLinkMergeRequest?: () => void;
  onLinkJiraTicket?: () => void;
  onLinkLinearIssue?: () => void;
  onLinkSentryIssue?: () => void;
}) {
  if (
    !onLinkPullRequest &&
    !onLinkIssue &&
    !onLinkMergeRequest &&
    !onLinkJiraTicket &&
    !onLinkLinearIssue &&
    !onLinkSentryIssue
  ) {
    return null;
  }
  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger disabled={disabled}>
        <IconLink className="mr-2 h-4 w-4" />
        Link
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-56">
        {onLinkPullRequest && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkPullRequest}>
            <IconGitPullRequest className="mr-2 h-4 w-4" />
            GitHub Pull Request
          </ContextMenuItem>
        )}
        {onLinkIssue && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkIssue}>
            <IconCircleDot className="mr-2 h-4 w-4" />
            GitHub Issue
          </ContextMenuItem>
        )}
        {onLinkMergeRequest && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkMergeRequest}>
            <IconBrandGitlab className="mr-2 h-4 w-4" />
            GitLab Merge Request
          </ContextMenuItem>
        )}
        {onLinkJiraTicket && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkJiraTicket}>
            <IconTicket className="mr-2 h-4 w-4" />
            Jira Ticket
          </ContextMenuItem>
        )}
        {onLinkLinearIssue && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkLinearIssue}>
            <IconCircleDot className="mr-2 h-4 w-4" />
            Linear Issue
          </ContextMenuItem>
        )}
        {onLinkSentryIssue && (
          <ContextMenuItem disabled={disabled} onSelect={onLinkSentryIssue}>
            <IconBrandSentry className="mr-2 h-4 w-4" />
            Sentry Issue
          </ContextMenuItem>
        )}
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}
