"use client";

import { useRef, useState } from "react";
import { useDraggable } from "@dnd-kit/core";
import { KanbanCardContextMenu } from "@/components/kanban-card-context-menu";
import { KanbanCardShell } from "@/components/kanban-card-content";
import { KanbanCardDialogs } from "@/components/kanban-card-dialogs";
import { useActiveWorkspaceRepositories } from "@/components/kanban-card-repositories";
export { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import {
  buildKanbanCardMenuEntries,
  useKanbanCardMoveTargets,
} from "@/components/kanban-card-menu-items";
import { useTaskPluginLinkActions } from "@/components/task/task-session-sidebar-link-actions";
import { useAppStore } from "@/components/state-provider";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { TaskDetachConfirmationSurface } from "@/components/task/task-detach-confirm-dialog";
import { type ExternalLinkProvider } from "@/components/task/task-external-link-dialog";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useTaskMultiSelectStore } from "@/hooks/use-task-multi-select";
import type { TaskActionOptions } from "@/hooks/use-task-actions";
import { useDetachTask } from "@/hooks/use-detach-task";
import { useUpdateTaskPriority } from "@/hooks/use-update-task-priority";
import { type TaskPriority } from "@/lib/types/http";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { Task, RepositoryChip, WorkflowStep, KanbanPresentation } from "./kanban-card-types";

export type { Task, RepositoryChip, WorkflowStep, KanbanPresentation };

interface KanbanCardProps {
  task: Task;
  workspaceId: string | null;
  presentation?: KanbanPresentation;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  /** Display labels and hover paths of every repository linked to the task, primary first. */
  repositoryChips?: RepositoryChip[];
  onClick?: (task: Task) => void;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task, opts?: TaskActionOptions) => void;
  onArchive?: (task: Task, opts?: TaskActionOptions) => void;
  onOpenFullPage?: (task: Task) => void;
  onMove?: (task: Task, targetStepId: string) => void;
  steps?: WorkflowStep[];
  showMaximizeButton?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isSelected?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  /** Shift-click range select within this card's column. */
  onRangeSelect?: (taskId: string) => void;
  isMultiSelectMode?: boolean;
}

function useKanbanCardMoveMenuActions({
  task,
  steps,
  isSelected,
  selectedIds,
  onMove,
}: Pick<KanbanCardProps, "task" | "steps" | "isSelected" | "selectedIds" | "onMove">) {
  const moveTargets = useKanbanCardMoveTargets(task.id, steps);
  const moveTasks = useTaskWorkflowMove();
  const { sortByDisplayOrder, getWorkflowIdForTask } = useTaskMultiSelectStore();

  const runMoveTasks = (
    taskIds: string[],
    workflowId: string,
    stepId: string,
    destination: "step" | "workflow",
  ) => {
    void moveTasks(taskIds, workflowId, stepId, destination).catch(() => {
      // useTaskWorkflowMove already shows the failure toast.
    });
  };
  const moveToStepFromDropdown = (stepId: string) => {
    if (onMove) return onMove(task, stepId);
    if (moveTargets.currentWorkflowId) {
      runMoveTasks([task.id], moveTargets.currentWorkflowId, stepId, "step");
    }
  };
  const selectedTaskIds = isSelected && selectedIds?.size ? [...selectedIds] : [task.id];
  const orderedSelectedIds = () => sortByDisplayOrder(selectedTaskIds);
  const isMixedWorkflowSelection =
    selectedTaskIds.length > 1 &&
    new Set(selectedTaskIds.map((id) => getWorkflowIdForTask(id))).size > 1;
  const moveSelectedToStep = (stepId: string) => {
    if (selectedTaskIds.length === 1 && selectedTaskIds[0] === task.id && onMove) {
      onMove(task, stepId);
      return;
    }
    if (!moveTargets.currentWorkflowId) return;
    runMoveTasks(orderedSelectedIds(), moveTargets.currentWorkflowId, stepId, "step");
  };

  return {
    moveTargets,
    moveToStepFromDropdown,
    moveSelectedToStep: isMixedWorkflowSelection ? undefined : moveSelectedToStep,
    sendTaskToWorkflow: (workflowId: string, stepId: string) => {
      runMoveTasks([task.id], workflowId, stepId, "workflow");
    },
    sendSelectionToWorkflow: (workflowId: string, stepId: string) => {
      runMoveTasks(orderedSelectedIds(), workflowId, stepId, "workflow");
    },
  };
}

function externalLinkHandlers(
  availability: KanbanCardProps["externalLinkAvailability"],
  setExternalLinkProvider: (provider: ExternalLinkProvider) => void,
) {
  return {
    onLinkJiraTicket: availability.jira ? () => setExternalLinkProvider("jira") : undefined,
    onLinkLinearIssue: availability.linear ? () => setExternalLinkProvider("linear") : undefined,
    onLinkSentryIssue: availability.sentry ? () => setExternalLinkProvider("sentry") : undefined,
  };
}

/** Link-dialog openers shared by both the dropdown and context menu builds. */
function buildLinkDialogHandlers(
  externalLinkAvailability: KanbanExternalLinkAvailability,
  dialogs: ReturnType<typeof useKanbanCardDialogState>,
) {
  return {
    onLinkPullRequest: () => dialogs.setShowPRDialog(true),
    onLinkIssue: () => dialogs.setShowIssueDialog(true),
    onLinkMergeRequest: externalLinkAvailability.gitlab
      ? () => dialogs.setShowMRDialog(true)
      : undefined,
    ...externalLinkHandlers(externalLinkAvailability, dialogs.setExternalLinkProvider),
  };
}

export function buildPluginMenuContext(
  task: Task,
  workspaceId: string | null,
  presentation: KanbanPresentation,
): PluginTaskMenuContext {
  return {
    workspaceId: workspaceId ?? "",
    taskId: task.id,
    taskTitle: task.title,
    workflowStepId: task.workflowStepId ?? null,
    presentation,
  };
}

/** Every confirm/link-dialog open flag the card menus and their dialogs share. */
function useKanbanCardDialogState() {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const [showDetachConfirm, setShowDetachConfirm] = useState(false);
  const [showPRDialog, setShowPRDialog] = useState(false);
  const [showIssueDialog, setShowIssueDialog] = useState(false);
  const [showMRDialog, setShowMRDialog] = useState(false);
  const [externalLinkProvider, setExternalLinkProvider] = useState<ExternalLinkProvider | null>(
    null,
  );
  return {
    showDeleteConfirm,
    setShowDeleteConfirm,
    showArchiveConfirm,
    setShowArchiveConfirm,
    showDetachConfirm,
    setShowDetachConfirm,
    showPRDialog,
    setShowPRDialog,
    showIssueDialog,
    setShowIssueDialog,
    showMRDialog,
    setShowMRDialog,
    externalLinkProvider,
    setExternalLinkProvider,
  };
}

function useKanbanCardMenus({
  task,
  workspaceId,
  presentation = "desktop",
  steps,
  isDeleting,
  isArchiving,
  isSelected,
  selectedIds,
  onEdit,
  onDelete,
  onArchive,
  onMove,
  externalLinkAvailability,
}: Pick<
  KanbanCardProps,
  | "task"
  | "workspaceId"
  | "presentation"
  | "externalLinkAvailability"
  | "steps"
  | "isDeleting"
  | "isArchiving"
  | "isSelected"
  | "selectedIds"
  | "onEdit"
  | "onDelete"
  | "onArchive"
  | "onMove"
>) {
  const pluginLinkActions = useTaskPluginLinkActions(task.id, task.repositories ?? []);
  // Plugins load asynchronously and can be disabled/uninstalled at runtime;
  // re-render on any registry change so a menu action a plugin registers
  // after this card already mounted still appears, and one whose plugin was
  // just disabled doesn't linger as a stale entry.
  usePluginRegistry();
  const moveMenu = useKanbanCardMoveMenuActions({ task, steps, isSelected, selectedIds, onMove });
  const dialogs = useKanbanCardDialogState();
  const { detachTask, detachingTaskId } = useDetachTask();
  const updateTaskPriority = useUpdateTaskPriority();
  const detachAnchorRef = useRef<HTMLDivElement>(null);
  const detachFocusReturnRef = useRef<HTMLButtonElement>(null);
  const isDetaching = detachingTaskId === task.id;
  const disabled = Boolean(isDeleting || isArchiving || isDetaching);
  const actingOnMultiSelection = Boolean(isSelected && selectedIds && selectedIds.size > 1);

  const handleDetachConfirm = async () => {
    try {
      await detachTask(task.id);
      dialogs.setShowDetachConfirm(false);
    } catch (error) {
      console.error("Failed to detach task:", error);
    }
  };

  const requestDetachConfirmation = () => {
    // Let Radix finish the menu's pointer sequence before the non-modal
    // popover opens; otherwise the initiating menu event is an outside click.
    window.setTimeout(() => dialogs.setShowDetachConfirm(true), 300);
  };

  const requestArchiveConfirmation = () => {
    // Let Radix finish the menu's pointer sequence before the local surface
    // opens; otherwise the initiating menu event is treated as outside input.
    window.setTimeout(() => dialogs.setShowArchiveConfirm(true), 300);
  };

  const menuBase = {
    currentWorkflowId: moveMenu.moveTargets.currentWorkflowId,
    currentStepId: task.workflowStepId,
    workflows: moveMenu.moveTargets.workflowItems,
    stepsByWorkflowId: moveMenu.moveTargets.stepsByWorkflowId,
    disabled,
    isDeleting,
    isArchiving,
    isDetaching,
    parentTaskId: task.parentTaskId,
    currentPriority: task.priority,
    onSelectPriority: (priority: TaskPriority) => void updateTaskPriority(task.id, priority),
    onEdit: onEdit ? () => onEdit(task) : undefined,
    onArchive: onArchive ? requestArchiveConfirmation : undefined,
    onDelete: onDelete ? () => dialogs.setShowDeleteConfirm(true) : undefined,
    onDetach: task.parentTaskId && !actingOnMultiSelection ? requestDetachConfirmation : undefined,
    ...buildLinkDialogHandlers(externalLinkAvailability, dialogs),
    pluginLinkActions,
  };

  const pluginMenuContext = buildPluginMenuContext(task, workspaceId, presentation);

  return {
    ...dialogs,
    dropdownMenuEntries: buildKanbanCardMenuEntries({
      ...menuBase,
      onMoveToStep: moveMenu.moveToStepFromDropdown,
      onSendToWorkflow: moveMenu.sendTaskToWorkflow,
      pluginMenuContext,
    }),
    contextMenuEntries: buildKanbanCardMenuEntries({
      ...menuBase,
      onMoveToStep: moveMenu.moveSelectedToStep,
      onSendToWorkflow: moveMenu.sendSelectionToWorkflow,
      pluginMenuContext,
    }),
    isDetaching,
    detachAnchorRef,
    detachFocusReturnRef,
    archiveAnchorRef: detachFocusReturnRef,
    archiveFocusReturnRef: detachFocusReturnRef,
    handleDetachConfirm,
  };
}

export type KanbanCardMenuState = ReturnType<typeof useKanbanCardMenus>;

/**
 * Cmd/Ctrl-click toggles a single card; Shift-click range-selects within the
 * column; either modifier enters multi-select mode without the toggle button.
 * A plain click toggles while in multi-select mode, otherwise previews/opens.
 */
/** @internal Exported for unit testing the four-branch click dispatch. */
export function dispatchKanbanCardClick(
  e: React.MouseEvent,
  taskId: string,
  task: Task,
  handlers: {
    onToggleSelect?: (taskId: string) => void;
    onRangeSelect?: (taskId: string) => void;
    onClick?: (task: Task) => void;
    isMultiSelectMode?: boolean;
  },
): void {
  // Only intercept a modifier click when the matching handler is wired, so a
  // card rendered without selection handlers still opens on Cmd/Shift click.
  if ((e.metaKey || e.ctrlKey) && handlers.onToggleSelect) {
    e.preventDefault();
    handlers.onToggleSelect(taskId);
    return;
  }
  if (e.shiftKey && handlers.onRangeSelect) {
    e.preventDefault();
    handlers.onRangeSelect(taskId);
    return;
  }
  if (handlers.isMultiSelectMode && handlers.onToggleSelect) {
    handlers.onToggleSelect(taskId);
    return;
  }
  handlers.onClick?.(task);
}

function KanbanCardFrame({
  task,
  presentation,
  repositoryChips,
  draggable,
  menu,
  isPreviewed,
  isSelected,
  isMultiSelectMode,
  showMaximizeButton,
  isDeleting,
  isArchiving,
  onArchive,
  onClick,
  onToggleSelect,
  onOpenFullPage,
}: Pick<
  KanbanCardProps,
  | "task"
  | "presentation"
  | "repositoryChips"
  | "isSelected"
  | "isMultiSelectMode"
  | "showMaximizeButton"
  | "isDeleting"
  | "isArchiving"
  | "onArchive"
  | "onToggleSelect"
  | "onOpenFullPage"
> & {
  draggable: ReturnType<typeof useDraggable>;
  menu: KanbanCardMenuState;
  isPreviewed: boolean;
  onClick: (e: React.MouseEvent) => void;
}) {
  return (
    <>
      <div ref={menu.detachAnchorRef} className="w-full">
        <KanbanCardContextMenu entries={menu.contextMenuEntries}>
          <KanbanCardShell
            task={task}
            repositoryChips={repositoryChips}
            attributes={draggable.attributes}
            listeners={draggable.listeners}
            setNodeRef={draggable.setNodeRef}
            transform={draggable.transform}
            isDragging={draggable.isDragging}
            isPreviewed={isPreviewed}
            isSelected={isSelected}
            isMultiSelectMode={isMultiSelectMode}
            showMaximizeButton={showMaximizeButton}
            isDeleting={isDeleting}
            isArchiving={isArchiving}
            menuEntries={menu.dropdownMenuEntries}
            menuTriggerRef={menu.detachFocusReturnRef}
            onClick={onClick}
            onCheckboxClick={(e) => {
              e.stopPropagation();
              onToggleSelect?.(task.id);
            }}
            onOpenFullPage={onOpenFullPage}
          />
        </KanbanCardContextMenu>
      </div>
      <TaskDetachConfirmationSurface
        open={menu.showDetachConfirm}
        anchorRef={menu.detachAnchorRef}
        focusReturnRef={menu.detachFocusReturnRef}
        taskTitle={task.title}
        sharesParentWorkspace={task.workspaceMode === "inherit_parent"}
        onOpenChange={menu.setShowDetachConfirm}
        onConfirm={menu.handleDetachConfirm}
      />
      <TaskArchiveConfirmation
        open={menu.showArchiveConfirm}
        anchorRef={menu.archiveAnchorRef}
        focusReturnRef={menu.archiveFocusReturnRef}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primaryExecutorType}
        isArchiving={isArchiving}
        forceDialog={presentation === "mobile"}
        onOpenChange={menu.setShowArchiveConfirm}
        onConfirm={({ cascade }) => onArchive?.(task, { cascade })}
      />
    </>
  );
}

export function KanbanCard({
  task,
  workspaceId,
  presentation = "desktop",
  externalLinkAvailability,
  repositoryChips,
  onClick,
  onEdit,
  onDelete,
  onArchive,
  onOpenFullPage,
  onMove,
  steps,
  showMaximizeButton = false,
  isDeleting,
  isArchiving,
  isSelected,
  selectedIds,
  onToggleSelect,
  onRangeSelect,
  isMultiSelectMode,
}: KanbanCardProps) {
  const draggable = useDraggable({
    id: task.id,
    disabled: isMultiSelectMode,
  });
  const isPreviewed = useAppStore((state) => state.kanbanPreviewedTaskId === task.id);
  const repositories = useActiveWorkspaceRepositories();
  const menu = useKanbanCardMenus({
    task,
    workspaceId,
    presentation,
    externalLinkAvailability,
    steps,
    isDeleting,
    isArchiving,
    isSelected,
    selectedIds,
    onEdit,
    onDelete,
    onArchive,
    onMove,
  });

  const handleClick = (e: React.MouseEvent) =>
    dispatchKanbanCardClick(e, task.id, task, {
      onToggleSelect,
      onRangeSelect,
      onClick,
      isMultiSelectMode,
    });

  return (
    <>
      <KanbanCardFrame
        task={task}
        presentation={presentation}
        repositoryChips={repositoryChips}
        draggable={draggable}
        menu={menu}
        isPreviewed={isPreviewed}
        isSelected={isSelected}
        isMultiSelectMode={isMultiSelectMode}
        showMaximizeButton={showMaximizeButton}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        onArchive={onArchive}
        onClick={handleClick}
        onToggleSelect={onToggleSelect}
        onOpenFullPage={onOpenFullPage}
      />
      <KanbanCardDialogs
        task={task}
        workspaceId={workspaceId}
        repositories={repositories}
        menu={menu}
        isDeleting={isDeleting}
        onDelete={onDelete}
      />
    </>
  );
}
