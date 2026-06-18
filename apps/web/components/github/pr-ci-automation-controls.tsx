"use client";

import { useCallback, useState } from "react";
import { IconEdit, IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { useTaskCIAutomationOptions } from "@/hooks/domains/github/use-task-ci-options";
import type { TaskCIAutomationPatch, TaskPR } from "@/lib/types/github";

function CIAutomationInfoButton() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6 cursor-help text-muted-foreground hover:text-foreground"
          aria-label="Explain CI automation options"
        >
          <IconInfoCircle className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="top" align="end" className="max-w-[280px] text-xs leading-relaxed">
        Watches this task's linked pull request during the 1 minute PR refresh loop. Auto-fix queues
        a task prompt for new failed checks and unresolved review comments, then snapshots what was
        handled so the next round only sends newly observed issues. Auto-merge runs only after CI,
        review, and mergeability are ready.
      </TooltipContent>
    </Tooltip>
  );
}

function CIAutomationPromptDialog({
  open,
  prompt,
  saving,
  onPromptChange,
  onClose,
  onSave,
  onReset,
}: {
  open: boolean;
  prompt: string;
  saving: boolean;
  onPromptChange: (value: string) => void;
  onClose: () => void;
  onSave: () => void;
  onReset: () => void;
}) {
  const trimmed = prompt.trim();
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Auto-fix prompt</DialogTitle>
          <DialogDescription>
            This prompt is used only for this task. Leave it blank to use the default prompt.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          <div className="flex items-center justify-between gap-2">
            <Label htmlFor="task-ci-auto-fix-prompt" className="text-xs">
              Task auto-fix prompt
            </Label>
            <a
              href="/settings/prompts"
              className="cursor-pointer text-xs text-primary hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              Edit default prompt
            </a>
          </div>
          <Textarea
            id="task-ci-auto-fix-prompt"
            value={prompt}
            onChange={(event) => onPromptChange(event.target.value)}
            rows={10}
            className="max-h-[50vh] min-h-48 resize-y font-mono text-xs"
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" className="cursor-pointer" disabled={saving} onClick={onClose}>
            Cancel
          </Button>
          <Button variant="outline" className="cursor-pointer" disabled={saving} onClick={onReset}>
            Use default
          </Button>
          <Button
            className="cursor-pointer"
            disabled={saving || trimmed.length === 0}
            onClick={onSave}
          >
            Save prompt
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CIAutomationRow({
  id,
  label,
  checked,
  disabled,
  onCheckedChange,
}: {
  id: string;
  label: string;
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex min-h-7 items-center justify-between gap-3 px-1">
      <Label htmlFor={id} className="min-w-0 flex-1 cursor-pointer truncate text-xs">
        {label}
      </Label>
      <Switch
        id={id}
        aria-label={label}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

export function PRCIAutomationControls({ pr }: { pr: TaskPR }) {
  const { options, loading, saving, error, update, resetPrompt } = useTaskCIAutomationOptions(
    pr.task_id,
  );
  const { toast } = useToast();
  const [promptOpen, setPromptOpen] = useState(false);
  const [promptDraft, setPromptDraft] = useState("");

  const openPromptEditor = useCallback(() => {
    setPromptDraft(options?.auto_fix_prompt_override ?? options?.effective_auto_fix_prompt ?? "");
    setPromptOpen(true);
  }, [options]);

  const reportError = useCallback(
    (description: string) => {
      toast({ description, variant: "error" });
    },
    [toast],
  );

  const patchOption = useCallback(
    (patch: TaskCIAutomationPatch) => {
      Promise.resolve(update(patch)).catch(() => reportError("Failed to update CI automation."));
    },
    [reportError, update],
  );

  const savePrompt = useCallback(() => {
    const value = promptDraft.trim();
    if (!value) return;
    Promise.resolve(update({ auto_fix_prompt_override: value }))
      .then(() => setPromptOpen(false))
      .catch(() => reportError("Failed to save auto-fix prompt."));
  }, [promptDraft, reportError, update]);

  const useDefaultPrompt = useCallback(() => {
    Promise.resolve(resetPrompt())
      .then(() => setPromptOpen(false))
      .catch(() => reportError("Failed to reset auto-fix prompt."));
  }, [reportError, resetPrompt]);

  const disabled = loading || saving || !options;
  return (
    <div
      data-testid="pr-ci-automation-controls"
      className="flex flex-col gap-1 border-t border-border/50 pt-2"
    >
      <div className="flex items-center justify-between gap-2 px-1">
        <div className="text-xs font-medium text-foreground">Automation</div>
        <div className="flex items-center gap-1">
          <CIAutomationInfoButton />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-6 w-6 cursor-pointer text-muted-foreground hover:text-foreground"
            aria-label="Edit auto-fix prompt for this task"
            disabled={!options}
            onClick={openPromptEditor}
          >
            <IconEdit className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      <CIAutomationRow
        id={`task-ci-auto-fix-${pr.task_id}`}
        label="Auto-fix CI and address comments"
        checked={Boolean(options?.auto_fix_enabled)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_fix_enabled: checked })}
      />
      <CIAutomationRow
        id={`task-ci-auto-merge-${pr.task_id}`}
        label="Auto-merge when ready"
        checked={Boolean(options?.auto_merge_enabled)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_merge_enabled: checked })}
      />
      {error && <div className="px-1 text-[11px] text-destructive">{error}</div>}
      <CIAutomationPromptDialog
        open={promptOpen}
        prompt={promptDraft}
        saving={saving}
        onPromptChange={setPromptDraft}
        onClose={() => setPromptOpen(false)}
        onSave={savePrompt}
        onReset={useDefaultPrompt}
      />
    </div>
  );
}
