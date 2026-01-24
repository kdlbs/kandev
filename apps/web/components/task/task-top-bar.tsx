'use client';

import { memo, useState, useCallback } from 'react';
import Link from 'next/link';
import {
  IconArrowLeft,
  IconCopy,
  IconDots,
  IconFolderOpen,
  IconGitBranch,
  IconGitMerge,
  IconGitPullRequest,
  IconChevronDown,
  IconCheck,
  IconLoader2,
  IconCloudDownload,
  IconCloudUpload,
  IconGitCherryPick,
  IconGitCommit,
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
} from '@kandev/ui/dropdown-menu';
import { Textarea } from '@kandev/ui/textarea';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { Popover, PopoverContent, PopoverTrigger } from '@kandev/ui/popover';
import { Checkbox } from '@kandev/ui/checkbox';
import { Label } from '@kandev/ui/label';
import { CommitStatBadge, LineStat } from '@/components/diff-stat';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';
import { useGitOperations } from '@/hooks/use-git-operations';
import type { FileInfo } from '@/lib/state/slices';
import { formatUserHomePath } from '@/lib/utils';
import { EditorsMenu } from '@/components/task/editors-menu';
import { useToast } from '@/components/toast-provider';
import { PreviewControls } from '@/components/task/preview/preview-controls';

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
  hasDevScript?: boolean;
};

const TaskTopBar = memo(function TaskTopBar({
  activeSessionId,
  taskTitle,
  baseBranch,
  worktreePath,
  worktreeBranch,
  repositoryPath,
  repositoryName,
  hasDevScript = false,
}: TaskTopBarProps) {
  const [copiedBranch, setCopiedBranch] = useState(false);
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [copiedRepo, setCopiedRepo] = useState(false);
  const [copiedWorktree, setCopiedWorktree] = useState(false);
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [commitMessage, setCommitMessage] = useState('');
  const [stageAll, setStageAll] = useState(true);
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [prTitle, setPrTitle] = useState('');
  const [prBody, setPrBody] = useState('');

  const { toast } = useToast();
  const gitStatus = useSessionGitStatus(activeSessionId ?? null);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  const { pull, push, rebase, merge, commit, createPR, isLoading: isGitLoading } = useGitOperations(activeSessionId ?? null);

  // Use worktree branch if available, otherwise fall back to base branch
  const displayBranch = worktreeBranch || baseBranch;

  // Calculate total additions and deletions from uncommitted files.
  let uncommittedAdditions = 0;
  let uncommittedDeletions = 0;
  if (gitStatus?.files && Object.keys(gitStatus.files).length > 0) {
    for (const file of Object.values(gitStatus.files) as FileInfo[]) {
      uncommittedAdditions += file.additions || 0;
      uncommittedDeletions += file.deletions || 0;
    }
  }

  // Calculate cumulative additions and deletions from commits.
  const commitAdditions = commits.reduce((sum, c) => sum + c.insertions, 0);
  const commitDeletions = commits.reduce((sum, c) => sum + c.deletions, 0);

  // Combined total (uncommitted + commits)
  const totalAdditions = uncommittedAdditions + commitAdditions;
  const totalDeletions = uncommittedDeletions + commitDeletions;

  const handleGitOperation = useCallback(async (
    operation: () => Promise<{ success: boolean; output: string; error?: string; conflict_files?: string[] }>,
    operationName: string
  ) => {
    try {
      const result = await operation();
      if (result.success) {
        toast({
          title: `${operationName} successful`,
          description: result.output.slice(0, 200) || `${operationName} completed successfully`,
          variant: 'success',
        });
      } else {
        toast({
          title: `${operationName} failed`,
          description: result.error || 'An error occurred',
          variant: 'error',
        });

      }
    } catch (error) {
      toast({
        title: `${operationName} failed`,
        description: error instanceof Error ? error.message : 'An unexpected error occurred',
        variant: 'error',
      });
    }
  }, [toast]);

  const handlePull = useCallback(() => {
    handleGitOperation(() => pull(), 'Pull');
  }, [handleGitOperation, pull]);

  const handlePush = useCallback(() => {
    handleGitOperation(() => push(), 'Push');
  }, [handleGitOperation, push]);

  const handleRebase = useCallback(() => {
    // Rebase onto the base branch (e.g., origin/main)
    const targetBranch = baseBranch || 'origin/main';
    handleGitOperation(() => rebase(targetBranch), 'Rebase');
  }, [handleGitOperation, rebase, baseBranch]);

  const handleMerge = useCallback(() => {
    // Merge from the base branch (e.g., origin/main)
    const targetBranch = baseBranch || 'origin/main';
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
    await handleGitOperation(() => commit(commitMessage.trim(), stageAll), 'Commit');
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
      const result = await createPR(prTitle.trim(), prBody.trim(), baseBranch);
      if (result.success) {
        toast({
          title: 'Pull Request created',
          description: result.pr_url || 'PR created successfully',
          variant: 'success',
        });
        // Open the PR URL in a new tab
        if (result.pr_url) {
          window.open(result.pr_url, '_blank');
        }
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
        description: e instanceof Error ? e.message : 'An error occurred',
        variant: 'error',
      });
    }
    setPrTitle('');
    setPrBody('');
  }, [prTitle, prBody, baseBranch, createPR, toast]);

  const handleBranchClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (displayBranch) {
      navigator.clipboard?.writeText(displayBranch);
      setCopiedBranch(true);
      setTimeout(() => setCopiedBranch(false), 500);
    }
  };

  // Preview controls are handled by a dedicated component.

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

        {/* Git Status: Total Lines Changed (uncommitted + commits) */}
        {(totalAdditions > 0 || totalDeletions > 0) && (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-default">
                <LineStat added={totalAdditions} removed={totalDeletions} />
              </span>
            </TooltipTrigger>
            <TooltipContent>
              Total changes{commits.length > 0 ? ` (${commits.length} commit${commits.length !== 1 ? 's' : ''}${uncommittedAdditions > 0 || uncommittedDeletions > 0 ? ' + uncommitted' : ''})` : ''}
            </TooltipContent>
          </Tooltip>
        )}

      </div>
      <div className="flex items-center gap-2">
        <PreviewControls activeSessionId={activeSessionId ?? null} hasDevScript={hasDevScript} />
        <EditorsMenu activeSessionId={activeSessionId ?? null} />

        {/* Commit Split Button */}
        <div className="inline-flex rounded-md border border-border overflow-hidden">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="outline"
                className={`rounded-none border-0 cursor-pointer ${
                  gitStatus?.files && Object.keys(gitStatus.files).length > 0
                    ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400 hover:bg-amber-500/20'
                    : ''
                }`}
                onClick={handleOpenCommitDialog}
                disabled={isGitLoading || !activeSessionId}
              >
                <IconGitCommit className={`h-4 w-4 ${gitStatus?.files && Object.keys(gitStatus.files).length > 0 ? 'text-amber-500' : ''}`} />
                Commit
                {gitStatus?.files && Object.keys(gitStatus.files).length > 0 && (
                  <span className="ml-1 rounded-full bg-amber-500/20 px-1.5 py-0.5 text-xs font-medium">
                    {Object.keys(gitStatus.files).length}
                  </span>
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {gitStatus?.files && Object.keys(gitStatus.files).length > 0
                ? `Commit ${Object.keys(gitStatus.files).length} changed file${Object.keys(gitStatus.files).length !== 1 ? 's' : ''}`
                : 'No changes to commit'}
            </TooltipContent>
          </Tooltip>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="outline" className="rounded-none border-0 border-l px-2 cursor-pointer" disabled={isGitLoading}>
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
                disabled={isGitLoading || !activeSessionId}
              >
                <IconGitPullRequest className="h-4 w-4 text-cyan-500" />
                <span className="flex-1">Create PR</span>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="cursor-pointer gap-3"
                onClick={handlePull}
                disabled={isGitLoading || !activeSessionId}
              >
                <IconCloudDownload className="h-4 w-4 text-blue-500" />
                <span className="flex-1">Pull</span>
                {(gitStatus?.behind ?? 0) > 0 && (
                  <span className="rounded-full bg-blue-500/10 px-2 py-0.5 text-xs font-medium text-blue-600 dark:text-blue-400">
                    ↓{gitStatus?.behind}
                  </span>
                )}
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer gap-3"
                onClick={handlePush}
                disabled={isGitLoading || !activeSessionId}
              >
                <IconCloudUpload className="h-4 w-4 text-green-500" />
                <span className="flex-1">Push</span>
                {(gitStatus?.ahead ?? 0) > 0 && (
                  <span className="rounded-full bg-green-500/10 px-2 py-0.5 text-xs font-medium text-green-600 dark:text-green-400">
                    ↑{gitStatus?.ahead}
                  </span>
                )}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="cursor-pointer gap-3"
                onClick={handleRebase}
                disabled={isGitLoading || !activeSessionId}
              >
                <IconGitCherryPick className="h-4 w-4 text-orange-500" />
                <span className="flex-1">Rebase</span>
                <span className="text-xs text-muted-foreground">onto {baseBranch || 'origin/main'}</span>
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer gap-3"
                onClick={handleMerge}
                disabled={isGitLoading || !activeSessionId}
              >
                <IconGitMerge className="h-4 w-4 text-purple-500" />
                <span className="flex-1">Merge</span>
                <span className="text-xs text-muted-foreground">from {baseBranch || 'origin/main'}</span>
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
            {/* TODO: Implement open workspace folder
            <DropdownMenuItem
              className="cursor-pointer"
              onClick={() => {
                // Open workspace folder in system file manager
              }}
              disabled={!worktreePath}
            >
              <IconFolderOpen className="h-4 w-4" />
              Open workspace folder
            </DropdownMenuItem>
            */}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Commit Dialog */}
      <Dialog open={commitDialogOpen} onOpenChange={setCommitDialogOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <IconGitCommit className="h-5 w-5 text-amber-500" />
              Commit Changes
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="text-sm text-muted-foreground">
              {gitStatus?.files && Object.keys(gitStatus.files).length > 0 ? (
                <span>
                  <span className="font-medium text-foreground">{Object.keys(gitStatus.files).length}</span> file{Object.keys(gitStatus.files).length !== 1 ? 's' : ''} changed
                  {(uncommittedAdditions > 0 || uncommittedDeletions > 0) && (
                    <span className="ml-2">
                      (<span className="text-green-600">+{uncommittedAdditions}</span>
                      {' / '}
                      <span className="text-red-600">-{uncommittedDeletions}</span>)
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
                id="stage-all"
                checked={stageAll}
                onCheckedChange={(checked) => setStageAll(checked === true)}
              />
              <Label htmlFor="stage-all" className="text-sm text-muted-foreground cursor-pointer">
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
              <IconGitPullRequest className="h-5 w-5 text-cyan-500" />
              Create Pull Request
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="text-sm text-muted-foreground">
              {baseBranch ? (
                <span>
                  Creating PR from <span className="font-medium text-foreground">{displayBranch}</span> → <span className="font-medium text-foreground">{baseBranch}</span>
                </span>
              ) : (
                <span>
                  Creating PR from <span className="font-medium text-foreground">{displayBranch}</span>
                </span>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="pr-title" className="text-sm">Title</Label>
              <input
                id="pr-title"
                type="text"
                placeholder="Pull request title..."
                value={prTitle}
                onChange={(e) => setPrTitle(e.target.value)}
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pr-body" className="text-sm">Description</Label>
              <Textarea
                id="pr-body"
                placeholder="Describe your changes..."
                value={prBody}
                onChange={(e) => setPrBody(e.target.value)}
                rows={6}
                className="resize-none"
              />
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
              className="bg-cyan-600 hover:bg-cyan-700 text-white"
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
    </header>
  );
});

export { TaskTopBar };
