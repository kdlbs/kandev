"use client";

import { useCallback } from "react";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { useRepositoryBranchData } from "@/components/task-create-dialog-repository-branch-data";
import type { RepositoryCloneSourceState } from "@/hooks/domains/repositories/use-repository-clone-source";
import {
  isHostRepositorySelection,
  remoteOriginBranchesForState,
} from "@/components/task-create-dialog-remote-origin-inspection";

type MobileRepositoryBranchHydratorsProps = {
  rows: TaskRepoRow[];
  repositories: Repository[];
  fs: DialogFormState;
  workspaceId: string | null;
  isLocalExecutor: boolean;
  onRowBranchChange: (key: string, value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
  remoteOriginMode?: boolean;
  remoteOriginStates?: Record<string, RepositoryCloneSourceState>;
};

export function MobileRepositoryBranchHydrators({
  rows,
  repositories,
  fs,
  workspaceId,
  isLocalExecutor,
  onRowBranchChange,
  lastUsedBranch,
  userSettingsLoaded,
  remoteOriginMode,
  remoteOriginStates,
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
          remoteOriginMode={
            remoteOriginMode && isHostRepositorySelection({ kind: "local", ...row }, repositories)
          }
          remoteOriginState={remoteOriginStates?.[row.key]}
        />
      ))}
    </>
  );
}

type MobileRepositoryBranchHydratorProps = Omit<
  MobileRepositoryBranchHydratorsProps,
  "rows" | "repositories" | "remoteOriginStates"
> & {
  row: TaskRepoRow;
  remoteOriginState?: RepositoryCloneSourceState;
};

function MobileRepositoryBranchHydrator({
  row,
  fs,
  workspaceId,
  isLocalExecutor,
  onRowBranchChange,
  lastUsedBranch,
  userSettingsLoaded,
  remoteOriginMode,
  remoteOriginState,
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
    remoteOriginMode,
    remoteBranches: remoteOriginBranchesForState(remoteOriginMode, remoteOriginState),
    remoteOriginInspectionLoading:
      remoteOriginMode && (!remoteOriginState || remoteOriginState.status === "checking"),
  });
  return null;
}
