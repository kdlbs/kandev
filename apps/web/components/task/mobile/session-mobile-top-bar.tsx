"use client";

import { memo, useState } from "react";
import Link from "next/link";
import {
  IconArrowLeft,
  IconMenu2,
  IconGitBranch,
  IconCheck,
  IconCloud,
  IconCloudOff,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { LineStat } from "@/components/diff-stat";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import {
  CommitDialog,
  PRDialog,
  GitActionsDropdown,
  computeUncommittedStats,
  useMobileGitActions,
} from "./session-mobile-top-bar-git-controls";

type SessionMobileTopBarProps = {
  taskTitle?: string;
  sessionId?: string | null;
  baseBranch?: string;
  worktreeBranch?: string | null;
  onMenuClick: () => void;
  showApproveButton?: boolean;
  onApprove?: () => void;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string | null;
  remoteExecutorName?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
};

function MobileTaskTitle({
  taskTitle,
  displayBranch,
  totalAdditions,
  totalDeletions,
}: {
  taskTitle?: string;
  displayBranch?: string;
  totalAdditions: number;
  totalDeletions: number;
}) {
  return (
    <div className="flex flex-col min-w-0 flex-1">
      <span className="text-sm font-medium truncate">{taskTitle ?? "Task details"}</span>
      {displayBranch && (
        <div className="flex items-center gap-1.5">
          <IconGitBranch className="h-3 w-3 text-muted-foreground flex-shrink-0" />
          <span className="text-xs text-muted-foreground truncate">{displayBranch}</span>
          {(totalAdditions > 0 || totalDeletions > 0) && (
            <LineStat added={totalAdditions} removed={totalDeletions} />
          )}
        </div>
      )}
    </div>
  );
}

function RemoteExecutorCloudStatus({
  isRemoteExecutor,
  remoteExecutorName,
  remoteExecutorType,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
}: {
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string | null;
  remoteExecutorName?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
}) {
  if (!isRemoteExecutor) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center px-1 cursor-default">
          {remoteStatusError ? (
            <IconCloudOff className="h-4 w-4 text-destructive" />
          ) : (
            <IconCloud className="h-4 w-4 text-muted-foreground" />
          )}
        </span>
      </TooltipTrigger>
      <TooltipContent side="bottom" className="space-y-0.5">
        <div className="font-medium">{remoteExecutorName ?? remoteExecutorType ?? "Remote"}</div>
        {remoteState && <div>State: {remoteState}</div>}
        {remoteCreatedAt && <div>Created: {new Date(remoteCreatedAt).toLocaleString()}</div>}
        {remoteCheckedAt && <div>Last check: {new Date(remoteCheckedAt).toLocaleString()}</div>}
        {remoteStatusError && (
          <div className="text-destructive">Status failed: {remoteStatusError}</div>
        )}
      </TooltipContent>
    </Tooltip>
  );
}

type MobileTopBarActionsProps = {
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string | null;
  remoteExecutorName?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
  showApproveButton: boolean;
  onApprove?: () => void;
  sessionId?: string | null;
  isGitLoading: boolean;
  uncommittedCount: number;
  baseBranch?: string;
  onCommitClick: () => void;
  onPRClick: () => void;
  onPull: () => void;
  onPush: (force?: boolean) => void;
  onRebase: () => void;
  onMerge: () => void;
  onMenuClick: () => void;
};

function MobileTopBarActions({
  isRemoteExecutor,
  remoteExecutorType,
  remoteExecutorName,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
  showApproveButton,
  onApprove,
  sessionId,
  isGitLoading,
  uncommittedCount,
  baseBranch,
  onCommitClick,
  onPRClick,
  onPull,
  onPush,
  onRebase,
  onMerge,
  onMenuClick,
}: MobileTopBarActionsProps) {
  return (
    <div className="flex items-center gap-1">
      <RemoteExecutorCloudStatus
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorType={remoteExecutorType}
        remoteExecutorName={remoteExecutorName}
        remoteState={remoteState}
        remoteCreatedAt={remoteCreatedAt}
        remoteCheckedAt={remoteCheckedAt}
        remoteStatusError={remoteStatusError}
      />
      {showApproveButton && onApprove && (
        <Button
          size="sm"
          className="h-7 gap-1 px-2 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white text-xs"
          onClick={onApprove}
        >
          <IconCheck className="h-3.5 w-3.5" />
          Approve
        </Button>
      )}
      <GitActionsDropdown
        sessionId={sessionId}
        isGitLoading={isGitLoading}
        uncommittedCount={uncommittedCount}
        baseBranch={baseBranch}
        onCommitClick={onCommitClick}
        onPRClick={onPRClick}
        onPull={onPull}
        onPush={onPush}
        onRebase={onRebase}
        onMerge={onMerge}
      />
      <Button variant="ghost" size="icon-sm" className="cursor-pointer" onClick={onMenuClick}>
        <IconMenu2 className="h-4 w-4" />
      </Button>
    </div>
  );
}

export const SessionMobileTopBar = memo(function SessionMobileTopBar({
  taskTitle,
  sessionId,
  baseBranch,
  worktreeBranch,
  onMenuClick,
  showApproveButton = false,
  onApprove,
  isRemoteExecutor,
  remoteExecutorType,
  remoteExecutorName,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
}: SessionMobileTopBarProps) {
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [prDialogOpen, setPrDialogOpen] = useState(false);

  const gitStatus = useSessionGitStatus(sessionId ?? null);
  const { commits } = useSessionCommits(sessionId ?? null);
  const {
    isGitLoading,
    handlePull,
    handlePush,
    handleRebase,
    handleMerge,
    handleCommit,
    handleCreatePR,
  } = useMobileGitActions(sessionId, baseBranch, setCommitDialogOpen, setPrDialogOpen);

  const displayBranch = worktreeBranch || baseBranch;
  const {
    additions: uncommittedAdditions,
    deletions: uncommittedDeletions,
    count: uncommittedCount,
  } = computeUncommittedStats(gitStatus?.files);
  const totalAdditions = uncommittedAdditions + commits.reduce((s, c) => s + c.insertions, 0);
  const totalDeletions = uncommittedDeletions + commits.reduce((s, c) => s + c.deletions, 0);

  return (
    <header className="flex items-center justify-between px-2 py-2 bg-background">
      <div className="flex items-center gap-2 min-w-0 flex-1">
        <Button variant="ghost" size="icon-sm" asChild>
          <Link href="/">
            <IconArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <MobileTaskTitle
          taskTitle={taskTitle}
          displayBranch={displayBranch}
          totalAdditions={totalAdditions}
          totalDeletions={totalDeletions}
        />
      </div>
      <MobileTopBarActions
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorType={remoteExecutorType}
        remoteExecutorName={remoteExecutorName}
        remoteState={remoteState}
        remoteCreatedAt={remoteCreatedAt}
        remoteCheckedAt={remoteCheckedAt}
        remoteStatusError={remoteStatusError}
        showApproveButton={showApproveButton}
        onApprove={onApprove}
        sessionId={sessionId}
        isGitLoading={isGitLoading}
        uncommittedCount={uncommittedCount}
        baseBranch={baseBranch}
        onCommitClick={() => setCommitDialogOpen(true)}
        onPRClick={() => setPrDialogOpen(true)}
        onPull={handlePull}
        onPush={handlePush}
        onRebase={handleRebase}
        onMerge={handleMerge}
        onMenuClick={onMenuClick}
      />
      <CommitDialog
        open={commitDialogOpen}
        onOpenChange={setCommitDialogOpen}
        uncommittedCount={uncommittedCount}
        uncommittedAdditions={uncommittedAdditions}
        uncommittedDeletions={uncommittedDeletions}
        isGitLoading={isGitLoading}
        onCommit={handleCommit}
      />
      <PRDialog
        open={prDialogOpen}
        onOpenChange={setPrDialogOpen}
        displayBranch={displayBranch}
        baseBranch={baseBranch}
        isGitLoading={isGitLoading}
        taskTitle={taskTitle}
        onCreatePR={handleCreatePR}
      />
    </header>
  );
});
