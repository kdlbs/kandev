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
import { Trans, useTranslation } from "react-i18next";

const PR_FEEDBACK_PLACEHOLDER = "{{pr.feedback}}";

function CIAutomationInfoButton() {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6 cursor-help text-muted-foreground hover:text-foreground"
          aria-label={t("github:explainCiAutomationOptions")}
        >
          <IconInfoCircle className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="top" align="end" className="max-w-[280px] text-xs leading-relaxed">
        {t("github:watchesThisTaskSLinkedPull")}
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
  const { t } = useTranslation();
  const trimmed = prompt.trim();
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("github:autoFixPrompt")}</DialogTitle>
          <DialogDescription>
            <Trans
              i18nKey="github:autoFixPromptDescription"
              values={{ placeholder: PR_FEEDBACK_PLACEHOLDER }}
            >
              <code
                data-testid="ci-auto-fix-pr-feedback-placeholder"
                className="rounded bg-muted px-1 py-0.5 text-[11px]"
              />
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Label htmlFor="task-ci-auto-fix-prompt" className="text-xs">
              {t("github:taskAutoFixPrompt")}
            </Label>
            <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 cursor-pointer px-2 text-xs"
                onClick={() => onPromptChange(insertPRFeedbackPlaceholder(prompt))}
              >
                {t("github:insertPrFeedback")}
              </Button>
              <a
                href="/settings/prompts"
                className="cursor-pointer text-xs text-primary hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {t("github:editDefaultPrompt")}
              </a>
            </div>
          </div>
          <div
            data-testid="ci-auto-fix-pr-feedback-help"
            className="rounded-md border border-border/70 bg-muted/30 p-3 text-xs leading-relaxed text-muted-foreground"
          >
            <p>{t("github:thePlaceholderInsertsTheCurrentPr")}</p>
            <p className="mt-2">{t("github:omitThePlaceholderIfYouWant")}</p>
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
            {t("common:cancel")}
          </Button>
          <Button variant="outline" className="cursor-pointer" disabled={saving} onClick={onReset}>
            {t("github:useDefault")}
          </Button>
          <Button
            className="cursor-pointer"
            disabled={saving || trimmed.length === 0}
            onClick={onSave}
          >
            {t("github:savePrompt")}
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
  const { t } = useTranslation();
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
        {t("github:retry")}
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
  const { t } = useTranslation();
  const round = autoFixRoundForState(state, maxRounds);
  return (
    <CIAutomationHelpButton
      testId="ci-auto-fix-round-help"
      ariaLabel={t("github:explainAutoFixRounds")}
    >
      <span data-testid="ci-auto-fix-round-explanation">
        {t("github:autoFixRoundExplanation", { current: round.current, max: round.max })}
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
  const { t } = useTranslation();
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
        label={t("github:prMerged")}
        describedBy={terminalHelpID}
        checked={Boolean(options?.prompt_on_merged)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_merged: checked })}
        help={
          <CIAutomationHelpButton
            testId="ci-pr-terminal-help"
            ariaLabel={t("github:explainFinalPrStateNotifications")}
          >
            {terminalHelp}
          </CIAutomationHelpButton>
        }
      />
      <CIAutomationRow
        id={`task-pr-closed-prompt-${taskId}`}
        label={t("github:prClosedWithoutMerging")}
        describedBy={terminalHelpID}
        checked={Boolean(options?.prompt_on_closed)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_closed: checked })}
      />
    </>
  );
}

function PREventsSection({
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
  const { t } = useTranslation();
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
          data-testid="ci-pr-events-trigger"
          data-legacy-testid="ci-review-follow-up-trigger"
          aria-label={t("github:togglePrEventPromptAutomation")}
          className={`w-full cursor-pointer justify-between px-1 text-xs text-muted-foreground ${minHeight}`}
        >
          {t("github:prEvents")}
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
  const { t } = useTranslation();
  const helpID = `task-pr-review-requested-prompt-${taskId}-description`;
  const help = "Wake the agent for any new request, including re-review after changes.";
  return (
    <>
      <span id={helpID} className="sr-only">
        {help}
      </span>
      <CIAutomationRow
        id={`task-pr-review-requested-prompt-${taskId}`}
        label={t("github:yourReviewIsRequested")}
        describedBy={helpID}
        checked={Boolean(options?.prompt_on_review_requested)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_review_requested: checked })}
        help={
          <CIAutomationHelpButton
            testId="ci-review-requested-help"
            ariaLabel={t("github:explainReviewRequestNotifications")}
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
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 px-1">
      <div className="text-xs font-medium text-foreground">{t("github:automation")}</div>
      <div className="flex items-center gap-1">
        <CIAutomationInfoButton />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6 cursor-pointer text-muted-foreground hover:text-foreground"
          aria-label={t("github:editAutoFixPromptForThis")}
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
  const { t } = useTranslation();
  return (
    <>
      <CIAutomationRow
        id={`task-ci-auto-fix-${pr.task_id}`}
        label={t("github:autoFixCiAndAddressComments")}
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
        label={t("github:autoMergeWhenReady")}
        checked={Boolean(options?.auto_merge_enabled)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_merge_enabled: checked })}
      />
      <PREventsSection
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
