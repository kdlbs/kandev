import { NewSubtaskDialog } from "../new-subtask-dialog";
import { TaskArchiveConfirmDialog } from "../task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "../task-delete-confirm-dialog";
import { TaskDetachTargetConfirmDialog } from "../task-detach-confirm-dialog";
import { TaskRenameDialog } from "../task-rename-dialog";
import { SidebarLinkDialogs } from "../task-session-sidebar-dialogs";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { SidebarTaskEditDialog, useSidebarTaskEdit } from "../task-session-sidebar-edit";
import type { useSheetData, useSheetActions } from "./session-task-switcher-sheet-hooks";
import type { useMobileTaskRename } from "./use-mobile-task-rename";
import type { useMobileTaskLinking } from "./session-task-switcher-sheet";

export function MobileSubtaskDialog({
  target,
  onTargetChange,
}: {
  target: { id: string; title: string } | null;
  onTargetChange: (next: { id: string; title: string } | null) => void;
}) {
  return (
    <NewSubtaskDialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) onTargetChange(null);
      }}
      parentTaskId={target?.id ?? ""}
      parentTaskTitle={target?.title ?? ""}
    />
  );
}

export function TaskSwitcherDialogs({
  dialogOpen,
  onDialogOpenChange,
  workspaceId,
  workflowId,
  data,
  actions,
  rename,
  edit,
  linking,
  subtaskTarget,
  onSubtaskTargetChange,
}: {
  dialogOpen: boolean;
  onDialogOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  workflowId: string | null;
  data: ReturnType<typeof useSheetData>;
  actions: ReturnType<typeof useSheetActions>;
  rename: ReturnType<typeof useMobileTaskRename>;
  edit: ReturnType<typeof useSidebarTaskEdit>;
  linking: ReturnType<typeof useMobileTaskLinking>;
  subtaskTarget: { id: string; title: string } | null;
  onSubtaskTargetChange: (target: { id: string; title: string } | null) => void;
}) {
  return (
    <>
      <TaskCreateDialog
        open={dialogOpen}
        onOpenChange={onDialogOpenChange}
        mode="create"
        workspaceId={workspaceId}
        workflowId={workflowId}
        defaultStepId={data.dialogSteps[0]?.id ?? null}
        steps={data.dialogSteps}
        onSuccess={actions.handleTaskCreated}
      />
      <MobileSubtaskDialog target={subtaskTarget} onTargetChange={onSubtaskTargetChange} />
      <SidebarTaskEditDialog
        target={edit.editingTask}
        onTargetChange={edit.setEditingTask}
        workspaceId={workspaceId}
        stepsByWorkflowId={data.stepsByWorkflowId}
      />
      <TaskArchiveConfirmDialog
        open={actions.archivingTask !== null}
        onOpenChange={(open) => {
          if (!open) actions.setArchivingTask(null);
        }}
        taskTitle={actions.archivingTask?.title ?? ""}
        taskId={actions.archivingTask?.id}
        executorType={actions.archivingTask?.executorType}
        isArchiving={actions.isArchiving}
        onConfirm={({ cascade }) => actions.handleArchiveConfirm({ cascade })}
      />
      <TaskRenameDialog
        open={rename.renamingTask !== null}
        onOpenChange={(open) => {
          if (!open) rename.setRenamingTask(null);
        }}
        currentTitle={rename.renamingTask?.title ?? ""}
        onSubmit={rename.handleRenameSubmit}
      />
      <TaskDeleteConfirmDialog
        open={actions.deletingTask !== null}
        onOpenChange={(open) => {
          if (!open) actions.setDeletingTask(null);
        }}
        taskTitle={actions.deletingTask?.title ?? ""}
        taskId={actions.deletingTask?.id}
        executorType={actions.deletingTask?.executorType}
        isDeleting={actions.isDeleting}
        onConfirm={({ cascade }) => actions.handleDeleteConfirm({ cascade })}
      />
      <TaskDetachTargetConfirmDialog
        target={actions.detachingTask}
        detachingTaskId={actions.detachingTaskId}
        onDismiss={() => actions.setDetachingTask(null)}
        onConfirm={actions.handleDetachConfirm}
      />
      <SidebarLinkDialogs
        actions={linking.actions}
        repositories={linking.repositories}
        workspaceId={workspaceId}
      />
    </>
  );
}
