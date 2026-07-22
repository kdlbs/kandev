"use client";

import { TaskRenameDialog } from "./task-rename-dialog";
import { TaskArchiveConfirmDialog } from "./task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "./task-delete-confirm-dialog";
import { TaskDetachTargetConfirmDialog } from "./task-detach-confirm-dialog";
import { TaskExternalLinkDialog } from "./task-external-link-dialog";
import { TaskGitHubIssueDialog } from "./task-github-issue-dialog";
import { TaskGitHubPRDialog } from "./task-github-pr-dialog";
import type { Repository } from "@/lib/types/http";
import type {
  SidebarExternalLinkTarget,
  SidebarLinkTarget,
} from "./task-session-sidebar-link-actions";

type Target = { id: string; title: string; executorType?: string | null } | null;
type DetachTarget = {
  id: string;
  title: string;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
} | null;
type LinkTarget = SidebarLinkTarget | null;

export type SidebarDialogsActions = {
  renamingTask: Target;
  setRenamingTask: (next: Target) => void;
  handleRenameSubmit: (newTitle: string) => Promise<void> | void;
  archivingTask: Target;
  setArchivingTask: (next: Target) => void;
  archivingTaskId: string | null;
  isArchiving: boolean;
  handleArchiveConfirm: (opts: { cascade: boolean }) => Promise<void> | void;
  deletingTask: Target;
  setDeletingTask: (next: Target) => void;
  isDeleting: boolean;
  handleDeleteConfirm: (opts: { cascade: boolean }) => Promise<void> | void;
  detachingTask: DetachTarget;
  setDetachingTask: (next: DetachTarget) => void;
  detachingTaskId: string | null;
  handleDetachConfirm: () => Promise<void> | void;
  linkingPullRequestTask: LinkTarget;
  setLinkingPullRequestTask: (next: LinkTarget) => void;
  linkingIssueTask: LinkTarget;
  setLinkingIssueTask: (next: LinkTarget) => void;
  linkingExternalIssueTask: SidebarExternalLinkTarget | null;
  setLinkingExternalIssueTask: (next: SidebarExternalLinkTarget | null) => void;
};

export function SidebarDialogs({
  actions,
  repositories,
  workspaceId,
}: {
  actions: SidebarDialogsActions;
  repositories: Repository[];
  workspaceId: string | null;
}) {
  const {
    renamingTask,
    setRenamingTask,
    handleRenameSubmit,
    archivingTask,
    setArchivingTask,
    archivingTaskId,
    isArchiving,
    handleArchiveConfirm,
    deletingTask,
    setDeletingTask,
    isDeleting,
    handleDeleteConfirm,
    detachingTask,
    setDetachingTask,
    detachingTaskId,
    handleDetachConfirm,
  } = actions;
  return (
    <>
      <TaskRenameDialog
        open={renamingTask !== null}
        onOpenChange={(open) => {
          if (!open) setRenamingTask(null);
        }}
        currentTitle={renamingTask?.title ?? ""}
        onSubmit={handleRenameSubmit}
      />
      <TaskArchiveConfirmDialog
        open={archivingTask !== null}
        onOpenChange={(open) => {
          if (!open) setArchivingTask(null);
        }}
        taskTitle={archivingTask?.title ?? ""}
        taskId={archivingTask?.id}
        executorType={archivingTask?.executorType}
        isArchiving={isArchiving && archivingTask?.id === archivingTaskId}
        onConfirm={handleArchiveConfirm}
      />
      <TaskDeleteConfirmDialog
        open={deletingTask !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingTask(null);
        }}
        taskTitle={deletingTask?.title ?? ""}
        taskId={deletingTask?.id}
        executorType={deletingTask?.executorType}
        isDeleting={isDeleting}
        onConfirm={handleDeleteConfirm}
      />
      <TaskDetachTargetConfirmDialog
        target={detachingTask}
        detachingTaskId={detachingTaskId}
        onDismiss={() => setDetachingTask(null)}
        onConfirm={handleDetachConfirm}
      />
      <SidebarLinkDialogs actions={actions} repositories={repositories} workspaceId={workspaceId} />
    </>
  );
}

export function SidebarLinkDialogs({
  actions,
  repositories,
  workspaceId,
}: {
  actions: Pick<
    SidebarDialogsActions,
    | "linkingPullRequestTask"
    | "setLinkingPullRequestTask"
    | "linkingIssueTask"
    | "setLinkingIssueTask"
    | "linkingExternalIssueTask"
    | "setLinkingExternalIssueTask"
  >;
  repositories: Repository[];
  workspaceId: string | null;
}) {
  const {
    linkingPullRequestTask,
    setLinkingPullRequestTask,
    linkingIssueTask,
    setLinkingIssueTask,
    linkingExternalIssueTask,
    setLinkingExternalIssueTask,
  } = actions;
  return (
    <>
      {linkingPullRequestTask && (
        <TaskGitHubPRDialog
          workspaceId={workspaceId}
          open={true}
          onOpenChange={(open) => {
            if (!open) setLinkingPullRequestTask(null);
          }}
          task={linkingPullRequestTask}
          repositories={repositories}
        />
      )}
      {linkingIssueTask && (
        <TaskGitHubIssueDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setLinkingIssueTask(null);
          }}
          task={linkingIssueTask}
          repositories={repositories}
        />
      )}
      {linkingExternalIssueTask && workspaceId && (
        <TaskExternalLinkDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setLinkingExternalIssueTask(null);
          }}
          provider={linkingExternalIssueTask.provider}
          task={linkingExternalIssueTask.task}
          workspaceId={workspaceId}
        />
      )}
    </>
  );
}
