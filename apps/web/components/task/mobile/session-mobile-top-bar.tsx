"use client";

import { memo, useCallback, useMemo, useState } from "react";
import Link from "@/components/routing/app-link";
import { IconArrowLeft, IconMenu2, IconGitBranch, IconCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { LineStat } from "@/components/diff-stat";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useRemoteContributionRelation } from "@/hooks/domains/session/use-remote-contribution-relation";
import {
  remoteContributionActionPolicy,
  remoteContributionActionReasonKey,
} from "@/hooks/domains/session/remote-contribution-relation";
import {
  CommitDialog,
  PRDialog,
  GitActionsDropdown,
  computeUncommittedStats,
  useMobileGitActions,
} from "./session-mobile-top-bar-git-controls";
import { TaskTopBarPluginActions } from "@/components/task/task-top-bar-plugin-actions";
import { LayoutPresetSelector } from "@/components/task/layout-preset-selector";
import { MRTopbarButton } from "@/components/gitlab/mr-topbar-button";
import { PortForwardButton } from "@/components/task/port-forward-dialog";
import { linkToTaskOverview } from "@/lib/links";
import { useTranslation } from "react-i18next";
import { openExternalLink } from "@/lib/desktop/external-links";
import {
  buildRemoteContributionResolutionTarget,
  useRemoteContributionResolution,
  useRemoteContributionResolutionConfirmation,
  type RemoteContributionResolutionTarget,
} from "../use-remote-contribution-resolution";
import { MobileContributionResolutionDrawer } from "./mobile-contribution-resolution-drawer";

type SessionMobileTopBarProps = {
  taskId?: string | null;
  workspaceId?: string | null;
  taskTitle?: string;
  /** `owner/repo` (or the repository name) of the task's primary repository. */
  repositoryLabel?: string | null;
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
  repositoryLabel,
  displayBranch,
  totalAdditions,
  totalDeletions,
}: {
  taskTitle?: string;
  repositoryLabel?: string | null;
  displayBranch?: string;
  totalAdditions: number;
  totalDeletions: number;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col min-w-0 flex-1">
      <span className="text-sm font-medium truncate">{taskTitle ?? t("task:taskDetails")}</span>
      {/* The phone bar has no breadcrumb, so the repository rides the same
          secondary line as the branch. It shrinks first: on a phone the branch
          and diff stats are the denser signal, and the full name stays in
          `title`. */}
      <div className="flex items-center gap-1.5 min-w-0">
        {repositoryLabel && (
          <span
            data-testid="mobile-task-repository"
            title={repositoryLabel}
            className="min-w-0 truncate text-xs text-muted-foreground/70"
          >
            {repositoryLabel}
          </span>
        )}
        {displayBranch && (
          <>
            <IconGitBranch className="h-3 w-3 shrink-0 text-muted-foreground" />
            <span className="shrink-0 max-w-[45%] truncate text-xs text-muted-foreground">
              {displayBranch}
            </span>
            {(totalAdditions > 0 || totalDeletions > 0) && (
              <LineStat added={totalAdditions} removed={totalDeletions} />
            )}
          </>
        )}
      </div>
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
  const { t } = useTranslation();
  return (
    <Button
      size="sm"
      className="h-7 gap-1 px-2 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white text-xs"
      onClick={onApprove}
    >
      <IconCheck className="h-3.5 w-3.5" />
      {t("task:approve")}
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

function useMobileRemoteActionPolicy(sessionId: string | null | undefined) {
  const contribution = useRemoteContributionRelation(sessionId);
  return {
    ...contribution,
    ...remoteContributionActionPolicy(contribution.relation),
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

function useMobileContributionResolutionActions(sessionId: string | null | undefined) {
  const { t } = useTranslation();
  const remoteActionPolicy = useMobileRemoteActionPolicy(sessionId);
  const resolution = useRemoteContributionResolution(
    sessionId,
    remoteActionPolicy.refreshProviderEvidence,
  );
  const remoteRepositoryLabel = t("task:remoteRepository");
  const resolutionTarget = useMemo(
    () =>
      buildRemoteContributionResolutionTarget(
        remoteActionPolicy.relation,
        remoteActionPolicy.repositoryName,
        remoteActionPolicy.selectedPR,
        remoteRepositoryLabel,
      ),
    [
      remoteActionPolicy.relation,
      remoteActionPolicy.repositoryName,
      remoteActionPolicy.selectedPR,
      remoteRepositoryLabel,
    ],
  );
  const requestReplace = useCallback(() => {
    if (resolutionTarget) resolution.requestReplace(resolutionTarget);
  }, [resolution, resolutionTarget]);
  const requestUse = useCallback(() => {
    if (resolutionTarget) resolution.requestUse(resolutionTarget);
  }, [resolution, resolutionTarget]);
  const viewPRVersion = useCallback(() => {
    const url = remoteActionPolicy.selectedPR?.pr_url;
    if (url) void openExternalLink(url).catch(() => undefined);
  }, [remoteActionPolicy.selectedPR?.pr_url]);
  const confirmResolution = useRemoteContributionResolutionConfirmation(resolution);

  return {
    remoteActionPolicy,
    resolution,
    resolutionTarget,
    requestReplace,
    requestUse,
    viewPRVersion,
    confirmResolution,
  };
}

function MobileResolutionDrawer({
  resolution,
  resolutionTarget,
  confirmResolution,
}: {
  resolution: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget: RemoteContributionResolutionTarget | null;
  confirmResolution: () => Promise<void>;
}) {
  if (!resolution.pending || !resolutionTarget) return null;
  return (
    <MobileContributionResolutionDrawer
      open
      action={resolution.pending.action}
      repositoryName={resolutionTarget.repositoryName ?? ""}
      expectedRemoteHead={resolution.pending.expectedRemoteHead}
      isLoading={resolution.isLoading}
      errorKey={resolution.errorKey}
      onOpenChange={(open) => {
        if (!open) resolution.cancel();
      }}
      onConfirm={confirmResolution}
    />
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

type MobileTopBarGitActionsProps = Pick<
  MobileTopBarActionsProps,
  | "sessionId"
  | "isGitLoading"
  | "uncommittedCount"
  | "baseBranch"
  | "onCommitClick"
  | "onPRClick"
  | "onPull"
  | "onPush"
  | "onRebase"
  | "onMerge"
>;

function MobileTopBarGitActions(props: MobileTopBarGitActionsProps) {
  const { t } = useTranslation();
  const {
    remoteActionPolicy,
    resolution,
    resolutionTarget,
    requestReplace,
    requestUse,
    viewPRVersion,
    confirmResolution,
  } = useMobileContributionResolutionActions(props.sessionId);
  const pushDisabledReasonKey = remoteContributionActionReasonKey(
    remoteActionPolicy.relation,
    "push",
  );
  const pullDisabledReasonKey = remoteContributionActionReasonKey(
    remoteActionPolicy.relation,
    "pull",
  );
  return (
    <>
      <GitActionsDropdown
        sessionId={props.sessionId}
        isGitLoading={props.isGitLoading}
        uncommittedCount={props.uncommittedCount}
        baseBranch={props.baseBranch}
        onCommitClick={props.onCommitClick}
        onPRClick={props.onPRClick}
        onPull={props.onPull}
        onPush={props.onPush}
        onRebase={props.onRebase}
        onMerge={props.onMerge}
        pushDisabled={remoteActionPolicy.pushDisabled}
        pullDisabled={remoteActionPolicy.pullDisabled}
        pushDisabledReason={pushDisabledReasonKey ? t(pushDisabledReasonKey) : undefined}
        pullDisabledReason={pullDisabledReasonKey ? t(pullDisabledReasonKey) : undefined}
        showContributionResolution={remoteActionPolicy.action === "diverged_replace"}
        replaceDisabled={remoteActionPolicy.replaceDisabled}
        useDisabled={remoteActionPolicy.useDisabled}
        onReplaceContribution={requestReplace}
        onUseContribution={requestUse}
        onViewPRVersion={viewPRVersion}
        prNumber={remoteActionPolicy.selectedPR?.pr_number}
      />
      <MobileResolutionDrawer
        resolution={resolution}
        resolutionTarget={resolutionTarget}
        confirmResolution={confirmResolution}
      />
    </>
  );
}

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
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1" data-testid="mobile-topbar-actions">
      <MRTopbarButton compact mobile />
      {!isArchived && <PortForwardButton sessionId={sessionId} />}
      {!isArchived && <LayoutPresetSelector mobile />}
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
      <MobileTopBarGitActions
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
        aria-label={t("task:openTaskSwitcher")}
      >
        <IconMenu2 className="h-4 w-4" />
      </Button>
    </div>
  );
}

export const SessionMobileTopBar = memo(function SessionMobileTopBar(
  props: SessionMobileTopBarProps,
) {
  const { t } = useTranslation();
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
          <Link
            href={linkToTaskOverview({ workspaceId: props.workspaceId ?? undefined })}
            aria-label={t("task:taskOverview")}
          >
            <IconArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <MobileTaskTitle
          taskTitle={props.taskTitle}
          repositoryLabel={props.repositoryLabel}
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
