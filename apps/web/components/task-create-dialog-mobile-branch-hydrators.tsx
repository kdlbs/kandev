"use client";

import { useCallback } from "react";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { useRepositoryBranchData } from "@/components/task-create-dialog-repository-branch-data";

type MobileRepositoryBranchHydratorsProps = {
  rows: TaskRepoRow[];
  fs: DialogFormState;
  workspaceId: string | null;
  isLocalExecutor: boolean;
  onRowBranchChange: (key: string, value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
};

export function MobileRepositoryBranchHydrators({
  rows,
  fs,
  workspaceId,
  isLocalExecutor,
  onRowBranchChange,
  lastUsedBranch,
  userSettingsLoaded,
}: MobileRepositoryBranchHydratorsProps) {
  return (
    <>
      {rows.map((row) => (
        <MobileRepositoryBranchHydrator
          key={row.key}
          row={row}
          fs={fs}
          workspaceId={workspaceId}
          isLocalExecutor={isLocalExecutor}
          onRowBranchChange={onRowBranchChange}
          lastUsedBranch={lastUsedBranch}
          userSettingsLoaded={userSettingsLoaded}
        />
      ))}
    </>
  );
}

type MobileRepositoryBranchHydratorProps = Omit<MobileRepositoryBranchHydratorsProps, "rows"> & {
  row: TaskRepoRow;
};

function MobileRepositoryBranchHydrator({
  row,
  fs,
  workspaceId,
  isLocalExecutor,
  onRowBranchChange,
  lastUsedBranch,
  userSettingsLoaded,
}: MobileRepositoryBranchHydratorProps) {
  const onBranchChange = useCallback(
    (value: string) => onRowBranchChange(row.key, value),
    [onRowBranchChange, row.key],
  );
  useRepositoryBranchData({
    row,
    workspaceId,
    onBranchChange,
    branchValue: isLocalExecutor ? row.branch : row.baseBranch || row.branch,
    preferredDefaultBranch: isLocalExecutor ? fs.currentLocalBranch : undefined,
    preferredDefaultBranchLoading: isLocalExecutor ? fs.currentLocalBranchLoading : false,
    lastUsedBranch,
    userSettingsLoaded,
  });
  return null;
}
