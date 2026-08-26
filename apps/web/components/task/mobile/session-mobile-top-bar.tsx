"use client";

import { memo } from "react";
import Link from "@/components/routing/app-link";
import { IconArrowLeft, IconMenu2, IconGitBranch, IconCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { LineStat } from "@/components/diff-stat";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import type { FileInfo } from "@/lib/state/slices";
import { TaskTopBarPluginActions } from "@/components/task/task-top-bar-plugin-actions";
import { MRTopbarButton } from "@/components/gitlab/mr-topbar-button";
import { PortForwardButton } from "@/components/task/port-forward-dialog";
import { linkToTaskOverview } from "@/lib/links";
import { useTranslation } from "react-i18next";

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

function computeUncommittedStats(files: Record<string, FileInfo> | undefined) {
  let additions = 0;
  let deletions = 0;
  for (const file of Object.values(files ?? {})) {
    additions += file.additions || 0;
    deletions += file.deletions || 0;
  }
  return { additions, deletions };
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
    displayBranch: worktreeBranch || baseBranch,
    totalAdditions: stats.additions + commits.reduce((sum, commit) => sum + commit.insertions, 0),
    totalDeletions: stats.deletions + commits.reduce((sum, commit) => sum + commit.deletions, 0),
  };
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
  taskTitle?: string;
  isArchived?: boolean;
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
  taskTitle,
  isArchived,
  onMenuClick,
}: MobileTopBarActionsProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1" data-testid="mobile-topbar-actions">
      <MRTopbarButton compact mobile />
      {!isArchived && <PortForwardButton sessionId={sessionId} />}
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
      <Button
        variant="ghost"
        size="icon-sm"
        className="h-11 w-11 cursor-pointer"
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
  const { displayBranch, totalAdditions, totalDeletions } = useMobileGitMetrics(
    props.sessionId,
    props.worktreeBranch,
    props.baseBranch,
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
        taskTitle={props.taskTitle}
        isArchived={props.isArchived}
        onMenuClick={props.onMenuClick}
      />
    </header>
  );
});
