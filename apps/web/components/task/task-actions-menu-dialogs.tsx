"use client";

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
};

type TaskActionsMenuEditDialogProps = {
  taskId: string;
  taskTitle: string;
  workspaceId: string | null;
  boardRow: TaskActionsMenuBoardRow;
  menu: TaskActionsMenuState;
};

function TaskActionsMenuEditDialog({
  taskId,
  taskTitle,
  workspaceId,
  boardRow,
  menu,
}: TaskActionsMenuEditDialogProps) {
  const store = useAppStoreApi();
  const editSteps = menu.stepsByWorkflowId[menu.currentWorkflowId ?? ""] ?? [];

  return (
    <TaskCreateDialog
      open={menu.showEditDialog}
      onOpenChange={menu.setShowEditDialog}
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
}: TaskActionsMenuDialogsProps) {
  const repositories = useActiveWorkspaceRepositories();
  if (!taskId) return null;
  const linkDialogTask = buildLinkDialogTask(taskId, taskTitle, boardRow);

  return (
    <>
      {boardRow && (
        <TaskActionsMenuEditDialog
          taskId={taskId}
          taskTitle={taskTitle}
          workspaceId={workspaceId}
          boardRow={boardRow}
          menu={menu}
        />
      )}
      <TaskArchiveConfirmation
        open={menu.showArchiveConfirm}
        anchorRef={menu.triggerRef}
        focusReturnRef={menu.triggerRef}
        taskTitle={taskTitle}
        taskId={taskId}
        executorType={boardRow?.primaryExecutorType}
        isArchiving={isArchiving}
        onOpenChange={menu.setShowArchiveConfirm}
        onConfirm={(values) => menu.onConfirmArchive(values)}
      />
      <TaskDeleteConfirmDialog
        open={menu.showDeleteConfirm}
        onOpenChange={menu.setShowDeleteConfirm}
        taskTitle={taskTitle}
        taskId={taskId}
        executorType={boardRow?.primaryExecutorType}
        isDeleting={isDeleting}
        onConfirm={(opts) => menu.onConfirmDelete(opts)}
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
      />
      <TaskGitHubIssueDialog
        open={menu.showIssueDialog}
        onOpenChange={menu.setShowIssueDialog}
        task={linkDialogTask}
        repositories={repositories}
      />
      {workspaceId && (
        <TaskMRLinkDialog
          open={menu.showMRDialog}
          onOpenChange={menu.setShowMRDialog}
          taskId={taskId}
          workspaceId={workspaceId}
          taskRepositories={boardRow?.repositories ?? []}
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
          task={linkDialogTask}
          workspaceId={workspaceId}
        />
      )}
    </>
  );
}
