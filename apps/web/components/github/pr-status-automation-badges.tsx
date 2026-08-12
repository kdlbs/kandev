"use client";

import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { autoFixRoundForState, findCIAutomationStateForPR } from "@/lib/github/ci-automation";
import type { AutoFixRoundInfo } from "@/lib/github/ci-automation";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

export type AutomationFlags = {
  autoFix: boolean;
  autoMerge: boolean;
  autoFixRound: AutoFixRoundInfo | null;
  promptOnReviewRequested: boolean;
  promptOnMerged: boolean;
  promptOnClosed: boolean;
};

function taskWideAutomationFlags(
  options: TaskCIAutomationOptions | null | undefined,
): Omit<AutomationFlags, "autoFixRound"> {
  return {
    autoFix: Boolean(options?.auto_fix_enabled),
    autoMerge: Boolean(options?.auto_merge_enabled),
    promptOnReviewRequested: Boolean(options?.prompt_on_review_requested),
    promptOnMerged: Boolean(options?.prompt_on_merged),
    promptOnClosed: Boolean(options?.prompt_on_closed),
  };
}

export function automationForPR(
  options: TaskCIAutomationOptions | null | undefined,
  pr: TaskPR,
): AutomationFlags {
  return {
    ...taskWideAutomationFlags(options),
    autoFixRound: options?.auto_fix_enabled
      ? autoFixRoundForState(
          findCIAutomationStateForPR(options.pr_states, pr),
          options.auto_fix_max_rounds,
        )
      : null,
  };
}

export function automationForPRs(
  options: TaskCIAutomationOptions | null | undefined,
  prs: TaskPR[],
): AutomationFlags {
  const roundInfos = options?.auto_fix_enabled
    ? prs.map((pr) =>
        autoFixRoundForState(
          findCIAutomationStateForPR(options.pr_states, pr),
          options.auto_fix_max_rounds,
        ),
      )
    : [];
  return {
    ...taskWideAutomationFlags(options),
    autoFixRound: pickAttentionRound(roundInfos),
  };
}

function pickAttentionRound(roundInfos: AutoFixRoundInfo[]): AutoFixRoundInfo | null {
  if (roundInfos.length === 0) return null;
  return roundInfos.reduce((best, next) => {
    if (next.exhausted && !best.exhausted) return next;
    if (next.exhausted === best.exhausted && next.current > best.current) return next;
    return best;
  });
}

function prEventsCount(automation: AutomationFlags): number {
  return [
    automation.promptOnReviewRequested,
    automation.promptOnMerged,
    automation.promptOnClosed,
  ].filter(Boolean).length;
}

export function automationAriaSuffix(automation: AutomationFlags, t: TFunction): string {
  const flags = [
    autoFixAriaLabel(automation, t),
    automation.autoMerge ? t("github:autoMergeEnabledAria") : null,
    automation.promptOnReviewRequested ? t("github:yourReviewIsRequested") : null,
    automation.promptOnMerged ? t("github:prMerged") : null,
    automation.promptOnClosed ? t("github:prClosedWithoutMerging") : null,
  ].filter(Boolean);
  return flags.length > 0 ? `, ${flags.join(", ")}` : "";
}

function autoFixAriaLabel(automation: AutomationFlags, t: TFunction): string | null {
  if (!automation.autoFix) return null;
  if (!automation.autoFixRound) return t("github:autoFixEnabledAria");
  if (automation.autoFixRound.exhausted) {
    return t("github:autoFixExhaustedAria", {
      current: automation.autoFixRound.current,
      max: automation.autoFixRound.max,
    });
  }
  return t("github:autoFixEnabledWithRoundsAria", {
    current: automation.autoFixRound.current,
    max: automation.autoFixRound.max,
  });
}

export function AutomationFlagBadges({ automation }: { automation: AutomationFlags }) {
  const { t } = useTranslation();
  const enabledPrEvents = prEventsCount(automation);
  if (!automation.autoFix && !automation.autoMerge && enabledPrEvents === 0) return null;
  const autoFixRound = automation.autoFixRound;
  return (
    <>
      {automation.autoFix && autoFixRound && (
        <span
          data-testid="pr-status-auto-fix-chip"
          data-auto-fix-round={`${autoFixRound.current}/${autoFixRound.max}`}
          data-auto-fix-exhausted={autoFixRound.exhausted ? "true" : "false"}
          className={`rounded-sm px-1 py-0.5 text-[9px] font-medium leading-none ${
            autoFixRound.exhausted
              ? "bg-yellow-500/15 text-yellow-500"
              : "bg-emerald-500/15 text-emerald-500"
          }`}
        >
          {t(autoFixRound.exhausted ? "github:autoFixExhausted" : "github:autoFix")}{" "}
          {autoFixRound.current}/{autoFixRound.max}
        </span>
      )}
      {automation.autoMerge && (
        <span
          data-testid="pr-status-auto-merge-chip"
          className="rounded-sm bg-sky-500/15 px-1 py-0.5 text-[9px] font-medium leading-none text-sky-500"
        >
          {t("github:autoMerge")}
        </span>
      )}
      {enabledPrEvents > 0 && (
        <span
          data-testid="pr-status-pr-events-chip"
          data-legacy-testid="pr-status-follow-up-chip"
          data-pr-events-count={enabledPrEvents}
          className="rounded-sm bg-violet-500/15 px-1 py-0.5 text-[9px] font-medium leading-none text-violet-500"
        >
          {t("github:prEvents")} {enabledPrEvents}/3
        </span>
      )}
    </>
  );
}
