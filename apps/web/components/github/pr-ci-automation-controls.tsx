"use client";

import { useCallback, useEffect, useState, type ReactNode } from "react";
import { IconChevronDown, IconEdit, IconInfoCircle, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Label } from "@kandev/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Switch } from "@kandev/ui/switch";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { useTaskCIAutomationOptions } from "@/hooks/domains/github/use-task-ci-options";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { autoFixRoundForState, findCIAutomationStateForPR } from "@/lib/github/ci-automation";
import type {
  TaskCIAutomationOptions,
  TaskCIAutomationPatch,
  TaskCIPRAutomationState,
  TaskPR,
} from "@/lib/types/github";

const PR_FEEDBACK_PLACEHOLDER = "{{pr.feedback}}";

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
        task prompts for newly observed feedback, auto-merge waits for the PR to be ready, and the
        notification switches wake the task's agent when the workspace's connected GitHub account is
        requested for review or the PR reaches a terminal state. Review requests silently baseline
        the account's current request state before notifying on a later request.
      </TooltipContent>
    </Tooltip>
  );
}

function insertPRFeedbackPlaceholder(prompt: string) {
  if (prompt.includes(PR_FEEDBACK_PLACEHOLDER)) return prompt;
  const trimmedEnd = prompt.trimEnd();
  if (!trimmedEnd) return PR_FEEDBACK_PLACEHOLDER;
  return `${trimmedEnd}\n\n${PR_FEEDBACK_PLACEHOLDER}`;
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
            This prompt is used only for this task. Leave it blank to use the default prompt. Add{" "}
            <code
              data-testid="ci-auto-fix-pr-feedback-placeholder"
              className="rounded bg-muted px-1 py-0.5 text-[11px]"
            >
              {PR_FEEDBACK_PLACEHOLDER}
            </code>{" "}
            when you want Kandev to include its PR feedback snapshot.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Label htmlFor="task-ci-auto-fix-prompt" className="text-xs">
              Task auto-fix prompt
            </Label>
            <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 cursor-pointer px-2 text-xs"
                onClick={() => onPromptChange(insertPRFeedbackPlaceholder(prompt))}
              >
                Insert PR feedback
              </Button>
              <a
                href="/settings/prompts"
                className="cursor-pointer text-xs text-primary hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                Edit default prompt
              </a>
            </div>
          </div>
          <div
            data-testid="ci-auto-fix-pr-feedback-help"
            className="rounded-md border border-border/70 bg-muted/30 p-3 text-xs leading-relaxed text-muted-foreground"
          >
            <p>
              The placeholder inserts the current PR identifier, new or changed failing checks with
              GitHub job links, and new or changed review comments with file, line, and body text.
            </p>
            <p className="mt-2">
              Omit the placeholder if you want the agent to pull or fetch the branch and inspect
              GitHub itself instead of receiving Kandev's snapshot.
            </p>
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
  help,
  describedBy,
}: {
  id: string;
  label: string;
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
  help?: ReactNode;
  describedBy?: string;
}) {
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const minHeight = isMobile || !isFinePointer ? "min-h-11" : "min-h-7";

  return (
    <div className={`flex items-center justify-between gap-3 px-1 ${minHeight}`}>
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        <Label htmlFor={id} className="min-w-0 cursor-pointer text-xs leading-5">
          {label}
        </Label>
        {help}
      </div>
      <Switch
        id={id}
        aria-label={label}
        aria-describedby={describedBy}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

function CIAutomationErrorRow({
  error,
  loading,
  onRetry,
}: {
  error: string;
  loading: boolean;
  onRetry: () => void;
}) {
  return (
    <div
      role="alert"
      className="flex items-center justify-between gap-2 px-1 text-[11px] text-destructive"
    >
      <span className="min-w-0 flex-1 truncate">{error}</span>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-6 cursor-pointer gap-1 px-2 text-[11px]"
        disabled={loading}
        onClick={onRetry}
      >
        <IconRefresh className={`h-3 w-3 ${loading ? "animate-spin" : ""}`} />
        Retry
      </Button>
    </div>
  );
}

function CIAutomationHelpButton({
  ariaLabel,
  testId,
  children,
}: {
  ariaLabel: string;
  testId: string;
  children: ReactNode;
}) {
  const { isFinePointer } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      data-testid={testId}
      className="h-5 w-5 cursor-help text-muted-foreground hover:text-foreground"
      aria-label={ariaLabel}
    >
      <IconInfoCircle className="h-3.5 w-3.5" />
    </Button>
  );
  if (!isFinePointer) {
    return (
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>{trigger}</PopoverTrigger>
        <PopoverContent
          side="top"
          align="start"
          portal={false}
          className="max-w-[280px] text-xs leading-relaxed"
        >
          {children}
        </PopoverContent>
      </Popover>
    );
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent side="top" align="start" className="max-w-[280px] text-xs leading-relaxed">
        {children}
      </TooltipContent>
    </Tooltip>
  );
}

function CIAutoFixRoundHelpButton({
  state,
  maxRounds,
}: {
  state: TaskCIPRAutomationState | undefined;
  maxRounds: number | null | undefined;
}) {
  const round = autoFixRoundForState(state, maxRounds);
  return (
    <CIAutomationHelpButton testId="ci-auto-fix-round-help" ariaLabel="Explain auto-fix rounds">
      <span data-testid="ci-auto-fix-round-explanation">
        Auto-fix has used {round.current} of {round.max} rounds for this PR. A round is counted when
        Kandev sends or queues a CI auto-fix message. Kandev waits for all PR checks to finish
        before starting a new CI auto-fix turn, so the agent gets the final failed checks and
        current comments together. Updating an already queued auto-fix message does not use another
        round. When this is at {round.max}/{round.max} and there is no pending auto-fix message left
        to update, Kandev pauses auto-fix for this PR so it cannot loop forever. Disable and
        re-enable auto-fix after manual review to start over.
      </span>
    </CIAutomationHelpButton>
  );
}

function PRAgentPromptRows({
  taskId,
  options,
  disabled,
  patchOption,
}: {
  taskId: string;
  options: TaskCIAutomationOptions | null;
  disabled: boolean;
  patchOption: (patch: TaskCIAutomationPatch) => void;
}) {
  const terminalHelpID = `task-pr-terminal-help-${taskId}`;
  const terminalHelp = "Wake the agent when review work ends. Choose either or both outcomes.";
  return (
    <>
      <ReviewRequestedPromptRow
        taskId={taskId}
        options={options}
        disabled={disabled}
        patchOption={patchOption}
      />
      <span id={terminalHelpID} className="sr-only">
        {terminalHelp}
      </span>
      <CIAutomationRow
        id={`task-pr-merged-prompt-${taskId}`}
        label="PR merged"
        describedBy={terminalHelpID}
        checked={Boolean(options?.prompt_on_merged)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_merged: checked })}
        help={
          <CIAutomationHelpButton
            testId="ci-pr-terminal-help"
            ariaLabel="Explain final PR state notifications"
          >
            {terminalHelp}
          </CIAutomationHelpButton>
        }
      />
      <CIAutomationRow
        id={`task-pr-closed-prompt-${taskId}`}
        label="PR closed without merging"
        describedBy={terminalHelpID}
        checked={Boolean(options?.prompt_on_closed)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_closed: checked })}
      />
    </>
  );
}

function ReviewFollowUpSection({
  taskId,
  options,
  disabled,
  patchOption,
}: {
  taskId: string;
  options: TaskCIAutomationOptions | null;
  disabled: boolean;
  patchOption: (patch: TaskCIAutomationPatch) => void;
}) {
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const lifecycleEnabled = Boolean(
    options?.prompt_on_review_requested || options?.prompt_on_merged || options?.prompt_on_closed,
  );
  const minHeight = isMobile || !isFinePointer ? "min-h-11" : "min-h-7";

  useEffect(() => {
    if (lifecycleEnabled) setOpen(true);
  }, [lifecycleEnabled]);

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          data-testid="ci-review-follow-up-trigger"
          aria-label="Toggle review follow-up automation"
          className={`w-full cursor-pointer justify-between px-1 text-xs text-muted-foreground ${minHeight}`}
        >
          Review follow-up
          <IconChevronDown
            aria-hidden="true"
            className={`h-3.5 w-3.5 transition-transform ${open ? "rotate-180" : ""}`}
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-1">
        <PRAgentPromptRows
          taskId={taskId}
          options={options}
          disabled={disabled}
          patchOption={patchOption}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

function ReviewRequestedPromptRow({
  taskId,
  options,
  disabled,
  patchOption,
}: {
  taskId: string;
  options: TaskCIAutomationOptions | null;
  disabled: boolean;
  patchOption: (patch: TaskCIAutomationPatch) => void;
}) {
  const helpID = `task-pr-review-requested-prompt-${taskId}-description`;
  const help = "Wake the agent for any new request, including re-review after changes.";
  return (
    <>
      <span id={helpID} className="sr-only">
        {help}
      </span>
      <CIAutomationRow
        id={`task-pr-review-requested-prompt-${taskId}`}
        label="Your review is requested"
        describedBy={helpID}
        checked={Boolean(options?.prompt_on_review_requested)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_review_requested: checked })}
        help={
          <CIAutomationHelpButton
            testId="ci-review-requested-help"
            ariaLabel="Explain review request notifications"
          >
            {help}
          </CIAutomationHelpButton>
        }
      />
    </>
  );
}

function CIAutomationHeader({
  disabled,
  onEditPrompt,
}: {
  disabled: boolean;
  onEditPrompt: () => void;
}) {
  return (
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
          disabled={disabled}
          onClick={onEditPrompt}
        >
          <IconEdit className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

function CIAutomationOptionRows({
  pr,
  options,
  disabled,
  patchOption,
  automationState,
}: {
  pr: TaskPR;
  options: TaskCIAutomationOptions | null;
  disabled: boolean;
  patchOption: (patch: TaskCIAutomationPatch) => void;
  automationState: TaskCIPRAutomationState | undefined;
}) {
  return (
    <>
      <CIAutomationRow
        id={`task-ci-auto-fix-${pr.task_id}`}
        label="Auto-fix CI and address comments"
        checked={Boolean(options?.auto_fix_enabled)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_fix_enabled: checked })}
        help={
          options?.auto_fix_enabled ? (
            <CIAutoFixRoundHelpButton
              state={automationState}
              maxRounds={options.auto_fix_max_rounds}
            />
          ) : null
        }
      />
      <CIAutomationRow
        id={`task-ci-auto-merge-${pr.task_id}`}
        label="Auto-merge when ready"
        checked={Boolean(options?.auto_merge_enabled)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_merge_enabled: checked })}
      />
      <ReviewFollowUpSection
        taskId={pr.task_id}
        options={options}
        disabled={disabled}
        patchOption={patchOption}
      />
    </>
  );
}

export function PRCIAutomationControls({ pr }: { pr: TaskPR }) {
  const { options, loading, saving, error, refresh, update, resetPrompt } =
    useTaskCIAutomationOptions(pr.task_id);
  const { toast } = useToast();
  const [promptOpen, setPromptOpen] = useState(false);
  const [promptDraft, setPromptDraft] = useState("");
  const automationState = findCIAutomationStateForPR(options?.pr_states, pr);

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

  const retryLoad = useCallback(() => {
    Promise.resolve(refresh()).catch(() => reportError("Failed to load CI automation."));
  }, [refresh, reportError]);

  const disabled = loading || saving || !options;
  return (
    <div
      data-testid="pr-ci-automation-controls"
      className="flex flex-col gap-1 border-t border-border/50 pt-2"
    >
      <CIAutomationHeader disabled={!options} onEditPrompt={openPromptEditor} />
      <CIAutomationOptionRows
        pr={pr}
        options={options}
        disabled={disabled}
        patchOption={patchOption}
        automationState={automationState}
      />
      {automationState?.last_error && (
        <CIAutomationErrorRow
          error={automationState.last_error}
          loading={loading}
          onRetry={retryLoad}
        />
      )}
      {error && <CIAutomationErrorRow error={error} loading={loading} onRetry={retryLoad} />}
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
