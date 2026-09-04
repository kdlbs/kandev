"use client";

import { useRef, useState } from "react";
import {
  buildKanbanCardMenuEntries,
  useKanbanCardMoveTargets,
} from "@/components/kanban-card-menu-items";
import { useTaskPluginLinkActions } from "@/components/task/task-session-sidebar-link-actions";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import {
  TaskExternalLinkDialog,
  type ExternalLinkProvider,
} from "@/components/task/task-external-link-dialog";
import { TaskGitHubIssueDialog } from "@/components/task/task-github-issue-dialog";
import { TaskGitHubPRDialog } from "@/components/task/task-github-pr-dialog";
import { TaskMRLinkDialog } from "@/components/gitlab/task-mr-link-dialog";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useTaskMultiSelectStore } from "@/hooks/use-task-multi-select";
import { useDetachTask } from "@/hooks/use-detach-task";
import type { Repository } from "@/lib/types/http";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import type { KanbanPresentation, Task, WorkflowStep } from "@/components/kanban-card";

/** Inputs shared by both the Kanban card and the pipeline row's task menu. */
export interface TaskCardMenuParams {
  task: Task;
  workspaceId: string | null;
  presentation?: KanbanPresentation;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  steps?: WorkflowStep[];
  isDeleting?: boolean;
  isArchiving?: boolean;
  /** Row-local in-flight move guard: disables move/send-to-workflow entries. */
  isMoving?: boolean;
  isSelected?: boolean;
  selectedIds?: Set<string>;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task, opts?: { cascade?: boolean }) => void;
  onArchive?: (task: Task, opts?: { cascade?: boolean }) => void;
  onMove?: (task: Task, targetStepId: string) => void;
}

function useKanbanCardMoveMenuActions({
  task,
  steps,
  isSelected,
  selectedIds,
  onMove,
}: Pick<TaskCardMenuParams, "task" | "steps" | "isSelected" | "selectedIds" | "onMove">) {
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
  availability: TaskCardMenuParams["externalLinkAvailability"],
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

export function useKanbanCardMenus({
  task,
  workspaceId,
  presentation = "desktop",
  steps,
  isDeleting,
  isArchiving,
  isMoving,
  isSelected,
  selectedIds,
  onEdit,
  onDelete,
  onArchive,
  onMove,
  externalLinkAvailability,
}: TaskCardMenuParams) {
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
  const disabled = Boolean(isDeleting || isArchiving || isDetaching || isMoving);
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

export type KanbanCardMenuState = ReturnType<typeof useKanbanCardMenus>;

/** Renders the non-anchored menu dialogs (delete, PR/issue/MR link, external link). */
export function KanbanCardDialogs({
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
  onDelete?: TaskCardMenuParams["onDelete"];
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
