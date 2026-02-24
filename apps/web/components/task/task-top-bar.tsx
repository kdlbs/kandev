"use client";

import { memo, useState } from "react";
import Link from "next/link";
import {
  IconBug,
  IconCopy,
  IconGitBranch,
  IconCheck,
  IconCloud,
  IconCloudOff,
  IconHome,
  IconSettings,
} from "@tabler/icons-react";
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
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { CommitStatBadge } from "@/components/diff-stat";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { formatUserHomePath } from "@/lib/utils";
import { EditorsMenu } from "@/components/task/editors-menu";
import { LayoutPresetSelector } from "@/components/task/layout-preset-selector";
import { DocumentControls } from "@/components/task/document/document-controls";
import { VcsSplitButton } from "@/components/vcs-split-button";
import { PRTopbarButton } from "@/components/github/pr-topbar-button";
import { WorkflowStepper, type WorkflowStepperStep } from "@/components/task/workflow-stepper";
import { DEBUG_UI } from "@/lib/config";

type TaskTopBarProps = {
  taskId?: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  taskDescription?: string;
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
  isArchived?: boolean;
  isRemoteExecutor?: boolean;
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
  isArchived,
  isRemoteExecutor,
  remoteExecutorName,
  remoteExecutorType,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
}: TaskTopBarProps) {
  const git = useSessionGit(activeSessionId);
  const displayBranch = worktreeBranch || baseBranch;

  return (
    <header className="grid grid-cols-[1fr_auto_1fr] items-center px-3 py-1 border-b border-border">
      <TopBarLeft
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
      />
    </header>
  );
});

/** Copies text to clipboard with a brief "copied" state */
function useCopyToClipboard(): [boolean, (text: string) => void] {
  const [copied, setCopied] = useState(false);
  const handleCopy = (text: string) => {
    navigator.clipboard?.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 500);
  };
  return [copied, handleCopy];
}

/** Copy button with check/copy icon toggle */
function CopyIconButton({
  copied,
  onClick,
  className,
}: {
  copied: boolean;
  onClick: (e: React.MouseEvent) => void;
  className?: string;
}) {
  return (
    <button type="button" onClick={onClick} className={className}>
      {copied ? (
        <IconCheck className="h-3 w-3 text-green-500" />
      ) : (
        <IconCopy className="h-3 w-3 text-muted-foreground hover:text-foreground" />
      )}
    </button>
  );
}

/** Copiable path row used in the branch popover */
function PathRow({ label, path }: { label: string; path: string }) {
  const [copied, handleCopy] = useCopyToClipboard();
  return (
    <div className="space-y-0.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="relative group/row overflow-hidden">
        <div className="text-xs text-muted-foreground bg-muted/50 px-2 py-1.5 pr-9 rounded-sm select-text cursor-text whitespace-nowrap">
          {formatUserHomePath(path)}
        </div>
        <CopyIconButton
          copied={copied}
          onClick={(e) => {
            e.stopPropagation();
            handleCopy(formatUserHomePath(path));
          }}
          className="absolute right-1 top-1 p-1 rounded bg-background/80 backdrop-blur-sm hover:bg-background transition-all shadow-sm"
        />
      </div>
    </div>
  );
}

function BranchPathPopover({
  displayBranch,
  repositoryPath,
  worktreePath,
}: {
  displayBranch?: string;
  repositoryPath?: string | null;
  worktreePath?: string | null;
}) {
  const [copiedBranch, handleCopyBranch] = useCopyToClipboard();
  const [popoverOpen, setPopoverOpen] = useState(false);
  if (!displayBranch) return null;

  return (
    <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <div className="group flex items-center gap-1.5 rounded-md px-2 h-7 bg-muted/40 hover:bg-muted/60 cursor-pointer transition-colors">
              <IconGitBranch className="h-3.5 w-3.5 text-muted-foreground" />
              <span className="text-xs text-muted-foreground">{displayBranch}</span>
              <CopyIconButton
                copied={copiedBranch}
                onClick={(e) => {
                  e.stopPropagation();
                  handleCopyBranch(displayBranch);
                }}
                className="opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer ml-0.5"
              />
            </div>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent side="right">Current branch</TooltipContent>
      </Tooltip>
      <PopoverContent side="bottom" sideOffset={5} className="p-0 w-auto max-w-[600px] gap-1">
        <div className="px-2 pt-1 pb-2 space-y-1.5">
          {repositoryPath && <PathRow label="Repository" path={repositoryPath} />}
          {worktreePath && <PathRow label="Worktree" path={worktreePath} />}
        </div>
      </PopoverContent>
    </Popover>
  );
}

function RemoteExecutorIndicator({
  isRemoteExecutor,
  remoteExecutorName,
  remoteExecutorType,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
}: {
  isRemoteExecutor?: boolean;
  remoteExecutorName?: string | null;
  remoteExecutorType?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
}) {
  if (!isRemoteExecutor) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center">
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

/** Left section: breadcrumbs, branch pill */
function TopBarLeft({
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
}: {
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
}) {
  return (
    <div className="flex items-center gap-2.5 min-w-0">
      <Breadcrumb>
        <BreadcrumbList className="flex-nowrap text-sm">
          <BreadcrumbItem>
            <BreadcrumbLink asChild>
              <Link
                href="/"
                className="text-muted-foreground hover:text-foreground transition-colors"
              >
                <IconHome className="h-4 w-4" />
              </Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          {repositoryName && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <span className="text-muted-foreground">{repositoryName}</span>
              </BreadcrumbItem>
            </>
          )}
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage className="font-medium">{taskTitle ?? "Task details"}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <BranchPathPopover
        displayBranch={displayBranch}
        repositoryPath={repositoryPath}
        worktreePath={worktreePath}
      />

      <RemoteExecutorIndicator
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorName={remoteExecutorName}
        remoteExecutorType={remoteExecutorType}
        remoteState={remoteState}
        remoteCreatedAt={remoteCreatedAt}
        remoteCheckedAt={remoteCheckedAt}
        remoteStatusError={remoteStatusError}
      />
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
}: {
  activeSessionId?: string | null;
  baseBranch?: string;
  gitStatus: { ahead: number; behind: number; remote_branch?: string | null };
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  isArchived?: boolean;
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
          <PRTopbarButton />
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
