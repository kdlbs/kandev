"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconEdit, IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { useTaskMRAutomationOptions } from "@/hooks/domains/gitlab/use-task-mr-automation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  findMRAutomationOptionsForMR,
  findMRAutomationStateForMR,
} from "@/lib/gitlab/mr-automation";
import type {
  TaskMR,
  TaskMRAutomationOptions,
  TaskMRAutomationOptionsForMR,
  TaskMRAutomationPatch,
} from "@/lib/types/gitlab";
import { MRAutoFixPromptDialog } from "./mr-auto-fix-prompt-dialog";
import {
  compactRowMinHeight,
  MRAgentPromptRows,
  MRAutomationOptionRows,
} from "./mr-automation-rows";

/** True when any of the three lifecycle-notification switches is on for this MR. */
function hasLifecycleSwitchEnabled(switches: TaskMRAutomationOptionsForMR): boolean {
  return (
    switches.prompt_on_review_requested || switches.prompt_on_merged || switches.prompt_on_closed
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

function MRAutomationHeader({
  mrIID,
  disabled,
  onEditPrompt,
}: {
  mrIID: number;
  disabled: boolean;
  onEditPrompt: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 px-1">
      <div className="flex min-w-0 items-baseline gap-1.5">
        <span className="text-xs font-medium text-foreground">{t("gitlab:mrAutomation")}</span>
        {/* The switches below are scoped to this merge request, so say which. */}
        <span
          data-testid="mr-automation-scope-label"
          className="truncate text-[11px] text-muted-foreground"
        >
          {t("gitlab:mrAutomationAppliesToMR", { iid: mrIID })}
        </span>
      </div>
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
    // A trimmed-empty override is "no override" as far as the backend's
    // effective-prompt resolution is concerned, so the editor must open on
    // the effective prompt rather than on unusable whitespace. Clearing via
    // the UI already round-trips as null, but a whitespace-only override
    // submitted through HTTP PATCH or MCP persists verbatim — `??` alone
    // would then open an all-whitespace textarea with Save disabled.
    const override = options?.auto_fix_prompt_override?.trim()
      ? options.auto_fix_prompt_override
      : null;
    setPromptDraft(override ?? options?.effective_auto_fix_prompt ?? "");
    setPromptOpen(true);
  }, [options]);

  // The auto-fix prompt override is task-level (it applies to every linked
  // MR), so these patches intentionally carry no MR identity.
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
 * every switch row, and stamps `mr`'s identity onto each patch so the backend
 * applies the change to this merge request alone instead of fanning it out to
 * every MR linked to the task. Split out of MRAutomationControls to keep that
 * component under the file's complexity limit.
 */
function useMRAutomationPatch(
  mr: TaskMR,
  update: (patch: TaskMRAutomationPatch) => Promise<unknown>,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  return useCallback(
    (patch: TaskMRAutomationPatch) => {
      const scoped: TaskMRAutomationPatch = {
        ...patch,
        repository_id: mr.repository_id ?? "",
        project_path: mr.project_path,
        mr_iid: mr.mr_iid,
      };
      update(scoped).catch((err) => {
        toast({
          title: t("gitlab:mrAutomationUpdateFailedTitle"),
          description:
            err instanceof Error ? err.message : t("gitlab:mrAutomationUpdateFailedDescription"),
          variant: "error",
        });
      });
    },
    [mr, t, toast, update],
  );
}

/**
 * MR automation controls for one linked merge request: an "Automation"
 * section (auto-fix CI + auto-merge) above a collapsible "Review follow-up"
 * group of three compact switch rows. Renders inside the GitLab MR topbar
 * dropdown and hover popover. Auto-expands "Review follow-up" when any of its
 * switches is already on so a previously configured MR doesn't hide its active
 * switches. Mirrors PRCIAutomationControls + ReviewFollowUpSection (GitHub).
 *
 * `mr` is required: all five switches are per-MR, so there is no meaningful
 * task-wide rendering of this group. A task with several linked MRs renders
 * one instance per MR.
 */
export function MRAutomationControls({ taskId, mr }: { taskId: string; mr: TaskMR }) {
  const { options, loading, saving, error, update, refresh, resetPrompt } =
    useTaskMRAutomationOptions(taskId);
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const promptEditor = useMRAutoFixPromptEditor(options, update, resetPrompt);
  const patchOption = useMRAutomationPatch(mr, update);
  const switches = findMRAutomationOptionsForMR(options?.mr_options, mr);
  const lifecycleEnabled = hasLifecycleSwitchEnabled(switches);
  const minHeight = compactRowMinHeight(isMobile, isFinePointer);
  const automationState = findMRAutomationStateForMR(options?.mr_states, mr);
  const elementIDSuffix = `${taskId}-${mr.id}`;

  useEffect(() => {
    if (lifecycleEnabled) setOpen(true);
  }, [lifecycleEnabled]);

  const loadFailed = Boolean(error) && !options;
  const disabled = saving || loading || !options;

  return (
    <div data-testid="mr-automation-controls" data-mr-iid={mr.mr_iid}>
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
      <MRAutomationHeader
        mrIID={mr.mr_iid}
        disabled={!options}
        onEditPrompt={promptEditor.openPromptEditor}
      />
      <MRAutomationOptionRows
        elementIDSuffix={elementIDSuffix}
        switches={switches}
        disabled={disabled}
        patchOption={patchOption}
        automationState={automationState}
        maxRounds={options?.auto_fix_max_rounds}
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
            elementIDSuffix={elementIDSuffix}
            switches={switches}
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
