"use client";

import { useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Textarea } from "@kandev/ui/textarea";
import { Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle } from "@kandev/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { createTask } from "@/lib/api/domains/kanban-api";
import { useIssueDraft, type IssueDraft } from "./new-task-draft";
import { NewTaskSelectorRow } from "./new-task-selector-row";
import { NewTaskBottomBar } from "./new-task-bottom-bar";
import {
  NewTaskStages,
  buildExecutionPolicy,
  EMPTY_STAGES,
  type StagesDraft,
} from "./new-task-stages";
import { clampTaskTitleInput } from "@/lib/task-title";
import { useTranslation } from "react-i18next";

function buildMetadata(draft: IssueDraft): Record<string, unknown> | undefined {
  const meta: Record<string, unknown> = {};
  if (draft.assigneeId) meta.assignee_agent_profile_id = draft.assigneeId;
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
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [submitting, setSubmitting] = useState(false);
  const [stages, setStages] = useState<StagesDraft>(EMPTY_STAGES);

  const { draft, updateDraft, clearDraft } = useIssueDraft(workspaceId, parentTaskId, {
    projectId: defaultProjectId,
    assigneeId: defaultAssigneeId,
  });

  const updateStages = useCallback(
    (patch: Partial<StagesDraft>) => setStages((prev) => ({ ...prev, ...patch })),
    [],
  );

  const handleCreate = useCallback(async () => {
    if (!draft.title.trim() || !draft.projectId || !workspaceId) return;
    setSubmitting(true);
    try {
      const executionPolicy = buildExecutionPolicy(stages);
      const metadata = buildMetadata(draft);
      await createTask({
        workspace_id: workspaceId,
        workflow_id: "",
        title: draft.title.trim(),
        description: draft.description.trim() || undefined,
        parent_id: parentTaskId,
        priority: draft.priority,
        project_id: draft.projectId || undefined,
        metadata: executionPolicy ? { ...metadata, execution_policy: executionPolicy } : metadata,
      });
      clearDraft();
      setStages(EMPTY_STAGES);
      onOpenChange(false);
      toast.success(t("office:taskCreated"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToCreateIssue"));
    } finally {
      setSubmitting(false);
    }
  }, [draft, stages, workspaceId, parentTaskId, clearDraft, onOpenChange, t]);

  const handleDiscard = useCallback(() => {
    clearDraft();
    onOpenChange(false);
  }, [clearDraft, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="office-new-issue-dialog"
        className="max-w-3xl sm:max-w-3xl lg:max-w-4xl"
      >
        <NewIssueDialogBody
          draft={draft}
          updateDraft={updateDraft}
          stages={stages}
          updateStages={updateStages}
          parentTaskId={parentTaskId}
          submitting={submitting}
          onDiscard={handleDiscard}
          onCreate={handleCreate}
        />
      </DialogContent>
    </Dialog>
  );
}

function NewIssueDialogBody({
  draft,
  updateDraft,
  stages,
  updateStages,
  parentTaskId,
  submitting,
  onDiscard,
  onCreate,
}: {
  draft: IssueDraft;
  updateDraft: (patch: Partial<IssueDraft>) => void;
  stages: StagesDraft;
  updateStages: (patch: Partial<StagesDraft>) => void;
  parentTaskId?: string;
  submitting: boolean;
  onDiscard: () => void;
  onCreate: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
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

      <div className="space-y-4">
        <Textarea
          placeholder={t("office:taskTitle")}
          value={draft.title}
          onChange={(e) => updateDraft({ title: clampTaskTitleInput(e.target.value) })}
          className="text-lg font-medium border-0 resize-none focus-visible:ring-0 min-h-[40px]"
          rows={1}
          autoFocus
        />
        <NewTaskSelectorRow draft={draft} onUpdate={updateDraft} />
        <Textarea
          placeholder={t("office:addDescription")}
          value={draft.description}
          onChange={(e) => updateDraft({ description: e.target.value })}
          className="min-h-[120px] text-sm"
        />
        <NewTaskStages stages={stages} onUpdate={updateStages} />
        <NewTaskBottomBar draft={draft} onUpdate={updateDraft} />
      </div>

      <DialogFooter className="flex justify-between sm:justify-between">
        <Button
          variant="ghost"
          className="text-muted-foreground cursor-pointer"
          onClick={onDiscard}
        >
          {t("office:discardDraft")}
        </Button>
        <CreateTaskButton draft={draft} submitting={submitting} onCreate={onCreate} />
      </DialogFooter>
    </>
  );
}

export function CreateTaskButton({
  draft,
  submitting,
  onCreate,
}: {
  draft: IssueDraft;
  submitting: boolean;
  onCreate: () => void;
}) {
  const { t } = useTranslation();
  const missingTitle = !draft.title.trim();
  const missingProject = !draft.projectId;
  const disabled = missingTitle || missingProject || submitting;
  let reason: string | null = null;
  if (missingProject) reason = t("office:selectAProjectToCreateATask");
  else if (missingTitle) reason = t("office:addATitleToCreateATask");

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
