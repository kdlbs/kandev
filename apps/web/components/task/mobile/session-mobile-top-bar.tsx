'use client';

import { memo, useState, useCallback } from 'react';
import Link from 'next/link';
import {
  IconArrowLeft,
  IconMenu2,
  IconGitBranch,
  IconGitCommit,
  IconGitPullRequest,
  IconCloudDownload,
  IconCloudUpload,
  IconGitCherryPick,
  IconGitMerge,
  IconDots,
  IconCheck,
  IconLoader2,
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
import { Checkbox } from '@kandev/ui/checkbox';
import { Label } from '@kandev/ui/label';
import { LineStat } from '@/components/diff-stat';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';
import { useGitOperations } from '@/hooks/use-git-operations';
import type { FileInfo } from '@/lib/state/slices';
import { useToast } from '@/components/toast-provider';

type SessionMobileTopBarProps = {
  taskTitle?: string;
  sessionId?: string | null;
  baseBranch?: string;
  worktreeBranch?: string | null;
  onMenuClick: () => void;
  showApproveButton?: boolean;
  onApprove?: () => void;
};

export const SessionMobileTopBar = memo(function SessionMobileTopBar({
  taskTitle,
  sessionId,
  baseBranch,
  worktreeBranch,
  onMenuClick,
  showApproveButton = false,
  onApprove,
}: SessionMobileTopBarProps) {
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [commitMessage, setCommitMessage] = useState('');
  const [stageAll, setStageAll] = useState(true);
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [prTitle, setPrTitle] = useState('');
  const [prBody, setPrBody] = useState('');
  const [prDraft, setPrDraft] = useState(true);

  const { toast } = useToast();
  const gitStatus = useSessionGitStatus(sessionId ?? null);
  const { commits } = useSessionCommits(sessionId ?? null);
  const { pull, push, rebase, merge, commit, createPR, isLoading: isGitLoading } = useGitOperations(sessionId ?? null);

  const displayBranch = worktreeBranch || baseBranch;

  // Calculate total additions and deletions
  let uncommittedAdditions = 0;
  let uncommittedDeletions = 0;
  if (gitStatus?.files && Object.keys(gitStatus.files).length > 0) {
    for (const file of Object.values(gitStatus.files) as FileInfo[]) {
      uncommittedAdditions += file.additions || 0;
      uncommittedDeletions += file.deletions || 0;
    }
  }

  const commitAdditions = commits.reduce((sum, c) => sum + c.insertions, 0);
  const commitDeletions = commits.reduce((sum, c) => sum + c.deletions, 0);
  const totalAdditions = uncommittedAdditions + commitAdditions;
  const totalDeletions = uncommittedDeletions + commitDeletions;
  const uncommittedCount = gitStatus?.files ? Object.keys(gitStatus.files).length : 0;

  const handleGitOperation = useCallback(async (
    operation: () => Promise<{ success: boolean; output: string; error?: string }>,
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
      const result = await createPR(prTitle.trim(), prBody.trim(), baseBranch, prDraft);
      if (result.success) {
        toast({
          title: prDraft ? 'Draft Pull Request created' : 'Pull Request created',
          description: result.pr_url || 'PR created successfully',
          variant: 'success',
        });
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
  }, [prTitle, prBody, baseBranch, prDraft, createPR, toast]);

  return (
    <header className="flex items-center justify-between px-2 py-2 bg-background">
      <div className="flex items-center gap-2 min-w-0 flex-1">
        <Button variant="ghost" size="icon-sm" asChild>
          <Link href="/">
            <IconArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <div className="flex flex-col min-w-0 flex-1">
          <span className="text-sm font-medium truncate">{taskTitle ?? 'Task details'}</span>
          {displayBranch && (
            <div className="flex items-center gap-1.5">
              <IconGitBranch className="h-3 w-3 text-muted-foreground flex-shrink-0" />
              <span className="text-xs text-muted-foreground truncate">{displayBranch}</span>
              {(totalAdditions > 0 || totalDeletions > 0) && (
                <LineStat added={totalAdditions} removed={totalDeletions} />
              )}
            </div>
          )}
        </div>
      </div>

      <div className="flex items-center gap-1">
        {showApproveButton && onApprove && (
          <Button
            size="sm"
            className="h-7 gap-1 px-2 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white text-xs"
            onClick={onApprove}
          >
            <IconCheck className="h-3.5 w-3.5" />
            Approve
          </Button>
        )}

        {/* Git Actions Menu */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="icon-sm" variant="ghost" className="cursor-pointer">
              {isGitLoading ? (
                <IconLoader2 className="h-4 w-4 animate-spin" />
              ) : (
                <IconDots className="h-4 w-4" />
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuItem
              className="cursor-pointer gap-3"
              onClick={handleOpenCommitDialog}
              disabled={isGitLoading || !sessionId}
            >
              <IconGitCommit className={`h-4 w-4 ${uncommittedCount > 0 ? 'text-amber-500' : ''}`} />
              <span className="flex-1">Commit</span>
              {uncommittedCount > 0 && (
                <span className="rounded-full bg-amber-500/20 px-1.5 py-0.5 text-xs font-medium text-amber-600">
                  {uncommittedCount}
                </span>
              )}
            </DropdownMenuItem>
            <DropdownMenuItem
              className="cursor-pointer gap-3"
              onClick={handleOpenPRDialog}
              disabled={isGitLoading || !sessionId}
            >
              <IconGitPullRequest className="h-4 w-4 text-cyan-500" />
              <span className="flex-1">Create PR</span>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="cursor-pointer gap-3"
              onClick={handlePull}
              disabled={isGitLoading || !sessionId}
            >
              <IconCloudDownload className="h-4 w-4 text-blue-500" />
              <span className="flex-1">Pull</span>
            </DropdownMenuItem>
            <DropdownMenuItem
              className="cursor-pointer gap-3"
              onClick={handlePush}
              disabled={isGitLoading || !sessionId}
            >
              <IconCloudUpload className="h-4 w-4 text-green-500" />
              <span className="flex-1">Push</span>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="cursor-pointer gap-3"
              onClick={handleRebase}
              disabled={isGitLoading || !sessionId}
            >
              <IconGitCherryPick className="h-4 w-4 text-orange-500" />
              <span className="flex-1">Rebase</span>
              <span className="text-xs text-muted-foreground">onto {baseBranch || 'main'}</span>
            </DropdownMenuItem>
            <DropdownMenuItem
              className="cursor-pointer gap-3"
              onClick={handleMerge}
              disabled={isGitLoading || !sessionId}
            >
              <IconGitMerge className="h-4 w-4 text-purple-500" />
              <span className="flex-1">Merge</span>
              <span className="text-xs text-muted-foreground">from {baseBranch || 'main'}</span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {/* Task Switcher Menu Button */}
        <Button variant="ghost" size="icon-sm" className="cursor-pointer" onClick={onMenuClick}>
          <IconMenu2 className="h-4 w-4" />
        </Button>
      </div>

      {/* Commit Dialog */}
      <Dialog open={commitDialogOpen} onOpenChange={setCommitDialogOpen}>
        <DialogContent className="max-w-[90vw] sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <IconGitCommit className="h-5 w-5 text-amber-500" />
              Commit Changes
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="text-sm text-muted-foreground">
              {uncommittedCount > 0 ? (
                <span>
                  <span className="font-medium text-foreground">{uncommittedCount}</span> file{uncommittedCount !== 1 ? 's' : ''} changed
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
                id="stage-all-mobile"
                checked={stageAll}
                onCheckedChange={(checked) => setStageAll(checked === true)}
              />
              <Label htmlFor="stage-all-mobile" className="text-sm text-muted-foreground cursor-pointer">
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
        <DialogContent className="max-w-[90vw] sm:max-w-[500px]">
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
                  Creating PR from <span className="font-medium text-foreground">{displayBranch}</span> to <span className="font-medium text-foreground">{baseBranch}</span>
                </span>
              ) : (
                <span>
                  Creating PR from <span className="font-medium text-foreground">{displayBranch}</span>
                </span>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="pr-title-mobile" className="text-sm">Title</Label>
              <input
                id="pr-title-mobile"
                type="text"
                placeholder="Pull request title..."
                value={prTitle}
                onChange={(e) => setPrTitle(e.target.value)}
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pr-body-mobile" className="text-sm">Description</Label>
              <Textarea
                id="pr-body-mobile"
                placeholder="Describe your changes..."
                value={prBody}
                onChange={(e) => setPrBody(e.target.value)}
                rows={4}
                className="resize-none"
              />
            </div>
            <div className="flex items-center space-x-2">
              <Checkbox
                id="pr-draft-mobile"
                checked={prDraft}
                onCheckedChange={(checked) => setPrDraft(checked === true)}
              />
              <Label htmlFor="pr-draft-mobile" className="text-sm cursor-pointer">
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
