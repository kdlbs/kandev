"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import { Switch } from "@kandev/ui/switch";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export type FreshBranchControlsProps = {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
  newBranchName: string;
  onNewBranchNameChange: (value: string) => void;
  currentLocalBranch: string;
};

function freshBranchTooltip(enabled: boolean, currentLocalBranch: string): string {
  if (enabled) {
    return "Pick a base branch above. Any uncommitted changes in your local clone will be discarded; you'll be asked to confirm if there are any.";
  }
  if (currentLocalBranch) {
    return `Local clone will use the branch currently checked out (${currentLocalBranch}). Enable to start the task on a new branch instead.`;
  }
  return "Local clone will use the branch currently checked out. Enable to start the task on a new branch instead.";
}

export function FreshBranchControls({
  enabled,
  onToggle,
  newBranchName,
  onNewBranchNameChange,
  currentLocalBranch,
}: FreshBranchControlsProps) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-md border border-dashed border-border px-3 py-2">
      <div className="flex items-center gap-2">
        <Switch
          id="fresh-branch-toggle"
          checked={enabled}
          onCheckedChange={onToggle}
          data-testid="fresh-branch-toggle"
        />
        <Label htmlFor="fresh-branch-toggle" className="cursor-pointer text-xs font-medium">
          New branch
        </Label>
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className="cursor-help text-muted-foreground"
              aria-label="What does New branch do?"
              data-testid="fresh-branch-tooltip-trigger"
            >
              <IconInfoCircle className="h-3.5 w-3.5" />
            </span>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs">
            {freshBranchTooltip(enabled, currentLocalBranch)}
          </TooltipContent>
        </Tooltip>
      </div>
      {enabled && (
        <Input
          value={newBranchName}
          onChange={(e) => onNewBranchNameChange(e.target.value)}
          placeholder="new-branch-name"
          aria-label="New branch name"
          data-testid="new-branch-name-input"
          className="h-7 max-w-[260px] text-xs"
        />
      )}
    </div>
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
