"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Label } from "@kandev/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { useTaskMRAutomationOptions } from "@/hooks/domains/gitlab/use-task-mr-automation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { TaskMRAutomationOptions, TaskMRAutomationPatch } from "@/lib/types/gitlab";

/**
 * Dual-mode help affordance: a tap popover on coarse pointers, a hover
 * tooltip on fine pointers. Mirrors CIAutomationHelpButton (GitHub).
 */
function MRAutomationHelpButton({
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

function MRAutomationErrorRow({ error }: { error: string }) {
  return (
    <div role="alert" className="px-1 text-[11px] text-destructive">
      <span className="min-w-0 flex-1 truncate">{error}</span>
    </div>
  );
}

/**
 * Shown outside the collapsible section so an initial-load failure stays
 * visible (and recoverable) in the default collapsed state — the group
 * never auto-expands when there are no loaded options to detect an enabled
 * switch from, so an error nested inside CollapsibleContent would otherwise
 * be invisible until the user manually opens it.
 */
function MRAutomationLoadErrorBanner({ error, onRetry }: { error: string; onRetry: () => void }) {
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const minHeight = isMobile || !isFinePointer ? "min-h-11" : "min-h-7";
  return (
    <div
      role="alert"
      className={`flex items-center justify-between gap-2 px-1 text-[11px] text-destructive ${minHeight}`}
    >
      <span className="min-w-0 flex-1 truncate">{error}</span>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        data-testid="mr-automation-retry"
        className={`cursor-pointer px-2 text-[11px] ${minHeight}`}
        onClick={onRetry}
      >
        {t("gitlab:mrAutomationRetry")}
      </Button>
    </div>
  );
}

function ReviewRequestedPromptRow({
  taskId,
  options,
  disabled,
  patchOption,
}: {
  taskId: string;
  options: TaskMRAutomationOptions | null;
  disabled: boolean;
  patchOption: (patch: TaskMRAutomationPatch) => void;
}) {
  const { t } = useTranslation();
  const helpID = `task-mr-review-requested-prompt-${taskId}-description`;
  const help = t("gitlab:mrAutomationReviewRequestedHelp");
  return (
    <>
      <span id={helpID} className="sr-only">
        {help}
      </span>
      <MRAutomationRow
        id={`task-mr-review-requested-prompt-${taskId}`}
        label={t("gitlab:mrAutomationReviewRequestedLabel")}
        describedBy={helpID}
        checked={Boolean(options?.prompt_on_review_requested)}
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

function MRAgentPromptRows({
  taskId,
  options,
  disabled,
  patchOption,
}: {
  taskId: string;
  options: TaskMRAutomationOptions | null;
  disabled: boolean;
  patchOption: (patch: TaskMRAutomationPatch) => void;
}) {
  const { t } = useTranslation();
  const terminalHelpID = `task-mr-terminal-help-${taskId}`;
  const terminalHelp = t("gitlab:mrAutomationTerminalHelp");
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
      <MRAutomationRow
        id={`task-mr-merged-prompt-${taskId}`}
        label={t("gitlab:mrAutomationMergedLabel")}
        describedBy={terminalHelpID}
        checked={Boolean(options?.prompt_on_merged)}
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
        id={`task-mr-closed-prompt-${taskId}`}
        label={t("gitlab:mrAutomationClosedLabel")}
        describedBy={terminalHelpID}
        checked={Boolean(options?.prompt_on_closed)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ prompt_on_closed: checked })}
      />
    </>
  );
}

/**
 * Collapsible "Review follow-up" group of three compact switch rows. Renders
 * inside the GitLab MR topbar dropdown, below the per-MR items. Auto-expands
 * when any switch is already on so a previously configured task doesn't hide
 * its active switches. Mirrors ReviewFollowUpSection (GitHub), AC29.
 */
export function MRAutomationControls({ taskId }: { taskId: string }) {
  const { options, loading, saving, error, update, refresh } = useTaskMRAutomationOptions(taskId);
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { toast } = useToast();
  const lifecycleEnabled = Boolean(
    options?.prompt_on_review_requested || options?.prompt_on_merged || options?.prompt_on_closed,
  );
  const minHeight = isMobile || !isFinePointer ? "min-h-11" : "min-h-7";

  useEffect(() => {
    if (lifecycleEnabled) setOpen(true);
  }, [lifecycleEnabled]);

  const patchOption = (patch: TaskMRAutomationPatch) => {
    update(patch).catch((err) => {
      toast({
        title: t("gitlab:mrAutomationUpdateFailedTitle"),
        description:
          err instanceof Error ? err.message : t("gitlab:mrAutomationUpdateFailedDescription"),
        variant: "error",
      });
    });
  };

  const loadFailed = Boolean(error) && !options;

  return (
    <div data-testid="mr-automation-controls">
      {loadFailed ? (
        <MRAutomationLoadErrorBanner
          error={error as string}
          onRetry={() => {
            refresh().catch(() => {
              // Error state is already surfaced by the hook; nothing further to do here.
            });
          }}
        />
      ) : null}
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            data-testid="mr-review-follow-up-trigger"
            aria-label={t("gitlab:mrAutomationToggleAriaLabel")}
            className={`w-full cursor-pointer justify-between px-1 text-xs text-muted-foreground ${minHeight}`}
          >
            {t("gitlab:mrAutomationTriggerLabel")}
            <IconChevronDown
              aria-hidden="true"
              className={`h-3.5 w-3.5 transition-transform ${open ? "rotate-180" : ""}`}
            />
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="flex flex-col gap-1">
          <p className="px-1 text-[11px] leading-snug text-muted-foreground">
            {t("gitlab:mrAutomationDescription")}
          </p>
          <MRAgentPromptRows
            taskId={taskId}
            options={options}
            disabled={saving || loading || !options}
            patchOption={patchOption}
          />
          {error && options ? <MRAutomationErrorRow error={error} /> : null}
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
