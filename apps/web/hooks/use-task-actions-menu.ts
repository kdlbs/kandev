"use client";

import { useCallback, useEffect, useRef } from "react";
import type { KanbanPresentation } from "@/components/kanban-card";
import type { KanbanCardMoveTargets } from "@/components/kanban-card-menu-items";
import type { WorkflowStep } from "@/components/kanban-card";
import { useTaskActionsMenuMoveTargets } from "@/hooks/use-task-actions-menu-move-targets";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import {
  useTaskMenuDialogState,
  useTaskMenuEditDialogState,
} from "@/hooks/use-task-menu-dialog-state";
import { useDetachTask } from "@/hooks/use-detach-task";
import { useTaskPluginLinkActions } from "@/components/task/task-session-sidebar-link-actions";
import { useKanbanExternalLinkAvailability } from "@/components/kanban-external-link-availability";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import type { TaskState } from "@/lib/types/http";
import { buildTaskActionsMenuEntries } from "@/lib/kanban/task-actions-menu-entries";

export type TaskActionsMenuBoardRow = {
  id: string;
  title: string;
  description?: string;
  workflowStepId?: string | null;
  state?: TaskState;
  repositoryId?: string;
  repositories?: Array<{
    id: string;
    repository_id: string;
    position: number;
    base_branch?: string;
    checkout_branch?: string;
    branch_policy_id?: string;
    branch_policy_name?: string;
    branch_policy_base_branch?: string;
    branch_policy_branch_template?: string;
    branch_policy_pull_request_target?: string;
  }>;
  parentTaskId?: string | null;
  primaryExecutorType?: string | null;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
};

function resolveTaskActionsMenuTier(
  isArchived: boolean,
  boardRow: TaskActionsMenuBoardRow | null,
): "archived" | "normal" | "unresolved-row" {
  if (isArchived) return "archived";
  return boardRow ? "normal" : "unresolved-row";
}

function buildLinkHandlers(
  dialogs: ReturnType<typeof useTaskMenuDialogState>,
  availability: ReturnType<typeof useKanbanExternalLinkAvailability>,
) {
  return {
    onLinkPullRequest: () => dialogs.setShowPRDialog(true),
    onLinkIssue: () => dialogs.setShowIssueDialog(true),
    onLinkMergeRequest: availability.gitlab ? () => dialogs.setShowMRDialog(true) : undefined,
    onLinkJiraTicket: availability.jira ? () => dialogs.setExternalLinkProvider("jira") : undefined,
    onLinkLinearIssue: availability.linear
      ? () => dialogs.setExternalLinkProvider("linear")
      : undefined,
    onLinkSentryIssue: availability.sentry
      ? () => dialogs.setExternalLinkProvider("sentry")
      : undefined,
  };
}

type ComputeEntriesArgs = {
  taskId: string | null;
  tier: ReturnType<typeof resolveTaskActionsMenuTier>;
  boardRow: TaskActionsMenuBoardRow | null;
  moveTargets: KanbanCardMoveTargets;
  disabled: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isDetaching: boolean;
  editDialog: ReturnType<typeof useTaskMenuEditDialogState>;
  dialogs: ReturnType<typeof useTaskMenuDialogState>;
  requestArchiveConfirmation: () => void;
  requestDetachConfirmation: () => void;
  linkHandlers: ReturnType<typeof buildLinkHandlers>;
  pluginLinkActions: ReturnType<typeof useTaskPluginLinkActions>;
  onMoveToStep: (stepId: string) => void;
  onSendToWorkflow: (workflowId: string, stepId: string) => void;
  pluginMenuContext: PluginTaskMenuContext;
};

function computeTaskActionsMenuEntries(args: ComputeEntriesArgs) {
  if (args.taskId == null) return [];
  const { boardRow, moveTargets, editDialog, dialogs } = args;
  return buildTaskActionsMenuEntries(args.tier, {
    currentWorkflowId: moveTargets.currentWorkflowId,
    currentStepId: boardRow?.workflowStepId ?? null,
    workflows: moveTargets.workflowItems,
    stepsByWorkflowId: moveTargets.stepsByWorkflowId,
    disabled: args.disabled,
    isDeleting: args.isDeleting,
    isArchiving: args.isArchiving,
    isDetaching: args.isDetaching,
    parentTaskId: boardRow?.parentTaskId,
    onEdit: boardRow ? () => editDialog.setShowEditDialog(true) : undefined,
    onArchive: args.requestArchiveConfirmation,
    onDelete: () => dialogs.setShowDeleteConfirm(true),
    onDetach: boardRow?.parentTaskId ? args.requestDetachConfirmation : undefined,
    ...args.linkHandlers,
    pluginLinkActions: args.pluginLinkActions,
    onMoveToStep: args.onMoveToStep,
    onSendToWorkflow: args.onSendToWorkflow,
    pluginMenuContext: args.pluginMenuContext,
  });
}

/**
 * Keeps every confirm/link dialog scoped to the subject that opened it
 * (AC-TASKS-TASK-ACTIONS-MENU-004.5a's no-retarget rule): resets them all on
 * a subject-identity change, and guards the two 300ms-delayed confirmations
 * so a subject swap during that window silently drops the request instead of
 * opening for the new subject.
 */
function useSubjectScopedDialogs(
  taskId: string | null,
  dialogs: ReturnType<typeof useTaskMenuDialogState>,
  editDialog: ReturnType<typeof useTaskMenuEditDialogState>,
) {
  const taskIdRef = useRef(taskId);
  taskIdRef.current = taskId;

  const closeDialogs = useCallback(() => {
    dialogs.closeAll();
    editDialog.closeAll();
  }, [dialogs, editDialog]);

  // The preview panel re-renders this hook with a new `task` prop without
  // unmounting, and the confirm/link popovers are non-modal, so nothing else
  // would close a dialog opened for the previous subject.
  useEffect(() => {
    closeDialogs();
    // Only the subject identity should trigger this; `closeDialogs` is stable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId]);

  const requestDetachConfirmation = useCallback(() => {
    // Let Radix finish the menu's pointer sequence before the non-modal
    // popover opens; otherwise the initiating menu event is an outside click.
    const requestedForTaskId = taskId;
    window.setTimeout(() => {
      if (taskIdRef.current !== requestedForTaskId) return;
      dialogs.setShowDetachConfirm(true);
    }, 300);
  }, [dialogs, taskId]);

  const requestArchiveConfirmation = useCallback(() => {
    const requestedForTaskId = taskId;
    window.setTimeout(() => {
      if (taskIdRef.current !== requestedForTaskId) return;
      dialogs.setShowArchiveConfirm(true);
    }, 300);
  }, [dialogs, taskId]);

  return { closeDialogs, requestDetachConfirmation, requestArchiveConfirmation };
}

type UseTaskActionsMenuArgs = {
  taskId: string | null;
  taskTitle: string;
  workspaceId: string | null;
  presentation?: KanbanPresentation;
  isArchived: boolean;
  /** The subject's board row, when resolvable. Null demotes to the identifier-only tier. */
  boardRow: TaskActionsMenuBoardRow | null;
  /** Current-workflow steps for the Move to submenu. */
  steps?: WorkflowStep[];
  isArchiving?: boolean;
  isDeleting?: boolean;
  onArchive: (opts: { cascade: boolean }) => void | Promise<void>;
  onDelete: (opts: { cascade: boolean }) => void | Promise<void>;
  /** The subject's own workflow step, independent of `boardRow` (AC-002.4b's
   * plugin task-menu context must not go stale just because the board
   * excludes the subject, e.g. an archived task). Falls back to
   * `boardRow?.workflowStepId` for callers with no independent source. */
  subjectWorkflowStepId?: string | null;
};

export function useTaskActionsMenu({
  taskId,
  taskTitle,
  workspaceId,
  presentation = "desktop",
  isArchived,
  boardRow,
  steps,
  isArchiving,
  isDeleting,
  onArchive,
  onDelete,
  subjectWorkflowStepId,
}: UseTaskActionsMenuArgs) {
  const dialogs = useTaskMenuDialogState();
  const editDialog = useTaskMenuEditDialogState();
  const { detachTask, detachingTaskId } = useDetachTask();
  const isDetaching = taskId != null && detachingTaskId === taskId;
  const externalLinkAvailability = useKanbanExternalLinkAvailability(workspaceId);
  const pluginLinkActions = useTaskPluginLinkActions(taskId ?? "", boardRow?.repositories ?? []);
  // Re-render on any registry mutation so a menu action a plugin registers or
  // disables at runtime appears/disappears in an already-open menu (AC-002.8).
  usePluginRegistry();
  const moveTargets = useTaskActionsMenuMoveTargets(taskId ?? "", steps);
  const moveTasks = useTaskWorkflowMove();

  const triggerRef = useRef<HTMLButtonElement>(null);

  const disabled = Boolean(isArchiving || isDeleting || isDetaching);

  const { closeDialogs, requestDetachConfirmation, requestArchiveConfirmation } =
    useSubjectScopedDialogs(taskId, dialogs, editDialog);

  const handleDetachConfirm = useCallback(async () => {
    if (!taskId) return;
    try {
      await detachTask(taskId);
      dialogs.setShowDetachConfirm(false);
    } catch (error) {
      console.error("Failed to detach task:", error);
    }
  }, [detachTask, dialogs, taskId]);

  const onMoveToStep = useCallback(
    (stepId: string) => {
      if (!taskId || !moveTargets.currentWorkflowId) return;
      void moveTasks([taskId], moveTargets.currentWorkflowId, stepId, "step").catch(() => {
        // useTaskWorkflowMove already shows the failure toast.
      });
    },
    [moveTasks, moveTargets.currentWorkflowId, taskId],
  );

  const onSendToWorkflow = useCallback(
    (workflowId: string, stepId: string) => {
      if (!taskId) return;
      void moveTasks([taskId], workflowId, stepId, "workflow").catch(() => {
        // useTaskWorkflowMove already shows the failure toast.
      });
    },
    [moveTasks, taskId],
  );

  const pluginMenuContext: PluginTaskMenuContext = {
    workspaceId: workspaceId ?? "",
    taskId: taskId ?? "",
    taskTitle,
    workflowStepId: subjectWorkflowStepId ?? boardRow?.workflowStepId ?? null,
    presentation,
  };

  const tier = resolveTaskActionsMenuTier(Boolean(isArchived), boardRow);
  const linkHandlers = buildLinkHandlers(dialogs, externalLinkAvailability);

  const entries = computeTaskActionsMenuEntries({
    taskId,
    tier,
    boardRow,
    moveTargets,
    disabled,
    isDeleting,
    isArchiving,
    isDetaching,
    editDialog,
    dialogs,
    requestArchiveConfirmation,
    requestDetachConfirmation,
    linkHandlers,
    pluginLinkActions,
    onMoveToStep,
    onSendToWorkflow,
    pluginMenuContext,
  });

  // Strip each state slice's own `closeAll` before spreading: `closeDialogs`
  // above is the single composed reset callers should use, and leaving both
  // `dialogs.closeAll`/`editDialog.closeAll` in the spread would collide under
  // one `closeAll` key that only resets the edit dialog (last spread wins).
  const { closeAll: _dialogsCloseAll, ...dialogsRest } = dialogs;
  const { closeAll: _editDialogCloseAll, ...editDialogRest } = editDialog;

  return {
    entries,
    ...dialogsRest,
    ...editDialogRest,
    isDetaching,
    handleDetachConfirm,
    triggerRef,
    currentWorkflowId: moveTargets.currentWorkflowId,
    stepsByWorkflowId: moveTargets.stepsByWorkflowId,
    onConfirmArchive: onArchive,
    onConfirmDelete: onDelete,
    closeDialogs,
  };
}
