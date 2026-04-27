"use client";

import { IconGitFork } from "@tabler/icons-react";
import { Toggle } from "@kandev/ui/toggle";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

const FRESH_BRANCH_TOOLTIP =
  "Create a new branch from the selected base. Any uncommitted changes in your local clone will be discarded; you'll be asked to confirm if there are any.";

export type FreshBranchToggleProps = {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
};

/**
 * Compact icon toggle shown beside the branch selector for local executors.
 * Pressed = "fork a new branch from the chosen base on submit". Matches the
 * affordance pattern used by other inline toggles in the dialog (e.g. the
 * paperclip in the prompt input).
 */
export function FreshBranchToggle({ enabled, onToggle }: FreshBranchToggleProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Toggle
          variant="outline"
          aria-label="Create a new branch"
          pressed={enabled}
          onPressedChange={onToggle}
          data-testid="fresh-branch-toggle"
          className="cursor-pointer"
        >
          <IconGitFork />
        </Toggle>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">{FRESH_BRANCH_TOOLTIP}</TooltipContent>
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
  if (lockedToCurrentBranch) return currentLocalBranch || "Uses current branch";
  if (!hasRepositorySelection) return "Select repository first";
  if (loading) return "Loading branches...";
  return optionCount > 0 ? "Select branch" : "No branches found";
}
