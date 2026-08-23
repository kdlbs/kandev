"use client";

import { useRef, useState } from "react";
import { useDraggable } from "@dnd-kit/core";
import { KanbanCardContextMenu } from "@/components/kanban-card-context-menu";
import { KanbanCardShell } from "@/components/kanban-card-content";
import { useActiveWorkspaceRepositories } from "@/components/kanban-card-repositories";
export { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import {
  buildKanbanCardMenuEntries,
  useKanbanCardMoveTargets,
} from "@/components/kanban-card-menu-items";
import { useTaskPluginLinkActions } from "@/components/task/task-session-sidebar-link-actions";
import { useAppStore } from "@/components/state-provider";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { TaskDetachConfirmationSurface } from "@/components/task/task-detach-confirm-dialog";
import {
  TaskExternalLinkDialog,
  type ExternalLinkProvider,
} from "@/components/task/task-external-link-dialog";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import type { TaskDependencyRef } from "@/lib/state/slices/kanban/types";
import { TaskGitHubIssueDialog } from "@/components/task/task-github-issue-dialog";
import { TaskGitHubPRDialog } from "@/components/task/task-github-pr-dialog";
import { TaskMRLinkDialog } from "@/components/gitlab/task-mr-link-dialog";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useTaskMultiSelectStore } from "@/hooks/use-task-multi-select";
import { useDetachTask } from "@/hooks/use-detach-task";
import {
  type ForegroundActivity,
  type Repository,
  type TaskPendingAction,
  type TaskPriority,
  type TaskState,
} from "@/lib/types/http";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { usePluginRegistry } from "@/lib/plugins/registry";

export interface Task {
  id: string;
  title: string;
  workflowStepId: string;
  state?: TaskState;
  priority?: TaskPriority;
  description?: string;
  position?: number;
  repositoryId?: string;
  /** All repositories linked to the task; used to render a "+N" chip for multi-repo. */
  repositories?: Array<{ id: string; repository_id: string; position: number }>;
  sessionCount?: number | null;
  primarySessionId?: string | null;
  /**
   * Primary session's runtime state. Decoupled from `state` (the workflow
   * column). Used to suppress the running-spinner when the agent has already
   * finished — the workflow may leave the task in IN_PROGRESS for review.
   */
  primarySessionState?: string | null;
  primarySessionPendingAction?: TaskPendingAction | null;
  taskPendingAction?: TaskPendingAction | null;
  /**
   * Task-level MOST-ACTIVE-WINS activity aggregate;
   * undefined/null when no session is running. Drives the background-running
   * affordance on the card status icon.
   */
  foregroundActivity?: ForegroundActivity | null;
  /** True when the task's session was mid-turn when the backend died. */
  interrupted?: boolean;
  /** True when a workflow step's auto_start_agent on_enter action failed to
   *  launch a run for this task. */
  autoStartFailed?: boolean;
  /** Live subagents summed across this task's sessions; drives the count chip. */
  activeSubagentCount?: number;
  reviewStatus?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primaryExecutorId?: string | null;
  primaryExecutorType?: string | null;
  primaryExecutorName?: string | null;
  isRemoteExecutor?: boolean;
  parentTaskId?: string | null;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
  updatedAt?: string;
  createdAt?: string;
  wipAdmitted?: boolean;
  queuedForStepId?: string;
  queuedForStepTitle?: string;
  /** Derived dependency state — see TaskDependencyRef in the kanban slice. */
  blocked?: boolean;
  blockedReason?: string;
  dependsOn?: TaskDependencyRef[];
  blocks?: TaskDependencyRef[];
  startWhenUnblocked?: boolean;
  queuedAt?: string;
  issueUrl?: string;
  issueNumber?: number;
}

export type RepositoryChip = {
  label: string;
  path?: string;
};

export interface WorkflowStep {
  id: string;
  title: string;
  color: string;
  events?: {
    on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_start?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_exit?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
}

export type KanbanPresentation = PluginTaskMenuContext["presentation"];

interface KanbanCardProps {
  task: Task;
  workspaceId: string | null;
  presentation?: KanbanPresentation;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  /** Display labels and hover paths of every repository linked to the task, primary first. */
  repositoryChips?: RepositoryChip[];
  onClick?: (task: Task) => void;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task, opts?: { cascade?: boolean }) => void;
  onArchive?: (task: Task, opts?: { cascade?: boolean }) => void;
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
    if (onMove) {
      onMove(task, stepId);
      return;
    }
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

type KanbanCardMenuState = ReturnType<typeof useKanbanCardMenus>;

function KanbanCardDialogs({
  task,
  workspaceId,
  repositories,
  menu,
  isDeleting,
  onDelete,
}: {
  task: Task;
  workspaceId: string | null;
  repositories: Repository[];
  menu: KanbanCardMenuState;
  isDeleting?: boolean;
  onDelete?: KanbanCardProps["onDelete"];
}) {
  return (
    <>
      <TaskDeleteConfirmDialog
        open={menu.showDeleteConfirm}
        onOpenChange={menu.setShowDeleteConfirm}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primaryExecutorType}
        isDeleting={isDeleting}
        onConfirm={({ cascade }) => onDelete?.(task, { cascade })}
      />
      <TaskGitHubPRDialog
        workspaceId={workspaceId}
        open={menu.showPRDialog}
        onOpenChange={menu.setShowPRDialog}
        task={task}
        repositories={repositories}
      />
      <TaskGitHubIssueDialog
        open={menu.showIssueDialog}
        onOpenChange={menu.setShowIssueDialog}
        task={task}
        repositories={repositories}
      />
      {workspaceId && (
        <TaskMRLinkDialog
          open={menu.showMRDialog}
          onOpenChange={menu.setShowMRDialog}
          taskId={task.id}
          workspaceId={workspaceId}
          taskRepositories={task.repositories ?? []}
          repositories={repositories}
        />
      )}
      {menu.externalLinkProvider && workspaceId && (
        <TaskExternalLinkDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) menu.setExternalLinkProvider(null);
          }}
          provider={menu.externalLinkProvider}
          task={task}
          workspaceId={workspaceId}
        />
      )}
    </>
  );
}

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
