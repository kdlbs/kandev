'use client';

import { memo, useState, useCallback } from 'react';
import {
  IconGitCommit,
  IconGitPullRequest,
  IconGitMerge,
  IconGitCherryPick,
  IconLoader2,
  IconCheck,
  IconChevronDown,
  IconCloudDownload,
  IconCloudUpload,
  IconAlertTriangle,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from '@kandev/ui/dialog';
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
import { Checkbox } from '@kandev/ui/checkbox';
import { Label } from '@kandev/ui/label';
import { Textarea } from '@kandev/ui/textarea';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useGitOperations } from '@/hooks/use-git-operations';
import { useToast } from '@/components/toast-provider';
import type { FileInfo } from '@/lib/state/slices';

type VcsSplitButtonProps = {
  sessionId: string | null;
  baseBranch?: string;
  taskTitle?: string;
  /** Branch name shown in the PR dialog (e.g. "feature/foo → main") */
  displayBranch?: string | null;
};

const VcsSplitButton = memo(function VcsSplitButton({
  sessionId,
  baseBranch,
  taskTitle,
  displayBranch,
}: VcsSplitButtonProps) {
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [commitMessage, setCommitMessage] = useState('');
  const [stageAll, setStageAll] = useState(true);
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [prTitle, setPrTitle] = useState('');
  const [prBody, setPrBody] = useState('');
  const [prDraft, setPrDraft] = useState(true);

  const { toast } = useToast();
  const gitStatus = useSessionGitStatus(sessionId);
  const { pull, push, rebase, merge, commit, createPR, isLoading: isGitLoading } =
    useGitOperations(sessionId);

  // Check if remote branch tracks current branch
  const currentBranch = gitStatus?.branch;
  const remoteBranch = gitStatus?.remote_branch;
  const hasMatchingUpstream =
    remoteBranch && currentBranch && remoteBranch === `origin/${currentBranch}`;

  // Uncommitted file stats
  const uncommittedFileCount = gitStatus?.files
    ? Object.keys(gitStatus.files).length
    : 0;

  let uncommittedAdditions = 0;
  let uncommittedDeletions = 0;
  if (gitStatus?.files && uncommittedFileCount > 0) {
    for (const file of Object.values(gitStatus.files) as FileInfo[]) {
      uncommittedAdditions += file.additions || 0;
      uncommittedDeletions += file.deletions || 0;
    }
  }

  const aheadCount = gitStatus?.ahead ?? 0;
  const behindCount = gitStatus?.behind ?? 0;

  // --- handlers ---

  const handleGitOperation = useCallback(
    async (
      operation: () => Promise<{
        success: boolean;
        output: string;
        error?: string;
      }>,
      operationName: string
    ) => {
      try {
        const result = await operation();
        if (result.success) {
          toast({
            title: `${operationName} successful`,
            description:
              result.output.slice(0, 200) || `${operationName} completed`,
            variant: 'success',
          });
        } else {
          toast({
            title: `${operationName} failed`,
            description: result.error || 'An error occurred',
            variant: 'error',
          });
        }
      } catch (e) {
        toast({
          title: `${operationName} failed`,
          description:
            e instanceof Error ? e.message : 'An unexpected error occurred',
          variant: 'error',
        });
      }
    },
    [toast]
  );

  const handlePull = useCallback(() => {
    handleGitOperation(() => pull(), 'Pull');
  }, [handleGitOperation, pull]);

  const handlePush = useCallback(
    (force = false) => {
      handleGitOperation(
        () => push({ force }),
        force ? 'Force Push' : 'Push'
      );
    },
    [handleGitOperation, push]
  );

  const handleRebase = useCallback(() => {
    const targetBranch = baseBranch?.replace(/^origin\//, '') || 'main';
    handleGitOperation(() => rebase(targetBranch), 'Rebase');
  }, [handleGitOperation, rebase, baseBranch]);

  const handleMerge = useCallback(() => {
    const targetBranch = baseBranch?.replace(/^origin\//, '') || 'main';
    handleGitOperation(() => merge(targetBranch), 'Merge');
  }, [handleGitOperation, merge, baseBranch]);

  const handleOpenCommitDialog = useCallback(() => {
    setCommitMessage('');
    setStageAll(true);
    setCommitDialogOpen(true);
  }, []);

  const handleCommit = useCallback(async () => {
    if (!commitMessage.trim()) return;
    setCommitDialogOpen(false);
    await handleGitOperation(
      () => commit(commitMessage.trim(), stageAll),
      'Commit'
    );
    setCommitMessage('');
  }, [commitMessage, stageAll, handleGitOperation, commit]);

  const handleOpenPRDialog = useCallback(() => {
    setPrTitle(taskTitle || '');
    setPrBody('');
    setPrDialogOpen(true);
  }, [taskTitle]);

  const handleCreatePR = useCallback(async () => {
    if (!prTitle.trim()) return;
    setPrDialogOpen(false);
    try {
      const result = await createPR(
        prTitle.trim(),
        prBody.trim(),
        baseBranch,
        prDraft
      );
      if (result.success) {
        toast({
          title: prDraft ? 'Draft PR created' : 'PR created',
          description: result.pr_url || 'PR created successfully',
          variant: 'success',
        });
        if (result.pr_url) window.open(result.pr_url, '_blank');
      } else {
        toast({
          title: 'Create PR failed',
          description: result.error || 'An error occurred',
          variant: 'error',
        });
      }
    } catch (e) {
      toast({
        title: 'Create PR failed',
        description:
          e instanceof Error ? e.message : 'An error occurred',
        variant: 'error',
      });
    }
    setPrTitle('');
    setPrBody('');
  }, [prTitle, prBody, baseBranch, prDraft, createPR, toast]);

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
      onClick: handleOpenCommitDialog,
    },
    pr: {
      icon: <IconGitPullRequest className="h-4 w-4" />,
      label: 'Create PR',
      badge: aheadCount > 0 ? aheadCount : null,
      tooltip: `Create PR (${aheadCount} commit${aheadCount !== 1 ? 's' : ''} ahead)`,
      onClick: handleOpenPRDialog,
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
    <>
      {/* Split button */}
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
              onClick={handleOpenPRDialog}
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

      {/* Commit Dialog */}
      <Dialog open={commitDialogOpen} onOpenChange={setCommitDialogOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <IconGitCommit className="h-5 w-5" />
              Commit Changes
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="text-sm text-muted-foreground">
              {uncommittedFileCount > 0 ? (
                <span>
                  <span className="font-medium text-foreground">
                    {uncommittedFileCount}
                  </span>{' '}
                  file{uncommittedFileCount !== 1 ? 's' : ''} changed
                  {(uncommittedAdditions > 0 || uncommittedDeletions > 0) && (
                    <span className="ml-2">
                      (
                      <span className="text-green-600">
                        +{uncommittedAdditions}
                      </span>
                      {' / '}
                      <span className="text-red-600">
                        -{uncommittedDeletions}
                      </span>
                      )
                    </span>
                  )}
                </span>
              ) : (
                <span>No changes to commit</span>
              )}
            </div>
            <Textarea
              placeholder="Enter commit message..."
              value={commitMessage}
              onChange={(e) => setCommitMessage(e.target.value)}
              rows={4}
              className="resize-none"
              autoFocus
            />
            <div className="flex items-center gap-2">
              <Checkbox
                id="vcs-stage-all"
                checked={stageAll}
                onCheckedChange={(checked) => setStageAll(checked === true)}
              />
              <Label
                htmlFor="vcs-stage-all"
                className="text-sm text-muted-foreground cursor-pointer"
              >
                Stage all changes before committing
              </Label>
            </div>
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                Cancel
              </Button>
            </DialogClose>
            <Button
              onClick={handleCommit}
              disabled={!commitMessage.trim() || isGitLoading}
            >
              {isGitLoading ? (
                <>
                  <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                  Committing...
                </>
              ) : (
                <>
                  <IconCheck className="h-4 w-4 mr-2" />
                  Commit
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create PR Dialog */}
      <Dialog open={prDialogOpen} onOpenChange={setPrDialogOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <IconGitPullRequest className="h-5 w-5" />
              Create Pull Request
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {displayBranch && (
              <div className="text-sm text-muted-foreground">
                {baseBranch ? (
                  <span>
                    Creating PR from{' '}
                    <span className="font-medium text-foreground">
                      {displayBranch}
                    </span>{' '}
                    →{' '}
                    <span className="font-medium text-foreground">
                      {baseBranch}
                    </span>
                  </span>
                ) : (
                  <span>
                    Creating PR from{' '}
                    <span className="font-medium text-foreground">
                      {displayBranch}
                    </span>
                  </span>
                )}
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="vcs-pr-title" className="text-sm">
                Title
              </Label>
              <input
                id="vcs-pr-title"
                type="text"
                placeholder="Pull request title..."
                value={prTitle}
                onChange={(e) => setPrTitle(e.target.value)}
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="vcs-pr-body" className="text-sm">
                Description
              </Label>
              <Textarea
                id="vcs-pr-body"
                placeholder="Describe your changes..."
                value={prBody}
                onChange={(e) => setPrBody(e.target.value)}
                rows={6}
                className="resize-none"
              />
            </div>
            <div className="flex items-center space-x-2">
              <Checkbox
                id="vcs-pr-draft"
                checked={prDraft}
                onCheckedChange={(checked) => setPrDraft(checked === true)}
              />
              <Label
                htmlFor="vcs-pr-draft"
                className="text-sm cursor-pointer"
              >
                Create as draft
              </Label>
            </div>
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                Cancel
              </Button>
            </DialogClose>
            <Button
              onClick={handleCreatePR}
              disabled={!prTitle.trim() || isGitLoading}
            >
              {isGitLoading ? (
                <>
                  <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                  Creating...
                </>
              ) : (
                <>
                  <IconGitPullRequest className="h-4 w-4 mr-2" />
                  Create PR
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
});

export { VcsSplitButton };
