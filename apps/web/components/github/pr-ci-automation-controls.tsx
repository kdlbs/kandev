"use client";

import { useCallback, useState } from "react";
import { useToast } from "@/components/toast-provider";
import { useTaskCIAutomationOptions } from "@/hooks/domains/github/use-task-ci-options";
import {
  findCIAutomationStateForPR,
  findPRAutomationOptionsForPR,
  deriveCIAutomationQueueState,
} from "@/lib/github/ci-automation";
import type { TaskCIAutomationPatch, TaskPR } from "@/lib/types/github";
import { CIAutomationPromptDialog } from "@/components/github/pr-ci-automation-prompt-dialog";
import {
  CIAutomationErrorRow,
  CIAutomationHeader,
  CIAutomationOptionRows,
} from "@/components/github/pr-ci-automation-rows";

export function PRCIAutomationControls({ pr }: { pr: TaskPR }) {
  const { options, loading, saving, error, refresh, update, resetPrompt } =
    useTaskCIAutomationOptions(pr.task_id);
  const { toast } = useToast();
  const [promptOpen, setPromptOpen] = useState(false);
  const [promptDraft, setPromptDraft] = useState("");
  const automationState = findCIAutomationStateForPR(options?.pr_states, pr);
  const prOptions = findPRAutomationOptionsForPR(options?.pr_options, pr);
  const queueState = deriveCIAutomationQueueState(pr, prOptions, automationState);

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

  // The five automation switches are scoped to this PR; every patch from the
  // switch rows below carries this PR's identity so the backend applies it
  // to this PR only instead of fanning out to every linked PR.
  const patchOption = useCallback(
    (patch: TaskCIAutomationPatch) => {
      const scoped: TaskCIAutomationPatch = {
        ...patch,
        repository_id: pr.repository_id ?? "",
        pr_number: pr.pr_number,
      };
      Promise.resolve(update(scoped)).catch(() => reportError("Failed to update CI automation."));
    },
    [pr, reportError, update],
  );

  // The auto-fix prompt override stays task-level (applies to every linked
  // PR), so this patch intentionally omits PR identity.
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
      <CIAutomationHeader
        pr={pr}
        queueState={queueState}
        disabled={!options}
        onEditPrompt={openPromptEditor}
      />
      <CIAutomationOptionRows
        pr={pr}
        prOptions={prOptions}
        autoFixMaxRounds={options?.auto_fix_max_rounds}
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
