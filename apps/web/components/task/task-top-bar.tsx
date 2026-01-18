'use client';

import { memo, useState } from 'react';
import Link from 'next/link';
import {
  IconArrowLeft,
  IconArrowUp,
  IconCopy,
  IconDots,
  IconFolderOpen,
  IconGitBranch,
  IconGitMerge,
  IconGitPullRequest,
  IconChevronDown,
  IconCheck,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { Popover, PopoverContent, PopoverTrigger } from '@kandev/ui/popover';
import { CommitStatBadge, LineStat } from '@/components/diff-stat';
import { useSessionGitStatus } from '@/hooks/use-session-git-status';
import { formatUserHomePath } from '@/lib/utils';
import { EditorsMenu } from '@/components/task/editors-menu';

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
};

const TaskTopBar = memo(function TaskTopBar({
  activeSessionId,
  taskTitle,
  baseBranch,
  worktreePath,
  worktreeBranch,
  repositoryPath,
  repositoryName,
}: TaskTopBarProps) {
  const [copiedBranch, setCopiedBranch] = useState(false);
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [copiedRepo, setCopiedRepo] = useState(false);
  const [copiedWorktree, setCopiedWorktree] = useState(false);

  const gitStatus = useSessionGitStatus(activeSessionId ?? null);

  // Use worktree branch if available, otherwise fall back to base branch
  const displayBranch = worktreeBranch || baseBranch;

  // Calculate total additions and deletions from all files.
  let totalAdditions = 0;
  let totalDeletions = 0;
  if (gitStatus?.files && Object.keys(gitStatus.files).length > 0) {
    for (const file of Object.values(gitStatus.files)) {
      totalAdditions += file.additions || 0;
      totalDeletions += file.deletions || 0;
    }
  }

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
    <header className="flex items-center justify-between p-3">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/">
            <IconArrowLeft className="h-4 w-4" />
            Back
          </Link>
        </Button>
        {repositoryName && (
          <>
            <span className="text-sm text-muted-foreground">{repositoryName}</span>
            <span className="text-sm text-muted-foreground">›</span>
          </>
        )}
        <span className="text-sm font-medium">{taskTitle ?? 'Task details'}</span>
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
                        className="absolute right-1 top-1 p-1 rounded bg-card/80 backdrop-blur-sm hover:bg-card transition-all shadow-sm"
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
                        className="absolute right-1 top-1 p-1 rounded bg-card/80 backdrop-blur-sm hover:bg-card transition-all shadow-sm"
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

        {/* Git Status: Lines Changed */}
        {(totalAdditions > 0 || totalDeletions > 0) && (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-default">
                <LineStat added={totalAdditions} removed={totalDeletions} />
              </span>
            </TooltipTrigger>
            <TooltipContent>Lines changed</TooltipContent>
          </Tooltip>
        )}

      </div>
      <div className="flex items-center gap-2">
        <EditorsMenu activeSessionId={activeSessionId ?? null} />

        {/* Create PR Split Button */}
        <div className="inline-flex rounded-md border border-border overflow-hidden">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="outline"
                className="rounded-none border-0 cursor-pointer"
                onClick={() => {
                  // TODO: Implement create PR
                  console.log('Create PR');
                }}
              >
                <IconGitPullRequest className="h-4 w-4" />
                Create PR
              </Button>
            </TooltipTrigger>
            <TooltipContent>Create pull request</TooltipContent>
          </Tooltip>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="outline" className="rounded-none border-0 border-l px-2 cursor-pointer">
                <IconChevronDown className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                className="cursor-pointer"
                onClick={() => {
                  // TODO: Implement pull
                  console.log('Pull from remote');
                }}
              >
                <IconArrowUp className="h-4 w-4 rotate-180" />
                Pull
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer"
                onClick={() => {
                  // TODO: Implement rebase
                  console.log('Rebase');
                }}
              >
                <IconGitBranch className="h-4 w-4" />
                Rebase
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer"
                onClick={() => {
                  // TODO: Implement push
                  console.log('Push to remote');
                }}
              >
                <IconArrowUp className="h-4 w-4" />
                Push
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer"
                onClick={() => {
                  // TODO: Implement merge
                  console.log('Merge');
                }}
              >
                <IconGitMerge className="h-4 w-4" />
                Merge
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        {/* More Options (3-dot) Dropdown */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm" variant="outline" className="cursor-pointer px-2">
              <IconDots className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-[220px]">
            <DropdownMenuItem
              className="cursor-pointer"
              onClick={() => {
                if (worktreePath) {
                  navigator.clipboard?.writeText(worktreePath);
                }
              }}
              disabled={!worktreePath}
            >
              <IconCopy className="h-4 w-4" />
              Copy workspace path
            </DropdownMenuItem>
            <DropdownMenuItem
              className="cursor-pointer"
              onClick={() => {
                // TODO: Implement open workspace folder
                console.log('Open workspace folder:', worktreePath);
              }}
              disabled={!worktreePath}
            >
              <IconFolderOpen className="h-4 w-4" />
              Open workspace folder
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
});

export { TaskTopBar };
