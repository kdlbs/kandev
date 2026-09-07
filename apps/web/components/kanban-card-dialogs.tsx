"use client";

import type { Task } from "@/components/kanban-card";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { TaskGitHubPRDialog } from "@/components/task/task-github-pr-dialog";
import { TaskGitHubIssueDialog } from "@/components/task/task-github-issue-dialog";
import { TaskMRLinkDialog } from "@/components/gitlab/task-mr-link-dialog";
import {
  TaskExternalLinkDialog,
  type ExternalLinkProvider,
} from "@/components/task/task-external-link-dialog";
import type { Repository } from "@/lib/types/http";

/** The subset of card menu state the link/delete dialogs read and drive. */
export interface KanbanCardDialogsMenuState {
  showDeleteConfirm: boolean;
  setShowDeleteConfirm: (open: boolean) => void;
  showPRDialog: boolean;
  setShowPRDialog: (open: boolean) => void;
  showIssueDialog: boolean;
  setShowIssueDialog: (open: boolean) => void;
  showMRDialog: boolean;
  setShowMRDialog: (open: boolean) => void;
  externalLinkProvider: ExternalLinkProvider | null;
  setExternalLinkProvider: (provider: ExternalLinkProvider | null) => void;
}

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
  menu: KanbanCardDialogsMenuState;
  isDeleting?: boolean;
  onDelete?: (task: Task, opts?: { cascade?: boolean }) => void;
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
        onConfirm={(opts) => onDelete?.(task, opts)}
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
