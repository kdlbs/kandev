'use client';

import { memo, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import {
  IconBug,
  IconCopy,
  IconGitBranch,
  IconCheck,
  IconHome,
  IconSettings,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@kandev/ui/breadcrumb';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { Popover, PopoverContent, PopoverTrigger } from '@kandev/ui/popover';
import { CommitStatBadge } from '@/components/diff-stat';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { formatUserHomePath } from '@/lib/utils';
import { EditorsMenu } from '@/components/task/editors-menu';
import { DocumentControls } from '@/components/task/document/document-controls';
import { VcsSplitButton } from '@/components/vcs-split-button';
import { WorkflowStepper, type WorkflowStepperStep } from '@/components/task/workflow-stepper';
import { DEBUG_UI } from '@/lib/config';

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
}: TaskTopBarProps) {
  const [copiedBranch, setCopiedBranch] = useState(false);
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [copiedRepo, setCopiedRepo] = useState(false);
  const [copiedWorktree, setCopiedWorktree] = useState(false);

  const router = useRouter();
  const gitStatus = useSessionGitStatus(activeSessionId ?? null);
  // Use worktree branch if available, otherwise fall back to base branch
  const displayBranch = worktreeBranch || baseBranch;

  const handleBranchClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (displayBranch) {
      navigator.clipboard?.writeText(displayBranch);
      setCopiedBranch(true);
      setTimeout(() => setCopiedBranch(false), 500);
    }
  };

  const handleCopyRepo = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (repositoryPath) {
      navigator.clipboard?.writeText(formatUserHomePath(repositoryPath));
      setCopiedRepo(true);
      setTimeout(() => setCopiedRepo(false), 500);
    }
  };

  const handleCopyWorktree = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (worktreePath) {
      navigator.clipboard?.writeText(formatUserHomePath(worktreePath));
      setCopiedWorktree(true);
      setTimeout(() => setCopiedWorktree(false), 500);
    }
  };

  return (
    <header className="grid grid-cols-[1fr_auto_1fr] items-center px-3 py-1 border-b border-border">
      <div className="flex items-center gap-2.5 min-w-0">
        <Breadcrumb>
          <BreadcrumbList className="flex-nowrap text-sm">
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link href="/" className="text-muted-foreground hover:text-foreground transition-colors">
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
              <BreadcrumbPage className="font-medium">
                {taskTitle ?? 'Task details'}
              </BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        {displayBranch && (
          <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
            <Tooltip>
              <TooltipTrigger asChild>
                <PopoverTrigger asChild>
                  <div className="group flex items-center gap-1.5 rounded-md px-2 h-7 bg-muted/40 hover:bg-muted/60 cursor-pointer transition-colors">
                    <IconGitBranch className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="text-xs text-muted-foreground">{displayBranch}</span>
                    <button
                      type="button"
                      onClick={handleBranchClick}
                      className="opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer ml-0.5"
                    >
                      {copiedBranch ? (
                        <IconCheck className="h-3 w-3 text-green-500" />
                      ) : (
                        <IconCopy className="h-3 w-3 text-muted-foreground hover:text-foreground" />
                      )}
                    </button>
                  </div>
                </PopoverTrigger>
              </TooltipTrigger>
              <TooltipContent side="right">Current branch</TooltipContent>
            </Tooltip>
            <PopoverContent
              side="bottom"
              sideOffset={5}
              className="p-0 w-auto max-w-[600px] gap-1"
            >
              <div className="px-2 pt-1 pb-2 space-y-1.5">
                {repositoryPath && (
                  <div className="space-y-0.5">
                    <div className="text-xs text-muted-foreground">Repository</div>
                    <div className="relative group/repo overflow-hidden">
                      <div className="text-xs text-muted-foreground bg-muted/50 px-2 py-1.5 pr-9 rounded-sm select-text cursor-text whitespace-nowrap">
                        {formatUserHomePath(repositoryPath)}
                      </div>
                      <button
                        type="button"
                        onClick={handleCopyRepo}
                        className="absolute right-1 top-1 p-1 rounded bg-background/80 backdrop-blur-sm hover:bg-background transition-all shadow-sm"
                      >
                        {copiedRepo ? (
                          <IconCheck className="h-3 w-3 text-green-500" />
                        ) : (
                          <IconCopy className="h-3 w-3 text-muted-foreground" />
                        )}
                      </button>
                    </div>
                  </div>
                )}
                {worktreePath && (
                  <div className="space-y-0.5">
                    <div className="text-xs text-muted-foreground">Worktree</div>
                    <div className="relative group/worktree overflow-hidden">
                      <div className="text-xs bg-muted/50 px-2 py-1.5 pr-9 rounded-sm select-text cursor-text whitespace-nowrap text-muted-foreground">
                        {formatUserHomePath(worktreePath)}
                      </div>
                      <button
                        type="button"
                        onClick={handleCopyWorktree}
                        className="absolute right-1 top-1 p-1 rounded bg-background/80 backdrop-blur-sm hover:bg-background transition-all shadow-sm"
                      >
                        {copiedWorktree ? (
                          <IconCheck className="h-3 w-3 text-green-500" />
                        ) : (
                          <IconCopy className="h-3 w-3 text-muted-foreground" />
                        )}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </PopoverContent>
          </Popover>
        )}

        {/* Git Status: Ahead/Behind */}
        {((gitStatus?.ahead ?? 0) > 0 || (gitStatus?.behind ?? 0) > 0) && (
          <div className="flex items-center gap-1">
            {(gitStatus?.ahead ?? 0) > 0 && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="cursor-default">
                    <CommitStatBadge label={`${gitStatus?.ahead ?? 0} ahead`} tone="ahead" />
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  {gitStatus?.ahead ?? 0} commit{(gitStatus?.ahead ?? 0) !== 1 ? 's' : ''} ahead of {gitStatus?.remote_branch || 'remote'}
                </TooltipContent>
              </Tooltip>
            )}
            {(gitStatus?.behind ?? 0) > 0 && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="cursor-default">
                    <CommitStatBadge label={`${gitStatus?.behind ?? 0} behind`} tone="behind" />
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  {gitStatus?.behind ?? 0} commit{(gitStatus?.behind ?? 0) !== 1 ? 's' : ''} behind {gitStatus?.remote_branch || 'remote'}
                </TooltipContent>
              </Tooltip>
            )}
          </div>
        )}

      </div>
      {workflowSteps && workflowSteps.length > 0 && (
        <WorkflowStepper
          steps={workflowSteps}
          currentStepId={currentStepId ?? null}
          taskId={taskId ?? null}
          workflowId={workflowId ?? null}
          isArchived={isArchived}
        />
      )}
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
              {showDebugOverlay ? 'Hide Debug Info' : 'Show Debug Info'}
            </TooltipContent>
          </Tooltip>
        )}
        <DocumentControls activeSessionId={activeSessionId ?? null} />
        {!isArchived && (
          <>
            <EditorsMenu activeSessionId={activeSessionId ?? null} />
            <VcsSplitButton
              sessionId={activeSessionId ?? null}
              baseBranch={baseBranch}
              taskTitle={taskTitle}
              displayBranch={displayBranch}
            />
          </>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="outline"
              className="cursor-pointer px-2"
              onClick={() => router.push('/settings/general')}
            >
              <IconSettings className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Settings</TooltipContent>
        </Tooltip>
      </div>

    </header>
  );
});

export { TaskTopBar };
