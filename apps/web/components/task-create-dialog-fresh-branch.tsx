"use client";

import { Switch } from "@kandev/ui/switch";
import { Label } from "@kandev/ui/label";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export type FreshBranchSwitchProps = {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
  currentLocalBranch: string;
};

function freshBranchTooltip(enabled: boolean, currentLocalBranch: string): string {
  if (enabled) {
    return "Forks a new branch from the selected base. Any uncommitted changes in your local clone will be discarded; you'll be asked to confirm if there are any.";
  }
  if (currentLocalBranch) {
    return `Uses ${currentLocalBranch} (currently checked out). Enable to start the task on a new branch forked from a base of your choice.`;
  }
  return "Uses the branch currently checked out. Enable to start the task on a new branch.";
}

/**
 * Inline switch shown next to the branch selector for local executors. Toggling
 * on enables the branch selector for picking a base branch (the new branch
 * name is generated server-side from the task title).
 */
export function FreshBranchSwitch({
  enabled,
  onToggle,
  currentLocalBranch,
}: FreshBranchSwitchProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="flex h-7 shrink-0 items-center gap-1.5 rounded-md border border-input px-2 text-xs">
          <Switch
            id="fresh-branch-toggle"
            checked={enabled}
            onCheckedChange={onToggle}
            data-testid="fresh-branch-toggle"
          />
          <Label htmlFor="fresh-branch-toggle" className="cursor-pointer font-medium">
            New branch
          </Label>
        </div>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        {freshBranchTooltip(enabled, currentLocalBranch)}
      </TooltipContent>
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
