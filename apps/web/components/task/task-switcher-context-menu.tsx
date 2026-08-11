"use client";

import { cloneElement, isValidElement, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconCopy,
  IconEdit,
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
import { useAppStore } from "@/components/state-provider";
import {
  TaskMoveContextMenuItems,
  type TaskMoveWorkflow,
} from "@/components/task/task-move-context-menu";
import { TaskNestContextMenuItems } from "@/components/task/task-nest-context-menu";
import { KanbanCardContextMenuItems } from "@/components/kanban-card-menu-items";
import { buildPrimaryPluginEntries } from "@/components/plugins/task-menu-actions";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { TaskColorMenu } from "./task-switcher-color-menu";
import { TaskLinkMenu } from "./task-switcher-link-menu";
import {
  TaskArchiveItem,
  TaskCreateSubtaskItem,
  TaskDeleteItem,
  TaskDetachItem,
} from "./task-switcher-action-items";
import type { StepDef, TaskSwitcherItem } from "./task-switcher-types";
export type { StepDef } from "./task-switcher-types";

type ContextMenuProps = {
  task: TaskSwitcherItem;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  steps?: StepDef[];
  children: React.ReactElement<{ menuOpen?: boolean }>;
  onEditTask?: (task: TaskSwitcherItem) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
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
  onEditTask,
  onRenameTask,
  onArchiveTask,
  onCreateSubtask,
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
          onEditTask={onEditTask}
          onRenameTask={onRenameTask}
          onArchiveTask={onArchiveTask}
          onCreateSubtask={onCreateSubtask}
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

// With several tasks selected, only actions that make sense for all of them
// are offered (Pin / Move / Archive / Delete) — the single-task actions
// (Rename, Color, Link, Duplicate) are hidden.
function renderBulkSelectionMenu(
  props: TaskContextMenuItemsProps,
  actingOnSelection: boolean,
  actingIds: string[],
  moveTasks: ReturnType<typeof useTaskWorkflowMove>,
): React.ReactNode | null {
  if (!actingOnSelection || actingIds.length <= 1) return null;
  const {
    task,
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
  } = props;
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

function TaskContextMenuItems(props: TaskContextMenuItemsProps) {
  const { t } = useTranslation();
  const {
    task,
    workflows,
    stepsByWorkflowId,
    steps,
    onEditTask,
    onRenameTask,
    onArchiveTask,
    onCreateSubtask,
    onDeleteTask,
    onDetachTask,
    onMoveToStep,
    onTogglePin,
    isPinned,
    isDeleting,
    selectedTaskIds,
    onBulkArchive,
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

  const bulkMenu = renderBulkSelectionMenu(props, actingOnSelection, actingIds, moveTasks);
  if (bulkMenu) return bulkMenu;

  // Acting on a lone selected row (Pin / Delete) must drop it from the selection
  // so later plain clicks navigate instead of toggling.
  const onDelete = withSelectionClear(actingOnSelection, onClearSelection, onDeleteTask);
  const onDetach = withSelectionClear(actingOnSelection, onClearSelection, onDetachTask);
  return (
    <>
      <TaskPinItem
        taskId={task.id}
        isPinned={isPinned}
        disabled={isDeleting}
        onTogglePin={withSelectionClear(actingOnSelection, onClearSelection, onTogglePin)}
      />
      <TaskEditItem task={task} disabled={isDeleting} onEditTask={onEditTask} />
      <TaskRenameItem task={task} disabled={isDeleting} onRenameTask={onRenameTask} />
      <TaskCreateSubtaskItem task={task} disabled={isDeleting} onCreateSubtask={onCreateSubtask} />
      {!task.isArchived && (
        <ContextMenuItem disabled>
          <IconCopy className="mr-2 h-4 w-4" />
          {t("settings:duplicate")}
        </ContextMenuItem>
      )}
      <TaskArchiveItem
        taskId={task.id}
        actingIds={actingIds}
        actingOnSelection={actingOnSelection}
        disabled={isDeleting}
        onArchiveTask={onArchiveTask}
        onBulkArchive={onBulkArchive}
      />
      {!task.isArchived && <TaskColorMenu taskId={task.id} disabled={isDeleting} />}
      <TaskNestContextMenuItems task={task} disabled={isDeleting} />
      <TaskPluginPrimaryMenuItems task={task} disabled={isDeleting} />
      <TaskLinkMenu disabled={isDeleting} {...selectTaskLinkActions(task, closeMenu, props)} />
      {!task.isArchived && (
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
      )}
      <TaskDetachItem task={task} disabled={isDeleting} onDetachTask={onDetach} />
      <TaskDeleteItem taskId={task.id} isDeleting={isDeleting} onDeleteTask={onDelete} />
    </>
  );
}

/**
 * Group "primary" plugin task-menu actions (e.g. the tags plugin's "Add
 * tag..." item), rendered as flat items immediately before the sidebar
 * menu's "Link" submenu — the same entries the kanban card menu already
 * renders via `buildPrimaryPluginEntries`, so a plugin registers once and
 * appears in both surfaces. `PluginTaskMenuContext.workspaceId` is typed
 * non-nullable, so with no active workspace this renders nothing rather than
 * passing an empty-string placeholder.
 *
 * `presentation` is derived from the viewport rather than passed in, because
 * this one row component backs both surfaces that render it: the desktop
 * sidebar and the phone session task-switcher sheet, which swap at the very
 * `useResponsiveBreakpoint` mobile boundary (768px) the sidebar itself uses.
 * The kanban card threads the value as a prop instead only because its mobile
 * board is a separate component tree.
 */
function TaskPluginPrimaryMenuItems({
  task,
  disabled,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
}) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { isMobile } = useResponsiveBreakpoint();
  if (!workspaceId) return null;

  const context: PluginTaskMenuContext = {
    workspaceId,
    taskId: task.id,
    taskTitle: task.title,
    workflowStepId: task.workflowStepId ?? null,
    presentation: isMobile ? "mobile" : "desktop",
  };
  return <KanbanCardContextMenuItems entries={buildPrimaryPluginEntries({ disabled, context })} />;
}

function withSelectionClear(
  actingOnSelection: boolean,
  onClearSelection: (() => void) | undefined,
  handler: ((id: string) => void) | undefined,
) {
  if (!actingOnSelection || !onClearSelection || !handler) return handler;
  return (id: string) => {
    onClearSelection();
    handler(id);
  };
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
  const { t } = useTranslation();
  const n = actingIds.length;
  const allPinned =
    actingIds.length > 0 && actingIds.every((id) => pinnedTaskIds?.includes(id) ?? false);
  const pinLabel = allPinned
    ? t("task:unpinTasksCount", { count: n })
    : t("task:pinTasksCount", { count: n });
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
            {t("task:deleteTasksCount", { count: n })}
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
  const { t } = useTranslation();
  if (!onTogglePin) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onTogglePin(taskId)}>
      {isPinned ? <IconPinFilled className="mr-2 h-4 w-4" /> : <IconPin className="mr-2 h-4 w-4" />}
      {isPinned ? t("task:unpin") : t("task:annotationPin")}
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
  const { t } = useTranslation();
  if (!onRenameTask) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onRenameTask(task.id, task.title)}>
      <IconPencil className="mr-2 h-4 w-4" />
      {t("task:rename")}
    </ContextMenuItem>
  );
}

function TaskEditItem({
  task,
  disabled,
  onEditTask,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
  onEditTask?: (task: TaskSwitcherItem) => void;
}) {
  const { t } = useTranslation();
  if (!onEditTask || task.isArchived || !task.workflowId || !task.workflowStepId) return null;
  return (
    <ContextMenuItem disabled={disabled} onSelect={() => onEditTask(task)}>
      <IconEdit className="mr-2 h-4 w-4" />
      {t("common:edit")}
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
