"use client";

import { useEffect } from "react";
import { autoSelectBranch } from "@/components/task-create-dialog-helpers";
import type { BranchSource } from "@/hooks/domains/workspace/use-repository-branches";

export function useRepoBranchAutoselect({
  branchSource,
  branchesLoading,
  branches,
  rowBranch,
  onBranchChange,
  preferredDefaultBranch,
  preferredDefaultBranchLoading,
  lastUsedBranch,
  userSettingsLoaded,
}: {
  branchSource: BranchSource | null;
  branchesLoading: boolean;
  branches: Parameters<typeof autoSelectBranch>[0];
  rowBranch?: string;
  onBranchChange: (value: string) => void;
  preferredDefaultBranch?: string;
  preferredDefaultBranchLoading?: boolean;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
}) {
  useEffect(() => {
    if (!branchSource || branchesLoading || branches.length === 0 || rowBranch) return;
    runAutoselect({
      branches,
      preferredDefaultBranch,
      preferredDefaultBranchLoading: !!preferredDefaultBranchLoading,
      lastUsedBranch,
      userSettingsLoaded,
      onBranchChange,
    });
  }, [
    branchSource,
    branchesLoading,
    branches,
    rowBranch,
    onBranchChange,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
    lastUsedBranch,
    userSettingsLoaded,
  ]);
}

function runAutoselect({
  branches,
  preferredDefaultBranch,
  preferredDefaultBranchLoading,
  lastUsedBranch,
  userSettingsLoaded,
  onBranchChange,
}: {
  branches: Parameters<typeof autoSelectBranch>[0];
  preferredDefaultBranch: string | undefined;
  preferredDefaultBranchLoading: boolean;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
  onBranchChange: (value: string) => void;
}) {
  if (preferredDefaultBranchLoading) return;
  if (preferredDefaultBranch) {
    onBranchChange(preferredDefaultBranch);
    return;
  }
  autoSelectBranch(branches, onBranchChange, { lastUsedBranch, userSettingsLoaded });
}
