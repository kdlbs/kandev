"use client";

import { memo, useCallback } from "react";
import Link from "next/link";
import { IconBug, IconHome, IconSettings } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@kandev/ui/breadcrumb";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { CommitStatBadge } from "@/components/diff-stat";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { EditorsMenu } from "@/components/task/editors-menu";
import { BranchPathPopover } from "@/components/task/branch-path-popover";
import { LayoutPresetSelector } from "@/components/task/layout-preset-selector";
import { DocumentControls } from "@/components/task/document/document-controls";
import { VcsSplitButton } from "@/components/vcs-split-button";
import { PRTopbarButton } from "@/components/github/pr-topbar-button";
import { PortForwardButton } from "@/components/task/port-forward-dialog";
import { WorkflowStepper, type WorkflowStepperStep } from "@/components/task/workflow-stepper";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { QuickChatButton } from "@/components/task/quick-chat-button";
import { DEBUG_UI } from "@/lib/config";
import { toast } from "sonner";

type TaskTopBarProps = {
  taskId?: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  baseBranch?: string;
  onStartAgent?: (agentProfileId: string) => void;
  onStopAgent?: () => void;
  isAgentRunning?: boolean;
  isAgentLoading?: boolean;
  worktreePath?: string | null;
  worktreeBranch?: string | null;
  repositoryPath?: string | null;
  repositoryName?: string | null;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  workflowSteps?: WorkflowStepperStep[];
  currentStepId?: string | null;
  workflowId?: string | null;
  workspaceId?: string | null;
  isArchived?: boolean;
  isRemoteExecutor?: boolean;
  isAgentctlReady?: boolean;
  remoteExecutorName?: string | null;
  remoteExecutorType?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
};

const TaskTopBar = memo(function TaskTopBar({
  taskId,
  activeSessionId,
  taskTitle,
  baseBranch,
  worktreePath,
  worktreeBranch,
  repositoryPath,
  repositoryName,
  showDebugOverlay,
  onToggleDebugOverlay,
  workflowSteps,
  currentStepId,
  workflowId,
  workspaceId,
  isArchived,
  isRemoteExecutor,
  isAgentctlReady,
  remoteExecutorName,
  remoteExecutorType,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
}: TaskTopBarProps) {
  const git = useSessionGit(activeSessionId);
  // Prefer live git status branch (updates after rename), fallback to session worktree branch
  const displayBranch = git.branch || worktreeBranch || baseBranch;

  // Callback for renaming branch
  const handleRenameBranch = useCallback(
    async (newName: string) => {
      const result = await git.renameBranch(newName);
      if (result.success) {
        toast.success(`Branch renamed to "${newName}"`);
      } else {
        const msg = result.error || "Failed to rename branch";
        toast.error(msg);
        throw new Error(msg);
      }
    },
    [git],
  );

  return (
    <header className="@container/topbar grid grid-cols-[1fr_auto_1fr] items-center px-3 py-1 border-b border-border">
      <TopBarLeft
        taskId={taskId}
        activeSessionId={activeSessionId}
        taskTitle={taskTitle}
        repositoryName={repositoryName}
        displayBranch={displayBranch}
        repositoryPath={repositoryPath}
        worktreePath={worktreePath}
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorName={remoteExecutorName}
        remoteExecutorType={remoteExecutorType}
        remoteState={remoteState}
        remoteCreatedAt={remoteCreatedAt}
        remoteCheckedAt={remoteCheckedAt}
        remoteStatusError={remoteStatusError}
        onRenameBranch={activeSessionId ? handleRenameBranch : undefined}
      />
      {workflowSteps && workflowSteps.length > 0 && (
        <WorkflowStepper
          steps={workflowSteps}
          currentStepId={currentStepId ?? null}
          taskId={taskId ?? null}
          workflowId={workflowId ?? null}
          isArchived={isArchived}
        />
      )}
      <TopBarRight
        activeSessionId={activeSessionId}
        baseBranch={baseBranch}
        gitStatus={git}
        showDebugOverlay={showDebugOverlay}
        onToggleDebugOverlay={onToggleDebugOverlay}
        isArchived={isArchived}
        workspaceId={workspaceId}
        isRemoteExecutor={isRemoteExecutor}
        isAgentctlReady={isAgentctlReady}
      />
    </header>
  );
});

/** Left section: breadcrumbs, branch pill */
function TopBarLeft({
  taskId,
  activeSessionId,
  taskTitle,
  repositoryName,
  displayBranch,
  repositoryPath,
  worktreePath,
  isRemoteExecutor,
  remoteExecutorName,
  remoteExecutorType,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
  onRenameBranch,
}: {
  taskId?: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  repositoryName?: string | null;
  displayBranch?: string;
  repositoryPath?: string | null;
  worktreePath?: string | null;
  isRemoteExecutor?: boolean;
  remoteExecutorName?: string | null;
  remoteExecutorType?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
  onRenameBranch?: (newName: string) => Promise<void>;
}) {
  return (
    <div className="flex items-center gap-2.5 min-w-0 overflow-hidden">
      <Breadcrumb className="min-w-0">
        <BreadcrumbList className="flex-nowrap text-sm min-w-0">
          <BreadcrumbItem className="shrink-0">
            <BreadcrumbLink asChild>
              <Link
                href="/"
                className="cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
              >
                <IconHome className="h-4 w-4" />
              </Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          {repositoryName && (
            <>
              <BreadcrumbSeparator className="shrink-0 @max-[900px]/topbar:hidden" />
              <BreadcrumbItem className="min-w-0 @max-[900px]/topbar:hidden">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="text-muted-foreground truncate block">{repositoryName}</span>
                  </TooltipTrigger>
                  <TooltipContent>{repositoryName}</TooltipContent>
                </Tooltip>
              </BreadcrumbItem>
            </>
          )}
          <BreadcrumbSeparator className="shrink-0" />
          <BreadcrumbItem className="min-w-0">
            <Tooltip>
              <TooltipTrigger asChild>
                <BreadcrumbPage className="font-medium truncate">
                  {taskTitle ?? "Task details"}
                </BreadcrumbPage>
              </TooltipTrigger>
              <TooltipContent>{taskTitle ?? "Task details"}</TooltipContent>
            </Tooltip>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <div className="shrink-0 @max-[1352px]/topbar:hidden">
        <BranchPathPopover
          displayBranch={displayBranch}
          repositoryPath={repositoryPath}
          worktreePath={worktreePath}
          onRenameBranch={onRenameBranch}
        />
      </div>

      {isRemoteExecutor && (
        <RemoteCloudTooltip
          taskId={taskId ?? ""}
          sessionId={activeSessionId}
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
      )}
    </div>
  );
}

/** Ahead/Behind commit status badges */
function GitAheadBehindBadges({
  gitStatus,
}: {
  gitStatus: { ahead: number; behind: number; remote_branch?: string | null };
}) {
  const ahead = gitStatus?.ahead ?? 0;
  const behind = gitStatus?.behind ?? 0;
  if (ahead === 0 && behind === 0) return null;
  const remoteBranch = gitStatus?.remote_branch || "remote";
  return (
    <div className="flex items-center gap-1">
      {ahead > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="cursor-default">
              <CommitStatBadge label={`${ahead} ahead`} tone="ahead" />
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {ahead} commit{ahead !== 1 ? "s" : ""} ahead of {remoteBranch}
          </TooltipContent>
        </Tooltip>
      )}
      {behind > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="cursor-default">
              <CommitStatBadge label={`${behind} behind`} tone="behind" />
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {behind} commit{behind !== 1 ? "s" : ""} behind {remoteBranch}
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

/** Right section: git badges, debug toggle, document controls, editors, VCS, settings */
function TopBarRight({
  activeSessionId,
  baseBranch,
  gitStatus,
  showDebugOverlay,
  onToggleDebugOverlay,
  isArchived,
  workspaceId,
  isRemoteExecutor,
  isAgentctlReady,
}: {
  activeSessionId?: string | null;
  baseBranch?: string;
  gitStatus: { ahead: number; behind: number; remote_branch?: string | null };
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  isArchived?: boolean;
  workspaceId?: string | null;
  isRemoteExecutor?: boolean;
  isAgentctlReady?: boolean;
}) {
  return (
    <div className="flex items-center gap-2 justify-end">
      {DEBUG_UI && onToggleDebugOverlay && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant={showDebugOverlay ? "default" : "outline"}
              className="cursor-pointer px-2"
              onClick={onToggleDebugOverlay}
            >
              <IconBug className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {showDebugOverlay ? "Hide Debug Info" : "Show Debug Info"}
          </TooltipContent>
        </Tooltip>
      )}
      <GitAheadBehindBadges gitStatus={gitStatus} />
      <DocumentControls activeSessionId={activeSessionId ?? null} />
      {!isArchived && (
        <>
          <PortForwardButton
            isRemoteExecutor={isRemoteExecutor}
            sessionId={activeSessionId}
            isAgentctlReady={isAgentctlReady}
          />
          <PRTopbarButton />
          <QuickChatButton workspaceId={workspaceId} />
          <VcsSplitButton sessionId={activeSessionId ?? null} baseBranch={baseBranch} />
          <LayoutPresetSelector />
          <EditorsMenu activeSessionId={activeSessionId ?? null} />
        </>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button size="sm" variant="outline" className="cursor-pointer px-2" asChild>
            <Link href="/settings/general">
              <IconSettings className="h-4 w-4" />
            </Link>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Settings</TooltipContent>
      </Tooltip>
    </div>
  );
}

export { TaskTopBar };
