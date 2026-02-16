'use client';

import { memo, useCallback } from 'react';
import {
  IconGitCommit,
  IconGitPullRequest,
  IconGitMerge,
  IconGitCherryPick,
  IconLoader2,
  IconChevronDown,
  IconCloudDownload,
  IconCloudUpload,
  IconAlertTriangle,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
} from '@kandev/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useGitOperations } from '@/hooks/use-git-operations';
import { useGitWithFeedback } from '@/hooks/use-git-with-feedback';
import { useVcsDialogs } from '@/components/vcs/vcs-dialogs';

type VcsSplitButtonProps = {
  sessionId: string | null;
  baseBranch?: string;
};

const VcsSplitButton = memo(function VcsSplitButton({
  sessionId,
  baseBranch,
}: VcsSplitButtonProps) {
  const gitWithFeedback = useGitWithFeedback();
  const gitStatus = useSessionGitStatus(sessionId);
  const { pull, push, rebase, merge, isLoading: isGitLoading } =
    useGitOperations(sessionId);
  const { openCommitDialog, openPRDialog } = useVcsDialogs();

  // Check if remote branch tracks current branch
  const currentBranch = gitStatus?.branch;
  const remoteBranch = gitStatus?.remote_branch;
  const hasMatchingUpstream =
    remoteBranch && currentBranch && remoteBranch === `origin/${currentBranch}`;

  // Uncommitted file stats
  const uncommittedFileCount = gitStatus?.files
    ? Object.keys(gitStatus.files).length
    : 0;

  const aheadCount = gitStatus?.ahead ?? 0;
  const behindCount = gitStatus?.behind ?? 0;

  // --- handlers ---

  const handlePull = useCallback(() => {
    gitWithFeedback(() => pull(), 'Pull');
  }, [gitWithFeedback, pull]);

  const handlePush = useCallback(
    (force = false) => {
      gitWithFeedback(
        () => push({ force }),
        force ? 'Force Push' : 'Push'
      );
    },
    [gitWithFeedback, push]
  );

  const handleRebase = useCallback(() => {
    const targetBranch = baseBranch?.replace(/^origin\//, '') || 'main';
    gitWithFeedback(() => rebase(targetBranch), 'Rebase');
  }, [gitWithFeedback, rebase, baseBranch]);

  const handleMerge = useCallback(() => {
    const targetBranch = baseBranch?.replace(/^origin\//, '') || 'main';
    gitWithFeedback(() => merge(targetBranch), 'Merge');
  }, [gitWithFeedback, merge, baseBranch]);

  // --- dynamic primary action ---

  const primaryAction =
    uncommittedFileCount > 0
      ? ('commit' as const)
      : aheadCount > 0
        ? ('pr' as const)
        : behindCount > 0
          ? ('rebase' as const)
          : ('commit' as const);

  const primaryButtonConfig = {
    commit: {
      icon: <IconGitCommit className="h-4 w-4" />,
      label: 'Commit',
      badge: uncommittedFileCount > 0 ? uncommittedFileCount : null,
      tooltip:
        uncommittedFileCount > 0
          ? `Commit ${uncommittedFileCount} changed file${uncommittedFileCount !== 1 ? 's' : ''}`
          : 'No changes to commit',
      onClick: openCommitDialog,
    },
    pr: {
      icon: <IconGitPullRequest className="h-4 w-4" />,
      label: 'Create PR',
      badge: aheadCount > 0 ? aheadCount : null,
      tooltip: `Create PR (${aheadCount} commit${aheadCount !== 1 ? 's' : ''} ahead)`,
      onClick: openPRDialog,
    },
    rebase: {
      icon: <IconGitCherryPick className="h-4 w-4" />,
      label: 'Rebase',
      badge: behindCount > 0 ? behindCount : null,
      tooltip: `Rebase onto ${baseBranch || 'origin/main'} (${behindCount} behind)`,
      onClick: handleRebase,
    },
  }[primaryAction];

  return (
    <div className="inline-flex rounded-md border border-border overflow-hidden">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="rounded-none border-0 cursor-pointer"
            onClick={primaryButtonConfig.onClick}
            disabled={isGitLoading || !sessionId}
          >
            {primaryButtonConfig.icon}
            {primaryButtonConfig.label}
            {primaryButtonConfig.badge != null && (
              <span className="ml-1 rounded-full bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground">
                {primaryButtonConfig.badge}
              </span>
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{primaryButtonConfig.tooltip}</TooltipContent>
      </Tooltip>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="rounded-none border-0 border-l px-2 cursor-pointer"
            disabled={isGitLoading || !sessionId}
          >
            {isGitLoading ? (
              <IconLoader2 className="h-4 w-4 animate-spin" />
            ) : (
              <IconChevronDown className="h-4 w-4" />
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuItem
            className="cursor-pointer gap-3"
            onClick={openPRDialog}
            disabled={isGitLoading || !sessionId}
          >
            <IconGitPullRequest className="h-4 w-4 text-muted-foreground" />
            <span className="flex-1">Create PR</span>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="cursor-pointer gap-3"
            onClick={handlePull}
            disabled={isGitLoading || !sessionId}
          >
            <IconCloudDownload className="h-4 w-4 text-muted-foreground" />
            <span className="flex-1">Pull</span>
            {hasMatchingUpstream && (gitStatus?.behind ?? 0) > 0 && (
              <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
                ↓{gitStatus?.behind}
              </span>
            )}
          </DropdownMenuItem>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger
              className="cursor-pointer gap-3"
              disabled={isGitLoading || !sessionId}
            >
              <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
              <span className="flex-1">Push</span>
              {hasMatchingUpstream && (gitStatus?.ahead ?? 0) > 0 && (
                <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
                  ↑{gitStatus?.ahead}
                </span>
              )}
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <DropdownMenuItem
                className="cursor-pointer gap-3"
                onClick={() => handlePush(false)}
                disabled={isGitLoading || !sessionId}
              >
                <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
                <span>Push</span>
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer gap-3"
                onClick={() => handlePush(true)}
                disabled={isGitLoading || !sessionId}
              >
                <IconAlertTriangle className="h-4 w-4 text-muted-foreground" />
                <span>Force Push</span>
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="cursor-pointer gap-3"
            onClick={handleRebase}
            disabled={isGitLoading || !sessionId}
          >
            <IconGitCherryPick className="h-4 w-4 text-muted-foreground" />
            <span className="flex-1">Rebase</span>
            <span className="text-xs text-muted-foreground">
              onto {baseBranch || 'origin/main'}
            </span>
          </DropdownMenuItem>
          <DropdownMenuItem
            className="cursor-pointer gap-3"
            onClick={handleMerge}
            disabled={isGitLoading || !sessionId}
          >
            <IconGitMerge className="h-4 w-4 text-muted-foreground" />
            <span className="flex-1">Merge</span>
            <span className="text-xs text-muted-foreground">
              from {baseBranch || 'origin/main'}
            </span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
});

export { VcsSplitButton };
