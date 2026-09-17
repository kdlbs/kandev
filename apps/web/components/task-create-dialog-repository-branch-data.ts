"use client";

import { useMemo } from "react";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import type { Branch } from "@/lib/types/http";
import { useBranches, type BranchSource } from "@/hooks/domains/workspace/use-repository-branches";
import { useRepoBranchAutoselect } from "@/components/task-create-dialog-repo-branch-autoselect";

export type RepositoryBranchDataArgs = {
  row: TaskRepoRow;
  workspaceId: string | null;
  onBranchChange: (value: string) => void;
  branchValue?: string;
  preferredDefaultBranch?: string;
  preferredDefaultBranchLoading?: boolean;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
  remoteBranches?: Branch[];
  isLocalExecutor?: boolean;
  /** True when the current executor clones local rows from their origin. */
  remoteOriginMode?: boolean;
  /** Current origin inspection has not returned a branch set yet. */
  remoteOriginInspectionLoading?: boolean;
};

export function useRepositoryBranchData({
  row,
  workspaceId,
  onBranchChange,
  branchValue = row.branch,
  preferredDefaultBranch,
  preferredDefaultBranchLoading,
  lastUsedBranch,
  userSettingsLoaded,
  remoteBranches,
  isLocalExecutor = false,
  remoteOriginMode,
  remoteOriginInspectionLoading = false,
}: RepositoryBranchDataArgs) {
  const branchSource = useMemo<BranchSource | null>(() => {
    if (!workspaceId) return null;
    if (row.repositoryId) {
      return { kind: "id", workspaceId, repositoryId: row.repositoryId };
    }
    if (row.localPath) {
      return { kind: "path", workspaceId, path: row.localPath };
    }
    return null;
  }, [workspaceId, row.repositoryId, row.localPath]);
  const {
    branches,
    isLoading: branchesLoading,
    refresh: refreshBranches,
    isLoaded: branchesLoaded,
  } = useBranches(branchSource, !!branchSource);
  const effectiveBranchState = resolveEffectiveBranchState({
    branches,
    branchesLoading,
    branchesLoaded,
    isLocalExecutor,
    remoteBranches,
    remoteOriginMode,
    remoteOriginInspectionLoading,
  });
  useRepoBranchAutoselect({
    branchSource,
    branchesLoading: effectiveBranchState.loading,
    branches: effectiveBranchState.branches,
    rowBranch: branchValue,
    onBranchChange,
    preferredDefaultBranch,
    preferredDefaultBranchLoading,
    lastUsedBranch,
    userSettingsLoaded,
  });
  return {
    branches: effectiveBranchState.branches,
    branchesLoading: effectiveBranchState.loading,
    branchesLoaded: effectiveBranchState.loaded,
    refreshBranches,
  };
}

function resolveEffectiveBranchState({
  branches,
  branchesLoading,
  branchesLoaded,
  isLocalExecutor,
  remoteBranches,
  remoteOriginMode,
  remoteOriginInspectionLoading,
}: {
  branches: Branch[];
  branchesLoading: boolean;
  branchesLoaded: boolean;
  isLocalExecutor: boolean;
  remoteBranches?: Branch[];
  remoteOriginMode?: boolean;
  remoteOriginInspectionLoading: boolean;
}): { branches: Branch[]; loading: boolean; loaded: boolean } {
  if (remoteOriginMode === true) {
    return {
      branches: remoteBranches ?? [],
      loading: remoteOriginInspectionLoading || !remoteBranches,
      loaded: !remoteOriginInspectionLoading && Boolean(remoteBranches),
    };
  }
  if (remoteOriginMode === undefined && !isLocalExecutor && remoteBranches) {
    return { branches: remoteBranches, loading: false, loaded: true };
  }
  return { branches, loading: branchesLoading, loaded: branchesLoaded };
}
