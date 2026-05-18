"use client";

import { useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Textarea } from "@kandev/ui/textarea";
import { Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle } from "@kandev/ui/dialog";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { createTask } from "@/lib/api/domains/kanban-api";
import type { OfficeMeta } from "@/lib/state/slices/office/types";
import { useIssueDraft, type IssueDraft } from "./new-task-draft";
import { NewTaskSelectorRow } from "./new-task-selector-row";
import { NewTaskBottomBar } from "./new-task-bottom-bar";
import {
  NewTaskStages,
  buildExecutionPolicy,
  EMPTY_STAGES,
  type StagesDraft,
} from "./new-task-stages";

const FALLBACK_PRIORITY_VALUES: Record<string, number> = {
  critical: 4,
  high: 3,
  medium: 2,
  low: 1,
  none: 0,
};

function priorityToNumber(priority: string, meta: OfficeMeta | null): number {
  if (meta) {
    const p = meta.priorities.find((m) => m.id === priority);
    if (p) return p.value;
  }
  return FALLBACK_PRIORITY_VALUES[priority] ?? 0;
}

function buildMetadata(draft: IssueDraft): Record<string, unknown> | undefined {
  const meta: Record<string, unknown> = {};
  if (draft.assigneeId) meta.assignee_agent_profile_id = draft.assigneeId;
  if (draft.projectId) meta.project_id = draft.projectId;
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
  const meta = useAppStore((s) => s.office.meta);
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
    if (!draft.title.trim() || !workspaceId) return;
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
        priority: priorityToNumber(draft.priority, meta),
        metadata: executionPolicy ? { ...metadata, execution_policy: executionPolicy } : metadata,
      });
      clearDraft();
      setStages(EMPTY_STAGES);
      onOpenChange(false);
      toast.success("Task created");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create issue");
    } finally {
      setSubmitting(false);
    }
  }, [draft, stages, workspaceId, parentTaskId, meta, clearDraft, onOpenChange]);

  const handleDiscard = useCallback(() => {
    clearDraft();
    onOpenChange(false);
  }, [clearDraft, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl sm:max-w-3xl lg:max-w-4xl">
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
  return (
    <>
      <DialogHeader>
        <DialogTitle className="sr-only">New issue</DialogTitle>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="font-mono text-xs">
            KAN
          </Badge>
          <span className="text-sm text-muted-foreground">New issue</span>
          {parentTaskId && (
            <Badge variant="secondary" className="text-xs">
              Sub-issue of {parentTaskId}
            </Badge>
          )}
        </div>
      </DialogHeader>

      <div className="space-y-4">
        <Textarea
          placeholder="Task title"
          value={draft.title}
          onChange={(e) => updateDraft({ title: e.target.value })}
          className="text-lg font-medium border-0 resize-none p-0 focus-visible:ring-0 min-h-[40px]"
          rows={1}
          autoFocus
        />
        <NewTaskSelectorRow draft={draft} onUpdate={updateDraft} />
        <Textarea
          placeholder="Add description..."
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
          Discard Draft
        </Button>
        <Button
          onClick={onCreate}
          disabled={!draft.title.trim() || submitting}
          className="cursor-pointer"
        >
          {submitting ? "Creating..." : "Create Task"}
        </Button>
      </DialogFooter>
    </>
  );
}
