"use client";

import { IconGitFork } from "@tabler/icons-react";
import { Toggle } from "@kandev/ui/toggle";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

export type FreshBranchToggleProps = {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
};

/**
 * Compact icon toggle shown beside the branch selector for local executors.
 * Pressed = "fork a new branch from the chosen base on submit". Matches the
 * affordance pattern used by other inline toggles in the dialog (e.g. the
 * attach-file button in the prompt input).
 */
export function FreshBranchToggle({ enabled, onToggle }: FreshBranchToggleProps) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Toggle
          variant="outline"
          aria-label={t("task:createANewBranch")}
          pressed={enabled}
          onPressedChange={onToggle}
          data-testid="fresh-branch-toggle"
          className="cursor-pointer"
        >
          <IconGitFork />
        </Toggle>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">{t("task:freshBranchTooltip")}</TooltipContent>
    </Tooltip>
  );
}

export type BranchPlaceholderArgs = {
  lockedToCurrentBranch: boolean;
  currentLocalBranch: string;
  hasRepositorySelection: boolean;
  loading: boolean;
  optionCount: number;
};

export function computeBranchPlaceholder({
  lockedToCurrentBranch,
  currentLocalBranch,
  hasRepositorySelection,
  loading,
  optionCount,
}: BranchPlaceholderArgs) {
  if (lockedToCurrentBranch) return currentLocalBranch || t("task:usesCurrentBranch");
  if (!hasRepositorySelection) return t("task:selectRepositoryFirst");
  if (loading) return t("task:loadingBranches2");
  return optionCount > 0 ? t("task:selectBranch") : t("task:noBranchesFound");
}
