"use client";

import { memo, useState } from "react";
import Link from "@/components/routing/app-link";
import { IconArrowLeft, IconMenu2, IconGitBranch, IconCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
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
import { MobileRepoPill } from "./mobile-repo-pill";
import { TaskTopBarPluginActions } from "@/components/task/task-top-bar-plugin-actions";
import { MRTopbarButton } from "@/components/gitlab/mr-topbar-button";

type SessionMobileTopBarProps = {
  taskId?: string | null;
  workspaceId?: string | null;
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
  isArchived?: boolean;
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

function RemoteExecutorIndicator({
  taskId,
  sessionId,
  remoteExecutorType,
  remoteExecutorName,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
}: {
  taskId?: string | null;
  sessionId?: string | null;
  remoteExecutorType?: string | null;
  remoteExecutorName?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
}) {
  return (
    <RemoteCloudTooltip
      taskId={taskId ?? ""}
      sessionId={sessionId}
      executorType={remoteExecutorType}
      fallbackName={remoteExecutorName ?? remoteExecutorType}
      iconClassName="h-4 w-4"
      status={{
        remote_name: remoteExecutorName ?? undefined,
        remote_state: remoteState ?? undefined,
        remote_created_at: remoteCreatedAt ?? undefined,
        remote_checked_at: remoteCheckedAt ?? undefined,
        remote_status_error: remoteStatusError ?? undefined,
      }}
    />
  );
}

function ApproveButton({ onApprove }: { onApprove: () => void }) {
  return (
    <Button
      size="sm"
      className="h-7 gap-1 px-2 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white text-xs"
      onClick={onApprove}
    >
      <IconCheck className="h-3.5 w-3.5" />
      Approve
    </Button>
  );
}

function useMobileGitMetrics(
  sessionId: string | null | undefined,
  worktreeBranch: string | null | undefined,
  baseBranch: string | undefined,
) {
  const gitStatus = useSessionGitStatus(sessionId ?? null);
  const { commits } = useSessionCommits(sessionId ?? null);
  const stats = computeUncommittedStats(gitStatus?.files);

  return {
    commits,
    displayBranch: worktreeBranch || baseBranch,
    uncommittedAdditions: stats.additions,
    uncommittedDeletions: stats.deletions,
    uncommittedCount: stats.count,
    totalAdditions: stats.additions + commits.reduce((sum, commit) => sum + commit.insertions, 0),
    totalDeletions: stats.deletions + commits.reduce((sum, commit) => sum + commit.deletions, 0),
  };
}

type MobileGitDialogsProps = {
  commitDialogOpen: boolean;
  setCommitDialogOpen: (open: boolean) => void;
  prDialogOpen: boolean;
  setPrDialogOpen: (open: boolean) => void;
  displayBranch?: string;
  baseBranch?: string;
  taskTitle?: string;
  firstCommitMessage?: string;
  isGitLoading: boolean;
  branchPushed: boolean;
  uncommittedCount: number;
  uncommittedAdditions: number;
  uncommittedDeletions: number;
  onCommit: (message: string, stageAll: boolean) => void;
  onCreatePR: (title: string, body: string, draft: boolean) => void;
};

function MobileGitDialogs(props: MobileGitDialogsProps) {
  return (
    <>
      <CommitDialog
        open={props.commitDialogOpen}
        onOpenChange={props.setCommitDialogOpen}
        uncommittedCount={props.uncommittedCount}
        uncommittedAdditions={props.uncommittedAdditions}
        uncommittedDeletions={props.uncommittedDeletions}
        isGitLoading={props.isGitLoading}
        onCommit={props.onCommit}
      />
      <PRDialog
        open={props.prDialogOpen}
        onOpenChange={props.setPrDialogOpen}
        displayBranch={props.displayBranch}
        baseBranch={props.baseBranch}
        isGitLoading={props.isGitLoading}
        taskTitle={props.taskTitle}
        firstCommitMessage={props.firstCommitMessage}
        onCreatePR={props.onCreatePR}
        branchPushed={props.branchPushed}
      />
    </>
  );
}

type MobileTopBarActionsProps = {
  taskId?: string | null;
  workspaceId?: string | null;
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
  taskTitle?: string;
  isArchived?: boolean;
  onCommitClick: () => void;
  onPRClick: () => void;
  onPull: () => void;
  onPush: (force?: boolean) => void;
  onRebase: () => void;
  onMerge: () => void;
  onMenuClick: () => void;
};

function MobileTopBarActions({
  taskId,
  workspaceId,
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
  taskTitle,
  isArchived,
  onCommitClick,
  onPRClick,
  onPull,
  onPush,
  onRebase,
  onMerge,
  onMenuClick,
}: MobileTopBarActionsProps) {
  return (
    <div className="flex items-center gap-1" data-testid="mobile-topbar-actions">
      <MobileRepoPill taskId={taskId ?? null} workspaceId={workspaceId ?? null} />
      <MRTopbarButton compact mobile />
      {!isArchived && (
        <TaskTopBarPluginActions
          sessionId={sessionId ?? null}
          taskId={taskId ?? null}
          taskTitle={taskTitle}
          workspaceId={workspaceId ?? null}
        />
      )}
      {isRemoteExecutor && (
        <RemoteExecutorIndicator
          taskId={taskId}
          sessionId={sessionId}
          remoteExecutorType={remoteExecutorType}
          remoteExecutorName={remoteExecutorName}
          remoteState={remoteState}
          remoteCreatedAt={remoteCreatedAt}
          remoteCheckedAt={remoteCheckedAt}
          remoteStatusError={remoteStatusError}
        />
      )}
      {showApproveButton && onApprove && <ApproveButton onApprove={onApprove} />}
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
      <Button
        variant="ghost"
        size="icon-sm"
        className="cursor-pointer"
        onClick={onMenuClick}
        data-testid="mobile-session-menu"
        aria-label="Open task switcher"
      >
        <IconMenu2 className="h-4 w-4" />
      </Button>
    </div>
  );
}

export const SessionMobileTopBar = memo(function SessionMobileTopBar(
  props: SessionMobileTopBarProps,
) {
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [prBranchPushed, setPrBranchPushed] = useState(false);

  const {
    commits,
    displayBranch,
    uncommittedAdditions,
    uncommittedDeletions,
    uncommittedCount,
    totalAdditions,
    totalDeletions,
  } = useMobileGitMetrics(props.sessionId, props.worktreeBranch, props.baseBranch);
  const {
    isGitLoading,
    handlePull,
    handlePush,
    handleRebase,
    handleMerge,
    handleCommit,
    handleCreatePR,
  } = useMobileGitActions(
    props.sessionId,
    props.baseBranch,
    setCommitDialogOpen,
    setPrDialogOpen,
    setPrBranchPushed,
  );

  return (
    <header className="flex items-center justify-between px-2 py-2 bg-background">
      <div className="flex items-center gap-2 min-w-0 flex-1">
        <Button variant="ghost" size="icon-sm" asChild>
          <Link href="/">
            <IconArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <MobileTaskTitle
          taskTitle={props.taskTitle}
          displayBranch={displayBranch}
          totalAdditions={totalAdditions}
          totalDeletions={totalDeletions}
        />
      </div>
      <MobileTopBarActions
        taskId={props.taskId}
        workspaceId={props.workspaceId}
        isRemoteExecutor={props.isRemoteExecutor}
        remoteExecutorType={props.remoteExecutorType}
        remoteExecutorName={props.remoteExecutorName}
        remoteState={props.remoteState}
        remoteCreatedAt={props.remoteCreatedAt}
        remoteCheckedAt={props.remoteCheckedAt}
        remoteStatusError={props.remoteStatusError}
        showApproveButton={props.showApproveButton ?? false}
        onApprove={props.onApprove}
        sessionId={props.sessionId}
        isGitLoading={isGitLoading}
        uncommittedCount={uncommittedCount}
        baseBranch={props.baseBranch}
        taskTitle={props.taskTitle}
        isArchived={props.isArchived}
        onCommitClick={() => setCommitDialogOpen(true)}
        onPRClick={() => {
          setPrBranchPushed(false);
          setPrDialogOpen(true);
        }}
        onPull={handlePull}
        onPush={handlePush}
        onRebase={handleRebase}
        onMerge={handleMerge}
        onMenuClick={props.onMenuClick}
      />
      <MobileGitDialogs
        commitDialogOpen={commitDialogOpen}
        setCommitDialogOpen={setCommitDialogOpen}
        prDialogOpen={prDialogOpen}
        setPrDialogOpen={setPrDialogOpen}
        displayBranch={displayBranch}
        baseBranch={props.baseBranch}
        taskTitle={props.taskTitle}
        firstCommitMessage={commits[0]?.commit_message}
        isGitLoading={isGitLoading}
        branchPushed={prBranchPushed}
        uncommittedCount={uncommittedCount}
        uncommittedAdditions={uncommittedAdditions}
        uncommittedDeletions={uncommittedDeletions}
        onCommit={handleCommit}
        onCreatePR={handleCreatePR}
      />
    </header>
  );
});
