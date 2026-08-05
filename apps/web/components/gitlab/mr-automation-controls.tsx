"use client";

import { useCallback, useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconEdit, IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Label } from "@kandev/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { useTaskMRAutomationOptions } from "@/hooks/domains/gitlab/use-task-mr-automation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { autoFixRoundForState, findMRAutomationStateForMR } from "@/lib/gitlab/mr-automation";
import type {
  TaskMR,
  TaskMRAutomationOptions,
  TaskMRAutomationPatch,
  TaskMRLifecycleState,
} from "@/lib/types/gitlab";
import { MRAutoFixPromptDialog } from "./mr-auto-fix-prompt-dialog";

/** Shared compact-row height: taller touch target on mobile/coarse pointers. */
function compactRowMinHeight(isMobile: boolean, isFinePointer: boolean): string {
  return isMobile || !isFinePointer ? "min-h-11" : "min-h-7";
}

/** True when any of the three #2125 lifecycle-notification switches is on. */
function hasLifecycleSwitchEnabled(options: TaskMRAutomationOptions | null): boolean {
  return Boolean(
    options?.prompt_on_review_requested || options?.prompt_on_merged || options?.prompt_on_closed,
  );
}

/** Resolves the round-help state for a single MR, when one is known. */
function resolveAutomationState(
  mr: TaskMR | undefined,
  states: TaskMRLifecycleState[] | undefined,
): TaskMRLifecycleState | undefined {
  return mr ? findMRAutomationStateForMR(states, mr) : undefined;
}

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
  const minHeight = compactRowMinHeight(isMobile, isFinePointer);
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

function MRAutomationHeader({
  disabled,
  onEditPrompt,
}: {
  disabled: boolean;
  onEditPrompt: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 px-1">
      <div className="text-xs font-medium text-foreground">{t("gitlab:mrAutomation")}</div>
      <div className="flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-6 w-6 cursor-help text-muted-foreground hover:text-foreground"
              aria-label={t("gitlab:mrAutomationExplainOptions")}
            >
              <IconInfoCircle className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="top" align="end" className="max-w-[280px] text-xs leading-relaxed">
            {t("gitlab:mrAutomationWatchesLinkedMergeRequest")}
          </TooltipContent>
        </Tooltip>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6 cursor-pointer text-muted-foreground hover:text-foreground"
          aria-label={t("gitlab:mrAutomationEditAutoFixPromptForThis")}
          disabled={disabled}
          onClick={onEditPrompt}
        >
          <IconEdit className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

function MRAutomationOptionRows({
  taskId,
  options,
  disabled,
  patchOption,
  automationState,
}: {
  taskId: string;
  options: TaskMRAutomationOptions | null;
  disabled: boolean;
  patchOption: (patch: TaskMRAutomationPatch) => void;
  automationState: TaskMRLifecycleState | undefined;
}) {
  const { t } = useTranslation();
  return (
    <>
      <MRAutomationRow
        id={`task-mr-auto-fix-${taskId}`}
        label={t("gitlab:mrAutoFixCiAndAddressComments")}
        checked={Boolean(options?.auto_fix_enabled)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_fix_enabled: checked })}
        help={
          options?.auto_fix_enabled ? (
            <MRAutoFixRoundHelpButton
              state={automationState}
              maxRounds={options.auto_fix_max_rounds}
            />
          ) : null
        }
      />
      <MRAutomationRow
        id={`task-mr-auto-merge-${taskId}`}
        label={t("gitlab:mrAutoMergeWhenReady")}
        checked={Boolean(options?.auto_merge_enabled)}
        disabled={disabled}
        onCheckedChange={(checked) => patchOption({ auto_merge_enabled: checked })}
      />
    </>
  );
}

/**
 * Encapsulates the auto-fix prompt editor's local state and save/reset
 * handlers, split out of MRAutomationControls to keep that component under
 * the file's size/complexity limits.
 */
function useMRAutoFixPromptEditor(
  options: TaskMRAutomationOptions | null,
  update: (patch: TaskMRAutomationPatch) => Promise<unknown>,
  resetPrompt: () => Promise<unknown>,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [promptOpen, setPromptOpen] = useState(false);
  const [promptDraft, setPromptDraft] = useState("");

  const reportError = useCallback(
    (description: string) => {
      toast({ description, variant: "error" });
    },
    [toast],
  );

  const openPromptEditor = useCallback(() => {
    setPromptDraft(options?.auto_fix_prompt_override ?? options?.effective_auto_fix_prompt ?? "");
    setPromptOpen(true);
  }, [options]);

  const savePrompt = useCallback(() => {
    const value = promptDraft.trim();
    if (!value) return;
    Promise.resolve(update({ auto_fix_prompt_override: value }))
      .then(() => setPromptOpen(false))
      .catch(() => reportError(t("gitlab:mrAutomationAutoFixPromptSaveFailed")));
  }, [promptDraft, reportError, t, update]);

  const useDefaultPrompt = useCallback(() => {
    Promise.resolve(resetPrompt())
      .then(() => setPromptOpen(false))
      .catch(() => reportError(t("gitlab:mrAutomationAutoFixPromptResetFailed")));
  }, [reportError, resetPrompt, t]);

  return {
    promptOpen,
    promptDraft,
    setPromptDraft,
    openPromptEditor,
    savePrompt,
    useDefaultPrompt,
    closePromptEditor: () => setPromptOpen(false),
  };
}

/**
 * Wraps `update` with the standard "toast on failure" handling shared by
 * every switch row. Split out of MRAutomationControls to keep that
 * component under the file's complexity limit.
 */
function useMRAutomationPatch(update: (patch: TaskMRAutomationPatch) => Promise<unknown>) {
  const { t } = useTranslation();
  const { toast } = useToast();
  return useCallback(
    (patch: TaskMRAutomationPatch) => {
      update(patch).catch((err) => {
        toast({
          title: t("gitlab:mrAutomationUpdateFailedTitle"),
          description:
            err instanceof Error ? err.message : t("gitlab:mrAutomationUpdateFailedDescription"),
          variant: "error",
        });
      });
    },
    [t, toast, update],
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
 * MR automation controls: an "Automation" section (auto-fix CI + auto-merge)
 * above a collapsible "Review follow-up" group of three compact switch rows.
 * Renders inside the GitLab MR topbar dropdown, below the per-MR items.
 * Auto-expands "Review follow-up" when any of its switches is already on so
 * a previously configured task doesn't hide its active switches. Mirrors
 * PRCIAutomationControls + ReviewFollowUpSection (GitHub), AC1, AC29.
 *
 * `mr` is optional and only used to look up the per-MR auto-fix round state
 * for the round-help button's count — when omitted (e.g. a task with
 * multiple linked MRs and no single one to attribute rounds to), the
 * round-help button still renders but reads 0 of max; the switches
 * themselves remain task-scoped either way.
 */
export function MRAutomationControls({ taskId, mr }: { taskId: string; mr?: TaskMR }) {
  const { options, loading, saving, error, update, refresh, resetPrompt } =
    useTaskMRAutomationOptions(taskId);
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const promptEditor = useMRAutoFixPromptEditor(options, update, resetPrompt);
  const patchOption = useMRAutomationPatch(update);
  const lifecycleEnabled = hasLifecycleSwitchEnabled(options);
  const minHeight = compactRowMinHeight(isMobile, isFinePointer);
  const automationState = resolveAutomationState(mr, options?.mr_states);

  useEffect(() => {
    if (lifecycleEnabled) setOpen(true);
  }, [lifecycleEnabled]);

  const loadFailed = Boolean(error) && !options;
  const disabled = saving || loading || !options;

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
      <MRAutomationHeader disabled={!options} onEditPrompt={promptEditor.openPromptEditor} />
      <MRAutomationOptionRows
        taskId={taskId}
        options={options}
        disabled={disabled}
        patchOption={patchOption}
        automationState={automationState}
      />
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
            disabled={disabled}
            patchOption={patchOption}
          />
          {error && options ? <MRAutomationErrorRow error={error} /> : null}
        </CollapsibleContent>
      </Collapsible>
      <MRAutoFixPromptDialog
        open={promptEditor.promptOpen}
        prompt={promptEditor.promptDraft}
        saving={saving}
        onPromptChange={promptEditor.setPromptDraft}
        onClose={promptEditor.closePromptEditor}
        onSave={promptEditor.savePrompt}
        onReset={promptEditor.useDefaultPrompt}
      />
    </div>
  );
}
