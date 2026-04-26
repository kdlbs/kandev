"use client";

import { useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Textarea } from "@kandev/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
} from "@kandev/ui/dialog";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { createTask } from "@/lib/api/domains/kanban-api";
import { useIssueDraft } from "./new-issue-draft";
import { NewIssueSelectorRow } from "./new-issue-selector-row";
import { NewIssueBottomBar } from "./new-issue-bottom-bar";

type NewIssueDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  parentTaskId?: string;
  defaultProjectId?: string;
  defaultAssigneeId?: string;
};

export function NewIssueDialog({
  open,
  onOpenChange,
  parentTaskId,
  defaultProjectId,
  defaultAssigneeId,
}: NewIssueDialogProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [submitting, setSubmitting] = useState(false);

  const { draft, updateDraft, clearDraft } = useIssueDraft(
    workspaceId,
    parentTaskId,
    { projectId: defaultProjectId, assigneeId: defaultAssigneeId },
  );

  const handleCreate = useCallback(async () => {
    if (!draft.title.trim() || !workspaceId) return;
    setSubmitting(true);
    try {
      // TODO: Add priority, assignee, project fields when Wave 3A backend lands
      await createTask({
        workspace_id: workspaceId,
        workflow_id: "",
        title: draft.title.trim(),
        description: draft.description.trim() || undefined,
        parent_id: parentTaskId,
      });
      clearDraft();
      onOpenChange(false);
      toast.success("Issue created");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create issue");
    } finally {
      setSubmitting(false);
    }
  }, [draft, workspaceId, parentTaskId, clearDraft, onOpenChange]);

  const handleDiscard = useCallback(() => {
    clearDraft();
    onOpenChange(false);
  }, [clearDraft, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
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
          {/* Title */}
          <Textarea
            placeholder="Issue title"
            value={draft.title}
            onChange={(e) => updateDraft({ title: e.target.value })}
            className="text-lg font-medium border-0 resize-none p-0 focus-visible:ring-0 min-h-[40px]"
            rows={1}
            autoFocus
          />

          {/* Quick selector row */}
          <NewIssueSelectorRow draft={draft} onUpdate={updateDraft} />

          {/* Description */}
          <Textarea
            placeholder="Add description..."
            value={draft.description}
            onChange={(e) => updateDraft({ description: e.target.value })}
            className="min-h-[120px] text-sm"
          />

          {/* Bottom bar */}
          <NewIssueBottomBar draft={draft} onUpdate={updateDraft} />
        </div>

        <DialogFooter className="flex justify-between sm:justify-between">
          <Button
            variant="ghost"
            className="text-muted-foreground cursor-pointer"
            onClick={handleDiscard}
          >
            Discard Draft
          </Button>
          <Button
            onClick={handleCreate}
            disabled={!draft.title.trim() || submitting}
            className="cursor-pointer"
          >
            {submitting ? "Creating..." : "Create Issue"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
