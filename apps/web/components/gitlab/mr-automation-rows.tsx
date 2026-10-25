"use client";

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { autoFixRoundForState } from "@/lib/gitlab/mr-automation";
import type {
  TaskMRAutomationOptionsForMR,
  TaskMRAutomationPatch,
  TaskMRLifecycleState,
} from "@/lib/types/gitlab";

/** Shared compact-row height: taller touch target on mobile/coarse pointers. */
export function compactRowMinHeight(isMobile: boolean, isFinePointer: boolean): string {
  return isMobile || !isFinePointer ? "min-h-11" : "min-h-7";
}

/**
 * Dual-mode help affordance: a tap popover on coarse pointers, a hover
 * tooltip on fine pointers. Mirrors CIAutomationHelpButton (GitHub).
 */
export function MRAutomationHelpButton({
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

function MRAutomationRow({
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
  const minHeight = compactRowMinHeight(isMobile, isFinePointer);

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

function MRAutoFixRoundHelpButton({
  state,
  maxRounds,
}: {
  state: TaskMRLifecycleState | undefined;
  maxRounds: number | null | undefined;
}) {
  const { t } = useTranslation();
  const round = autoFixRoundForState(state, maxRounds);
  return (
    <MRAutomationHelpButton
      testId="mr-auto-fix-round-help"
      ariaLabel={t("gitlab:mrAutomationExplainAutoFixRounds")}
    >
      <span data-testid="mr-auto-fix-round-explanation">
        {t("gitlab:mrAutoFixRoundExplanation", { current: round.current, max: round.max })}
      </span>
    </MRAutomationHelpButton>
  );
}

function MRAutoMergeHelpButton() {
  const { t } = useTranslation();
  return (
    <MRAutomationHelpButton
      testId="mr-auto-merge-help"
      ariaLabel={t("gitlab:mrAutomationExplainAutoMerge")}
    >
      <span data-testid="mr-auto-merge-explanation">{t("gitlab:mrAutoMergeReadyExplanation")}</span>
    </MRAutomationHelpButton>
  );
}

/**
 * Props shared by every switch-row group. `elementIDSuffix` scopes the DOM ids
 * to one merge request: the multi-MR dropdown renders one group per linked MR,
 * so a task-only suffix would emit duplicate ids and point every `<Label>` at
 * the first MR's switch.
 */
type MRAutomationRowsProps = {
  elementIDSuffix: string;
  switches: TaskMRAutomationOptionsForMR;
  disabled: boolean;
  patchOption: (patch: TaskMRAutomationPatch) => void;
};

export function MRAutomationOptionRows({
  elementIDSuffix,
  switches,
  disabled,
  patchOption,
  automationState,
  maxRounds,
}: MRAutomationRowsProps & {
  automationState: TaskMRLifecycleState | undefined;
  maxRounds: number | null | undefined;
}) {
  const { t } = useTranslation();
  return (
    <>
      <MRAutomationRow
        id={`task-mr-auto-fix-${elementIDSuffix}`}
        label={t("gitlab:mrAutoFixCiAndAddressComments")}
        checked={switches.auto_fix_enabled}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_fix_enabled: checked })}
        help={
          switches.auto_fix_enabled ? (
            <MRAutoFixRoundHelpButton state={automationState} maxRounds={maxRounds} />
          ) : null
        }
      />
      <MRAutomationRow
        id={`task-mr-auto-merge-${elementIDSuffix}`}
        label={t("gitlab:mrAutoMergeWhenReady")}
        checked={switches.auto_merge_enabled}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_merge_enabled: checked })}
        help={switches.auto_merge_enabled ? <MRAutoMergeHelpButton /> : null}
      />
    </>
  );
}

function ReviewRequestedPromptRow({
  elementIDSuffix,
  switches,
  disabled,
  patchOption,
}: MRAutomationRowsProps) {
  const { t } = useTranslation();
  const helpID = `task-mr-review-requested-prompt-${elementIDSuffix}-description`;
  const help = t("gitlab:mrAutomationReviewRequestedHelp");
  return (
    <>
      <span id={helpID} className="sr-only">
        {help}
      </span>
      <MRAutomationRow
        id={`task-mr-review-requested-prompt-${elementIDSuffix}`}
        label={t("gitlab:mrAutomationReviewRequestedLabel")}
        describedBy={helpID}
        checked={switches.prompt_on_review_requested}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_review_requested: checked })}
        help={
          <MRAutomationHelpButton
            testId="mr-review-requested-help"
            ariaLabel={t("gitlab:mrAutomationReviewRequestedHelpAriaLabel")}
          >
            {help}
          </MRAutomationHelpButton>
        }
      />
    </>
  );
}

export function MRAgentPromptRows({
  elementIDSuffix,
  switches,
  disabled,
  patchOption,
}: MRAutomationRowsProps) {
  const { t } = useTranslation();
  const terminalHelpID = `task-mr-terminal-help-${elementIDSuffix}`;
  const terminalHelp = t("gitlab:mrAutomationTerminalHelp");
  return (
    <>
      <ReviewRequestedPromptRow
        elementIDSuffix={elementIDSuffix}
        switches={switches}
        disabled={disabled}
        patchOption={patchOption}
      />
      <span id={terminalHelpID} className="sr-only">
        {terminalHelp}
      </span>
      <MRAutomationRow
        id={`task-mr-merged-prompt-${elementIDSuffix}`}
        label={t("gitlab:mrAutomationMergedLabel")}
        describedBy={terminalHelpID}
        checked={switches.prompt_on_merged}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_merged: checked })}
        help={
          <MRAutomationHelpButton
            testId="mr-terminal-help"
            ariaLabel={t("gitlab:mrAutomationTerminalHelpAriaLabel")}
          >
            {terminalHelp}
          </MRAutomationHelpButton>
        }
      />
      <MRAutomationRow
        id={`task-mr-closed-prompt-${elementIDSuffix}`}
        label={t("gitlab:mrAutomationClosedLabel")}
        describedBy={terminalHelpID}
        checked={switches.prompt_on_closed}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_closed: checked })}
      />
    </>
  );
}
