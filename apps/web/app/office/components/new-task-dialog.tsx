"use client";

import { useCallback, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Textarea } from "@kandev/ui/textarea";
import { Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle } from "@kandev/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { createTask } from "@/lib/api/domains/kanban-api";
import { toMessageAttachments } from "@/components/task-create-dialog-helpers";
import {
  attachmentSubmissionBlocker,
  restoreTaskCreateAttachments,
  toStoredCreationAttachments,
  useTaskCreateAttachments,
} from "@/hooks/domains/task/use-task-create-attachments";
import { useIssueDraft, type IssueDraft } from "./new-task-draft";
import { NewTaskSelectorRow } from "./new-task-selector-row";
import { NewTaskBottomBar } from "./new-task-bottom-bar";
import { NewTaskAttachmentComposer } from "./new-task-attachment-composer";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";
import { useTranslation } from "react-i18next";

function buildMetadata(draft: IssueDraft): Record<string, unknown> | undefined {
  const meta: Record<string, unknown> = {};
  if (draft.status && draft.status !== "todo") meta.initial_status = draft.status;
  return Object.keys(meta).length > 0 ? meta : undefined;
}

type NewIssueDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  parentTaskId?: string;
  defaultProjectId?: string;
  defaultAssigneeId?: string;
};

export function NewTaskDialog({
  open,
  onOpenChange,
  parentTaskId,
  defaultProjectId,
  defaultAssigneeId,
}: NewIssueDialogProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  return (
    <NewTaskDialogSession
      key={`${workspaceId ?? "none"}:${parentTaskId ?? ""}`}
      open={open}
      workspaceId={workspaceId}
      onOpenChange={onOpenChange}
      parentTaskId={parentTaskId}
      defaultProjectId={defaultProjectId}
      defaultAssigneeId={defaultAssigneeId}
    />
  );
}

function useNewTaskSubmission({
  draft,
  workspaceId,
  parentTaskId,
  attachmentController,
  clearDraft,
  onOpenChange,
}: {
  draft: IssueDraft;
  workspaceId: string | null;
  parentTaskId?: string;
  attachmentController: ReturnType<typeof useTaskCreateAttachments>;
  clearDraft: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);

  const handleCreate = useCallback(async () => {
    if (submittingRef.current) return;
    if (!draft.title.trim() || !draft.projectId || !workspaceId) return;
    const blocker = attachmentSubmissionBlocker(
      attachmentController.attachments,
      attachmentController.isProcessing,
    );
    if (blocker) {
      toast.error(
        blocker === "uploading"
          ? t("chat:attachmentUploadPending")
          : t("task:attachmentUploadFailed"),
      );
      return;
    }
    submittingRef.current = true;
    attachmentController.setAccepting(false);
    setSubmitting(true);
    try {
      await createTask({
        workspace_id: workspaceId,
        workflow_id: "",
        title: draft.title.trim(),
        description: draft.description.trim() || undefined,
        parent_id: parentTaskId,
        priority: draft.priority,
        project_id: draft.projectId || undefined,
        assignee_agent_profile_id: draft.assigneeId || undefined,
        attachments: toMessageAttachments(attachmentController.attachments),
        metadata: buildMetadata(draft),
      });
      attachmentController.clearAttachments({ deleteStaged: false });
      attachmentController.setAccepting(true);
      clearDraft();
      onOpenChange(false);
      toast.success(t("office:taskCreated"));
    } catch (err) {
      attachmentController.setAccepting(true);
      toast.error(err instanceof Error ? err.message : t("office:failedToCreateIssue"));
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  }, [attachmentController, draft, workspaceId, parentTaskId, clearDraft, onOpenChange, t]);

  const handleDiscard = useCallback(() => {
    if (submittingRef.current) return;
    attachmentController.clearAttachments();
    clearDraft();
    onOpenChange(false);
  }, [attachmentController, clearDraft, onOpenChange]);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next && submittingRef.current) return;
      onOpenChange(next);
    },
    [onOpenChange],
  );

  return { submitting, handleCreate, handleDiscard, handleOpenChange };
}

function NewTaskDialogSession({
  open,
  workspaceId,
  onOpenChange,
  parentTaskId,
  defaultProjectId,
  defaultAssigneeId,
}: NewIssueDialogProps & { workspaceId: string | null }) {
  const { draft, updateDraft, clearDraft } = useIssueDraft(workspaceId, parentTaskId, {
    projectId: defaultProjectId,
    assigneeId: defaultAssigneeId,
  });
  // The saved descriptor set seeds the controller once; later writes go the
  // other way, from ready controller state into the draft record.
  const [initialAttachments] = useState(() => restoreTaskCreateAttachments(draft.attachments));

  const persistAttachments = useCallback(
    (attachments: Parameters<typeof toStoredCreationAttachments>[0]) => {
      updateDraft({ attachments: toStoredCreationAttachments(attachments) });
    },
    [updateDraft],
  );
  const attachmentController = useTaskCreateAttachments({
    workspaceId,
    initialAttachments,
    invalidateOnUnmount: true,
    onAttachmentsChange: persistAttachments,
  });
  const attachmentBlocker = attachmentSubmissionBlocker(
    attachmentController.attachments,
    attachmentController.isProcessing,
  );
  const { submitting, handleCreate, handleDiscard, handleOpenChange } = useNewTaskSubmission({
    draft,
    workspaceId,
    parentTaskId,
    attachmentController,
    clearDraft,
    onOpenChange,
  });

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        data-testid="office-new-issue-dialog"
        className="max-w-3xl sm:max-w-3xl lg:max-w-4xl max-md:inset-0 max-md:left-0 max-md:top-0 max-md:h-[100dvh] max-md:max-h-[100dvh] max-md:w-full max-md:max-w-none max-md:translate-x-0 max-md:translate-y-0 max-md:flex max-md:flex-col max-md:gap-0 max-md:rounded-none max-md:border-0 max-md:p-0"
      >
        <NewIssueDialogBody
          workspaceId={workspaceId}
          draft={draft}
          updateDraft={updateDraft}
          parentTaskId={parentTaskId}
          attachmentController={attachmentController}
          attachmentBlocker={attachmentBlocker}
          submitting={submitting}
          onDiscard={handleDiscard}
          onCreate={handleCreate}
        />
      </DialogContent>
    </Dialog>
  );
}

function NewIssueDialogBody({
  workspaceId,
  draft,
  updateDraft,
  parentTaskId,
  attachmentController,
  attachmentBlocker,
  submitting,
  onDiscard,
  onCreate,
}: {
  workspaceId: string | null;
  draft: IssueDraft;
  updateDraft: (patch: Partial<IssueDraft>) => void;
  parentTaskId?: string;
  attachmentController: ReturnType<typeof useTaskCreateAttachments>;
  attachmentBlocker: "uploading" | "failed" | null;
  submitting: boolean;
  onDiscard: () => void;
  onCreate: () => void;
}) {
  const { t } = useTranslation();
  const { inputRef, clampChange } = useTaskTitleSelectionRestore<HTMLTextAreaElement>(draft.title);
  return (
    <>
      <DialogHeader className="max-md:shrink-0 max-md:px-4 max-md:pt-4">
        <DialogTitle className="sr-only">{t("office:newIssue")}</DialogTitle>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="font-mono text-xs">
            KAN
          </Badge>
          <span className="text-sm text-muted-foreground">{t("office:newIssue")}</span>
          {parentTaskId && (
            <Badge variant="secondary" className="text-xs">
              {t("office:subIssueOfParent", { parent: parentTaskId })}
            </Badge>
          )}
        </div>
      </DialogHeader>

      <div className="space-y-4 max-md:min-h-0 max-md:flex-1 max-md:overflow-y-auto max-md:px-4 max-md:pb-2">
        <Textarea
          ref={inputRef}
          placeholder={t("office:taskTitle")}
          value={draft.title}
          onChange={(e) => updateDraft({ title: clampChange(e) })}
          className="text-lg font-medium border-0 resize-none focus-visible:ring-0 min-h-[40px]"
          rows={1}
          autoFocus
        />
        <NewTaskSelectorRow draft={draft} onUpdate={updateDraft} />
        {workspaceId && (
          <NewTaskAttachmentComposer
            workspaceId={workspaceId}
            description={draft.description}
            controller={attachmentController}
            disabled={submitting}
            onDescriptionChange={(description) => updateDraft({ description })}
          />
        )}
        <NewTaskBottomBar draft={draft} onUpdate={updateDraft} />
      </div>

      <DialogFooter className="flex justify-between sm:justify-between max-md:shrink-0 max-md:border-t max-md:border-border max-md:px-4 max-md:py-3 max-md:pb-[max(0.75rem,env(safe-area-inset-bottom))]">
        <Button
          variant="ghost"
          className="text-muted-foreground cursor-pointer"
          onClick={onDiscard}
          disabled={submitting}
        >
          {t("office:discardDraft")}
        </Button>
        <CreateTaskButton
          draft={draft}
          submitting={submitting}
          attachmentBlocker={attachmentBlocker}
          onCreate={onCreate}
        />
      </DialogFooter>
    </>
  );
}

export function CreateTaskButton({
  draft,
  submitting,
  attachmentBlocker = null,
  onCreate,
}: {
  draft: IssueDraft;
  submitting: boolean;
  attachmentBlocker?: "uploading" | "failed" | null;
  onCreate: () => void;
}) {
  const { t } = useTranslation();
  const missingTitle = !draft.title.trim();
  const missingProject = !draft.projectId;
  let attachmentReason: string | null = null;
  if (attachmentBlocker === "uploading") attachmentReason = t("chat:attachmentUploadPending");
  else if (attachmentBlocker === "failed") attachmentReason = t("task:attachmentUploadFailed");
  const disabled = missingTitle || missingProject || submitting || Boolean(attachmentReason);
  let reason: string | null = null;
  if (missingProject) reason = t("office:selectAProjectToCreateATask");
  else if (missingTitle) reason = t("office:addATitleToCreateATask");
  else if (attachmentReason) reason = attachmentReason;

  const button = (
    <Button
      onClick={onCreate}
      disabled={disabled}
      className="cursor-pointer"
      data-testid="new-task-create-button"
    >
      {submitting ? t("office:creating") : t("office:createTask")}
    </Button>
  );

  if (!reason) return button;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0} aria-label={reason}>
          {button}
        </span>
      </TooltipTrigger>
      <TooltipContent>{reason}</TooltipContent>
    </Tooltip>
  );
}
