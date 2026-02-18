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

import type { ReactNode } from 'react';

function determinePrimaryAction(uncommittedFileCount: number, aheadCount: number, behindCount: number): 'commit' | 'pr' | 'rebase' {
  if (uncommittedFileCount > 0) return 'commit';
  if (aheadCount > 0) return 'pr';
  if (behindCount > 0) return 'rebase';
  return 'commit';
}

type PrimaryButtonConfig = { icon: ReactNode; label: string; badge: number | null; tooltip: string; onClick: () => void };

function buildCommitConfig(uncommittedFileCount: number, openCommitDialog: () => void): PrimaryButtonConfig {
  return {
    icon: <IconGitCommit className="h-4 w-4" />,
    label: 'Commit',
    badge: uncommittedFileCount > 0 ? uncommittedFileCount : null,
    tooltip: uncommittedFileCount > 0 ? `Commit ${uncommittedFileCount} changed file${uncommittedFileCount !== 1 ? 's' : ''}` : 'No changes to commit',
    onClick: openCommitDialog,
  };
}

function buildPrConfig(aheadCount: number, openPRDialog: () => void): PrimaryButtonConfig {
  return {
    icon: <IconGitPullRequest className="h-4 w-4" />,
    label: 'Create PR',
    badge: aheadCount > 0 ? aheadCount : null,
    tooltip: `Create PR (${aheadCount} commit${aheadCount !== 1 ? 's' : ''} ahead)`,
    onClick: openPRDialog,
  };
}

function buildRebaseConfig(behindCount: number, baseBranch: string | undefined, handleRebase: () => void): PrimaryButtonConfig {
  return {
    icon: <IconGitCherryPick className="h-4 w-4" />,
    label: 'Rebase',
    badge: behindCount > 0 ? behindCount : null,
    tooltip: `Rebase onto ${baseBranch || 'origin/main'} (${behindCount} behind)`,
    onClick: handleRebase,
  };
}

type PrimaryConfigArgs = {
  primaryAction: 'commit' | 'pr' | 'rebase';
  uncommittedFileCount: number; aheadCount: number; behindCount: number;
  baseBranch: string | undefined;
  openCommitDialog: () => void; openPRDialog: () => void; handleRebase: () => void;
};

function buildPrimaryButtonConfig({ primaryAction, uncommittedFileCount, aheadCount, behindCount, baseBranch, openCommitDialog, openPRDialog, handleRebase }: PrimaryConfigArgs): PrimaryButtonConfig {
  if (primaryAction === 'pr') return buildPrConfig(aheadCount, openPRDialog);
  if (primaryAction === 'rebase') return buildRebaseConfig(behindCount, baseBranch, handleRebase);
  return buildCommitConfig(uncommittedFileCount, openCommitDialog);
}

type VcsDropdownItemsProps = {
  disabled: boolean;
  baseBranch?: string;
  hasMatchingUpstream: boolean | "" | null | undefined;
  behindCount: number;
  aheadCount: number;
  onPR: () => void;
  onPull: () => void;
  onPush: (force: boolean) => void;
  onRebase: () => void;
  onMerge: () => void;
};

function VcsDropdownItems({
  disabled, baseBranch, hasMatchingUpstream,
  behindCount, aheadCount,
  onPR, onPull, onPush, onRebase, onMerge,
}: VcsDropdownItemsProps) {
  return (
    <DropdownMenuContent align="end" className="w-56">
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onPR} disabled={disabled}>
        <IconGitPullRequest className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">Create PR</span>
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onPull} disabled={disabled}>
        <IconCloudDownload className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">Pull</span>
        {hasMatchingUpstream && behindCount > 0 && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
            ↓{behindCount}
          </span>
        )}
      </DropdownMenuItem>
      <DropdownMenuSub>
        <DropdownMenuSubTrigger className="cursor-pointer gap-3" disabled={disabled}>
          <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
          <span className="flex-1">Push</span>
          {hasMatchingUpstream && aheadCount > 0 && (
            <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
              ↑{aheadCount}
            </span>
          )}
        </DropdownMenuSubTrigger>
        <DropdownMenuSubContent>
          <DropdownMenuItem className="cursor-pointer gap-3" onClick={() => onPush(false)} disabled={disabled}>
            <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
            <span>Push</span>
          </DropdownMenuItem>
          <DropdownMenuItem className="cursor-pointer gap-3" onClick={() => onPush(true)} disabled={disabled}>
            <IconAlertTriangle className="h-4 w-4 text-muted-foreground" />
            <span>Force Push</span>
          </DropdownMenuItem>
        </DropdownMenuSubContent>
      </DropdownMenuSub>
      <DropdownMenuSeparator />
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onRebase} disabled={disabled}>
        <IconGitCherryPick className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">Rebase</span>
        <span className="text-xs text-muted-foreground">onto {baseBranch || 'origin/main'}</span>
      </DropdownMenuItem>
      <DropdownMenuItem className="cursor-pointer gap-3" onClick={onMerge} disabled={disabled}>
        <IconGitMerge className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">Merge</span>
        <span className="text-xs text-muted-foreground">from {baseBranch || 'origin/main'}</span>
      </DropdownMenuItem>
    </DropdownMenuContent>
  );
}

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

  const currentBranch = gitStatus?.branch;
  const remoteBranch = gitStatus?.remote_branch;
  const hasMatchingUpstream =
    remoteBranch && currentBranch && remoteBranch === `origin/${currentBranch}`;

  const uncommittedFileCount = gitStatus?.files
    ? Object.keys(gitStatus.files).length
    : 0;
  const aheadCount = gitStatus?.ahead ?? 0;
  const behindCount = gitStatus?.behind ?? 0;
  const isDisabled = isGitLoading || !sessionId;

  const handlePull = useCallback(() => {
    gitWithFeedback(() => pull(), 'Pull');
  }, [gitWithFeedback, pull]);

  const handlePush = useCallback(
    (force = false) => {
      gitWithFeedback(() => push({ force }), force ? 'Force Push' : 'Push');
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

  const primaryAction = determinePrimaryAction(uncommittedFileCount, aheadCount, behindCount);
  const primaryButtonConfig = buildPrimaryButtonConfig({ primaryAction, uncommittedFileCount, aheadCount, behindCount, baseBranch, openCommitDialog, openPRDialog, handleRebase });

  return (
    <div className="inline-flex rounded-md border border-border overflow-hidden">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="rounded-none border-0 cursor-pointer"
            onClick={primaryButtonConfig.onClick}
            disabled={isDisabled}
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
            disabled={isDisabled}
          >
            {isGitLoading ? (
              <IconLoader2 className="h-4 w-4 animate-spin" />
            ) : (
              <IconChevronDown className="h-4 w-4" />
            )}
          </Button>
        </DropdownMenuTrigger>
        <VcsDropdownItems
          disabled={isDisabled}
          baseBranch={baseBranch}
          hasMatchingUpstream={hasMatchingUpstream}
          behindCount={behindCount}
          aheadCount={aheadCount}
          onPR={openPRDialog}
          onPull={handlePull}
          onPush={handlePush}
          onRebase={handleRebase}
          onMerge={handleMerge}
        />
      </DropdownMenu>
    </div>
  );
});

export { VcsSplitButton };
