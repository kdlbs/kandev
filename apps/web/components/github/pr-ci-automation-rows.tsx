"use client";

import { useEffect, useState, type ReactNode } from "react";
import { IconChevronDown, IconEdit, IconInfoCircle, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Label } from "@kandev/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { autoFixRoundForState } from "@/lib/github/ci-automation";
import type {
  TaskCIAutomationPatch,
  TaskCIPRAutomationState,
  TaskPR,
  TaskPRAutomationOptions,
} from "@/lib/types/github";
import { useTranslation } from "react-i18next";

/**
 * Element-id scope unique per (task, repository, PR) so simultaneously
 * mounted PR tabs in MultiPRCIPopover never share a Switch id/htmlFor pair.
 */
export function prAutomationScopeKey(pr: TaskPR): string {
  return `${pr.task_id}-${pr.repository_id ?? "none"}-${pr.pr_number}`;
}

export function CIAutomationInfoButton() {
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

export function CIAutomationRow({
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

export function CIAutomationErrorRow({
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

export function CIAutomationHelpButton({
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
  scopeKey,
  prOptions,
  disabled,
  patchOption,
}: {
  scopeKey: string;
  prOptions: TaskPRAutomationOptions;
  disabled: boolean;
  patchOption: (patch: TaskCIAutomationPatch) => void;
}) {
  const { t } = useTranslation();
  const terminalHelpID = `task-pr-terminal-help-${scopeKey}`;
  const terminalHelp = "Wake the agent when review work ends. Choose either or both outcomes.";
  return (
    <>
      <ReviewRequestedPromptRow
        scopeKey={scopeKey}
        prOptions={prOptions}
        disabled={disabled}
        patchOption={patchOption}
      />
      <span id={terminalHelpID} className="sr-only">
        {terminalHelp}
      </span>
      <CIAutomationRow
        id={`task-pr-merged-prompt-${scopeKey}`}
        label={t("github:prMerged")}
        describedBy={terminalHelpID}
        checked={prOptions.prompt_on_merged}
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
        id={`task-pr-closed-prompt-${scopeKey}`}
        label={t("github:prClosedWithoutMerging")}
        describedBy={terminalHelpID}
        checked={prOptions.prompt_on_closed}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_closed: checked })}
      />
    </>
  );
}

export function ReviewFollowUpSection({
  scopeKey,
  prOptions,
  disabled,
  patchOption,
}: {
  scopeKey: string;
  prOptions: TaskPRAutomationOptions;
  disabled: boolean;
  patchOption: (patch: TaskCIAutomationPatch) => void;
}) {
  const { t } = useTranslation();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const lifecycleEnabled =
    prOptions.prompt_on_review_requested ||
    prOptions.prompt_on_merged ||
    prOptions.prompt_on_closed;
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
          aria-label={t("github:toggleReviewFollowUpAutomation")}
          className={`w-full cursor-pointer justify-between px-1 text-xs text-muted-foreground ${minHeight}`}
        >
          {t("github:reviewFollowUp")}
          <IconChevronDown
            aria-hidden="true"
            className={`h-3.5 w-3.5 transition-transform ${open ? "rotate-180" : ""}`}
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-1">
        <PRAgentPromptRows
          scopeKey={scopeKey}
          prOptions={prOptions}
          disabled={disabled}
          patchOption={patchOption}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

function ReviewRequestedPromptRow({
  scopeKey,
  prOptions,
  disabled,
  patchOption,
}: {
  scopeKey: string;
  prOptions: TaskPRAutomationOptions;
  disabled: boolean;
  patchOption: (patch: TaskCIAutomationPatch) => void;
}) {
  const { t } = useTranslation();
  const helpID = `task-pr-review-requested-prompt-${scopeKey}-description`;
  const help = "Wake the agent for any new request, including re-review after changes.";
  return (
    <>
      <span id={helpID} className="sr-only">
        {help}
      </span>
      <CIAutomationRow
        id={`task-pr-review-requested-prompt-${scopeKey}`}
        label={t("github:yourReviewIsRequested")}
        describedBy={helpID}
        checked={prOptions.prompt_on_review_requested}
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

export function CIAutomationHeader({
  pr,
  disabled,
  onEditPrompt,
}: {
  pr: TaskPR;
  disabled: boolean;
  onEditPrompt: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 px-1">
      <div className="flex min-w-0 flex-col">
        <div className="text-xs font-medium text-foreground">{t("github:automation")}</div>
        <div className="truncate text-[11px] text-muted-foreground">
          {t("github:automationForPr", { number: pr.pr_number })}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1">
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

export function CIAutomationOptionRows({
  pr,
  prOptions,
  autoFixMaxRounds,
  disabled,
  patchOption,
  automationState,
}: {
  pr: TaskPR;
  prOptions: TaskPRAutomationOptions;
  autoFixMaxRounds: number | null | undefined;
  disabled: boolean;
  patchOption: (patch: TaskCIAutomationPatch) => void;
  automationState: TaskCIPRAutomationState | undefined;
}) {
  const { t } = useTranslation();
  const scopeKey = prAutomationScopeKey(pr);
  return (
    <>
      <CIAutomationRow
        id={`task-ci-auto-fix-${scopeKey}`}
        label={t("github:autoFixCiAndAddressComments")}
        checked={prOptions.auto_fix_enabled}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_fix_enabled: checked })}
        help={
          prOptions.auto_fix_enabled ? (
            <CIAutoFixRoundHelpButton state={automationState} maxRounds={autoFixMaxRounds} />
          ) : null
        }
      />
      <CIAutomationRow
        id={`task-ci-auto-merge-${scopeKey}`}
        label={t("github:autoMergeWhenReady")}
        checked={prOptions.auto_merge_enabled}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_merge_enabled: checked })}
      />
      <ReviewFollowUpSection
        scopeKey={scopeKey}
        prOptions={prOptions}
        disabled={disabled}
        patchOption={patchOption}
      />
    </>
  );
}
