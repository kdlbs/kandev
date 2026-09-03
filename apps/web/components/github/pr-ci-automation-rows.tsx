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
import {
  autoFixRoundForState,
  type CIAutomationQueueRecoveryState,
  deriveCIAutomationQueueState,
} from "@/lib/github/ci-automation";
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
        {t("github:ciAutomationQueueRecoveryHelp")}
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
  supportingText,
  describedBy,
}: {
  id: string;
  label: string;
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
  help?: ReactNode;
  supportingText?: ReactNode;
  describedBy?: string;
}) {
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const minHeight = isMobile || !isFinePointer ? "min-h-11" : "min-h-7";

  return (
    <div className={`flex items-center justify-between gap-3 px-1 ${minHeight}`}>
      <div className="flex min-w-0 flex-1 flex-col items-start">
        <div className="flex min-w-0 items-center gap-1.5">
          <Label htmlFor={id} className="min-w-0 cursor-pointer text-xs leading-5">
            {label}
          </Label>
          {help}
        </div>
        {supportingText ? (
          <span className="max-w-full break-words text-[10px] leading-4 text-muted-foreground">
            {supportingText}
          </span>
        ) : null}
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
  actionLabel,
  onAction,
}: {
  error: string;
  loading: boolean;
  actionLabel: string;
  onAction: () => void;
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
        className="h-11 min-w-11 cursor-pointer gap-1 px-2 text-[11px] sm:h-6 sm:min-w-0"
        disabled={loading}
        onClick={onAction}
      >
        <IconRefresh className={`h-3 w-3 ${loading ? "animate-spin" : ""}`} />
        {actionLabel}
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
  const terminalHelp = t("github:wakeAgentOnTerminalPrState");
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
  const help = t("github:wakeAgentOnReviewRequest");
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

type CIAutomationTranslate = (key: string, options?: Record<string, unknown>) => string;

function queueContextTitle(
  t: CIAutomationTranslate,
  context: CIAutomationQueueRecoveryState["context"],
): string {
  switch (context) {
    case "queued":
      return t("github:mergeQueueAutomation");
    case "recovery":
      return t("github:mergeQueueRecovery");
    default:
      return t("github:automation");
  }
}

function queueRemovalCauseLabel(
  t: CIAutomationTranslate,
  cause: CIAutomationQueueRecoveryState["removalCause"],
): string {
  switch (cause) {
    case "checks_failed":
      return t("github:mergeQueueRemovalCauseChecksFailed");
    case "checks_timed_out":
      return t("github:mergeQueueRemovalCauseChecksTimedOut");
    case "conflict":
      return t("github:mergeQueueRemovalCauseConflict");
    default:
      return "";
  }
}

function autoFixQueueSupport(
  t: CIAutomationTranslate,
  queueState: CIAutomationQueueRecoveryState,
  enabled: boolean,
): string | undefined {
  switch (queueState.status) {
    case "queued":
      return t("github:autoFixQueueSupportQueued");
    case "removed_actionable":
      return enabled ? undefined : t("github:autoFixQueueSupportEnable");
    case "repair_requested":
      return t("github:autoFixQueueSupportRequested");
    case "removed_not_actionable":
      return t("github:autoFixQueueSupportNotActionable");
    case "waiting_for_checks":
      return t("github:autoFixQueueSupportNewHead");
    default:
      return undefined;
  }
}

function autoMergeQueueSupport(
  t: CIAutomationTranslate,
  queueState: CIAutomationQueueRecoveryState,
  enabled: boolean,
): string | undefined {
  switch (queueState.status) {
    case "queued":
      return t("github:autoMergeQueueSupportQueued");
    case "repair_requested":
    case "waiting_for_commit":
      return t("github:autoMergeQueueSupportWaitingForCommit");
    case "waiting_for_checks":
      return t("github:autoMergeQueueSupportWaitingForChecks");
    default:
      if (queueState.context !== "recovery") return undefined;
      return enabled
        ? t("github:autoMergeQueueSupportNewHead")
        : t("github:autoMergeQueueSupportDisabled");
  }
}

export function CIAutomationHeader({
  pr,
  queueState,
  disabled,
  onEditPrompt,
}: {
  pr: TaskPR;
  queueState: CIAutomationQueueRecoveryState;
  disabled: boolean;
  onEditPrompt: () => void;
}) {
  const { t } = useTranslation();
  const title = queueContextTitle(t, queueState.context);
  let subtitle = t("github:automationForPr", { number: pr.pr_number });
  if (queueState.context === "queued") {
    subtitle = t("github:automationForPrQueued", { number: pr.pr_number });
  } else if (queueState.context === "recovery") {
    const cause = queueRemovalCauseLabel(t, queueState.removalCause);
    if (cause) {
      subtitle = t("github:automationForPrRemoved", { number: pr.pr_number, cause });
    } else {
      subtitle = t("github:automationForPrRemovedGeneric", { number: pr.pr_number });
    }
  }
  return (
    <div className="flex items-center justify-between gap-2 px-1">
      <div className="flex min-w-0 flex-col">
        <div className="text-xs font-medium text-foreground">{title}</div>
        <div className="truncate text-[11px] text-muted-foreground">{subtitle}</div>
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

export function CIAutomationQueueStatusRow({
  queueState,
}: {
  queueState: CIAutomationQueueRecoveryState;
}) {
  const { t } = useTranslation();
  if (queueState.status === "none") return null;
  let message: string;
  switch (queueState.status) {
    case "queued":
      message = t("github:mergeQueueRecoveryStatusQueued");
      break;
    case "removed_actionable":
      message = t("github:mergeQueueRecoveryStatusActionable");
      break;
    case "removed_not_actionable":
      message = t("github:mergeQueueRecoveryStatusNotActionable");
      break;
    case "repair_requested":
      if (queueState.waitingForCommit) {
        message = t("github:mergeQueueRecoveryStatusRepairRequestedWaiting");
      } else {
        message = t("github:mergeQueueRecoveryStatusRepairRequested");
      }
      break;
    case "waiting_for_commit":
      message = t("github:mergeQueueRecoveryStatusWaitingForCommit");
      break;
    case "waiting_for_checks":
      message = t("github:mergeQueueRecoveryStatusWaitingForChecks");
      break;
  }
  return (
    <div
      data-testid="ci-merge-queue-recovery-status"
      role="status"
      className="break-words px-1 text-[11px] leading-4 text-muted-foreground"
    >
      {message}
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
  const queueState = deriveCIAutomationQueueState(pr, prOptions, automationState);
  const autoFixSupportingText = autoFixQueueSupport(t, queueState, prOptions.auto_fix_enabled);
  const autoMergeSupportingText = autoMergeQueueSupport(
    t,
    queueState,
    prOptions.auto_merge_enabled,
  );
  return (
    <>
      <CIAutomationRow
        id={`task-ci-auto-fix-${scopeKey}`}
        label={t("github:autoFixCiAndAddressComments")}
        checked={prOptions.auto_fix_enabled}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_fix_enabled: checked })}
        supportingText={autoFixSupportingText}
        help={
          prOptions.auto_fix_enabled ? (
            <CIAutoFixRoundHelpButton state={automationState} maxRounds={autoFixMaxRounds} />
          ) : null
        }
      />
      <CIAutomationRow
        id={`task-ci-auto-merge-${scopeKey}`}
        label={t("github:autoMergeOrRequeueWhenReady")}
        checked={prOptions.auto_merge_enabled}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_merge_enabled: checked })}
        supportingText={autoMergeSupportingText}
      />
      <CIAutomationQueueStatusRow queueState={queueState} />
      <ReviewFollowUpSection
        scopeKey={scopeKey}
        prOptions={prOptions}
        disabled={disabled}
        patchOption={patchOption}
      />
    </>
  );
}
