"use client";

import { useRef, type RefObject } from "react";
import { useActiveWorkspaceRepositories } from "@/components/kanban-card-repositories";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { TaskDetachConfirmationSurface } from "@/components/task/task-detach-confirm-dialog";
import { TaskExternalLinkDialog } from "@/components/task/task-external-link-dialog";
import { TaskGitHubIssueDialog } from "@/components/task/task-github-issue-dialog";
import { TaskGitHubPRDialog } from "@/components/task/task-github-pr-dialog";
import { TaskMRLinkDialog } from "@/components/gitlab/task-mr-link-dialog";
import type { Task } from "@/components/kanban-card";
import type { useTaskActionsMenu, TaskActionsMenuBoardRow } from "@/hooks/use-task-actions-menu";
import { hydrateEditedTask } from "@/hooks/domains/kanban/use-kanban-actions";
import { useAppStoreApi } from "@/components/state-provider";

type TaskActionsMenuState = ReturnType<typeof useTaskActionsMenu>;

function buildLinkDialogTask(
  taskId: string,
  taskTitle: string,
  boardRow: TaskActionsMenuBoardRow | null,
): Task {
  return {
    id: taskId,
    title: taskTitle,
    workflowStepId: boardRow?.workflowStepId ?? "",
    repositories: boardRow?.repositories,
  };
}

type TaskActionsMenuDialogsProps = {
  taskId: string | null;
  taskTitle: string;
  workspaceId: string | null;
  boardRow: TaskActionsMenuBoardRow | null;
  isArchiving?: boolean;
  isDeleting?: boolean;
  menu: TaskActionsMenuState;
  /** The subject's own executor type, independent of `boardRow`: falls back
   * to this for the Archive/Delete confirmations' executor-specific cleanup
   * copy when the board row is unresolvable (e.g. an archived task). */
  subjectExecutorType?: string | null;
};

type TaskActionsMenuEditDialogProps = {
  taskId: string;
  taskTitle: string;
  workspaceId: string | null;
  boardRow: TaskActionsMenuBoardRow;
  /** Whether the dialog should be visually open. Distinct from
   * `menu.showEditDialog`: this also folds in board-row availability, so a
   * board row lost mid-edit closes the dialog through its own close
   * transition (and focus-return) rather than an abrupt unmount. */
  open: boolean;
  focusReturnRef: RefObject<HTMLElement | null>;
  menu: TaskActionsMenuState;
};

function TaskActionsMenuEditDialog({
  taskId,
  taskTitle,
  workspaceId,
  boardRow,
  open,
  focusReturnRef,
  menu,
}: TaskActionsMenuEditDialogProps) {
  const store = useAppStoreApi();
  const editSteps = menu.stepsByWorkflowId[menu.currentWorkflowId ?? ""] ?? [];

  return (
    <TaskCreateDialog
      open={open}
      onOpenChange={menu.setShowEditDialog}
      focusReturnRef={focusReturnRef}
      mode="edit"
      workspaceId={workspaceId}
      workflowId={menu.currentWorkflowId}
      lockedFields={{ workflow: true }}
      defaultStepId={boardRow.workflowStepId ?? null}
      steps={editSteps}
      editingTask={{
        id: taskId,
        title: taskTitle,
        description: boardRow.description,
        workflowStepId: boardRow.workflowStepId ?? "",
        state: boardRow.state,
        repositoryId: boardRow.repositoryId,
        repositories: boardRow.repositories,
      }}
      initialValues={{
        title: taskTitle,
        description: boardRow.description,
        state: boardRow.state,
        repositoryId: boardRow.repositoryId,
        repositories: boardRow.repositories,
      }}
      onSuccess={(task) => hydrateEditedTask(store, task, store.getState().kanban)}
    />
  );
}

/**
 * Tracks the most recent non-null board row seen for the current subject.
 * Lets the Edit dialog stay mounted (and so run its own close-focus
 * transition) when the row disappears mid-edit, instead of losing its data
 * to an abrupt unmount the instant `boardRow` goes null.
 */
function useLastResolvedBoardRow(
  taskId: string | null,
  boardRow: TaskActionsMenuBoardRow | null,
): TaskActionsMenuBoardRow | null {
  const ref = useRef<{ taskId: string | null; boardRow: TaskActionsMenuBoardRow | null }>({
    taskId,
    boardRow,
  });
  if (ref.current.taskId !== taskId || boardRow != null) {
    ref.current = { taskId, boardRow };
  }
  return ref.current.boardRow;
}

/**
 * Mounts every dialog a task actions menu entry can open. Hosted per surface
 * so a dialog outlives the menu that opened it.
 */
export function TaskActionsMenuDialogs({
  taskId,
  taskTitle,
  workspaceId,
  boardRow,
  isArchiving,
  isDeleting,
  menu,
  subjectExecutorType,
}: TaskActionsMenuDialogsProps) {
  const repositories = useActiveWorkspaceRepositories();
  const editBoardRow = useLastResolvedBoardRow(taskId, boardRow);
  if (!taskId) return null;
  const linkDialogTask = buildLinkDialogTask(taskId, taskTitle, boardRow);
  const executorType = boardRow?.primaryExecutorType ?? subjectExecutorType;

  return (
    <>
      {editBoardRow && (
        <TaskActionsMenuEditDialog
          taskId={taskId}
          taskTitle={taskTitle}
          workspaceId={workspaceId}
          boardRow={editBoardRow}
          open={boardRow != null && menu.showEditDialog}
          focusReturnRef={menu.triggerRef}
          menu={menu}
        />
      )}
      <TaskArchiveConfirmation
        open={menu.showArchiveConfirm}
        anchorRef={menu.triggerRef}
        focusReturnRef={menu.triggerRef}
        taskTitle={taskTitle}
        taskId={taskId}
        executorType={executorType}
        isArchiving={isArchiving}
        onOpenChange={menu.setShowArchiveConfirm}
        onConfirm={(values) => menu.onConfirmArchive(values)}
      />
      <TaskDeleteConfirmDialog
        open={menu.showDeleteConfirm}
        onOpenChange={menu.setShowDeleteConfirm}
        taskTitle={taskTitle}
        taskId={taskId}
        executorType={executorType}
        isDeleting={isDeleting}
        onConfirm={(opts) => menu.onConfirmDelete(opts)}
        focusReturnRef={menu.triggerRef}
      />
      <TaskDetachConfirmationSurface
        open={menu.showDetachConfirm}
        anchorRef={menu.triggerRef}
        focusReturnRef={menu.triggerRef}
        taskTitle={taskTitle}
        sharesParentWorkspace={boardRow?.workspaceMode === "inherit_parent"}
        onOpenChange={menu.setShowDetachConfirm}
        onConfirm={menu.handleDetachConfirm}
      />
      <TaskGitHubPRDialog
        workspaceId={workspaceId}
        open={menu.showPRDialog}
        onOpenChange={menu.setShowPRDialog}
        task={linkDialogTask}
        repositories={repositories}
        focusReturnRef={menu.triggerRef}
      />
      <TaskGitHubIssueDialog
        open={menu.showIssueDialog}
        onOpenChange={menu.setShowIssueDialog}
        task={linkDialogTask}
        repositories={repositories}
        focusReturnRef={menu.triggerRef}
      />
      {workspaceId && (
        <TaskMRLinkDialog
          open={menu.showMRDialog}
          onOpenChange={menu.setShowMRDialog}
          taskId={taskId}
          workspaceId={workspaceId}
          taskRepositories={boardRow?.repositories ?? []}
          repositories={repositories}
          focusReturnRef={menu.triggerRef}
        />
      )}
      {menu.externalLinkProvider && workspaceId && (
        <TaskExternalLinkDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) menu.setExternalLinkProvider(null);
          }}
          provider={menu.externalLinkProvider}
          task={linkDialogTask}
          workspaceId={workspaceId}
          focusReturnRef={menu.triggerRef}
        />
      )}
    </>
  );
}
