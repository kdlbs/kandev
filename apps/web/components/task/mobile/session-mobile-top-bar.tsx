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

function computeUncommittedStats(files: Record<string, FileInfo> | undefined) {
  let additions = 0;
  let deletions = 0;
  if (files) {
    for (const file of Object.values(files) as FileInfo[]) {
      additions += file.additions || 0;
      deletions += file.deletions || 0;
    }
  }
  return { additions, deletions, count: files ? Object.keys(files).length : 0 };
}

type GitOperationRunner = () => Promise<{ success: boolean; output: string; error?: string }>;

function useGitToast() {
  const { toast } = useToast();

  return useCallback(async (operation: GitOperationRunner, operationName: string) => {
    try {
      const result = await operation();
      if (result.success) {
        toast({ title: `${operationName} successful`, description: result.output.slice(0, 200) || `${operationName} completed successfully`, variant: 'success' });
      } else {
        toast({ title: `${operationName} failed`, description: result.error || 'An error occurred', variant: 'error' });
      }
    } catch (error) {
      toast({ title: `${operationName} failed`, description: error instanceof Error ? error.message : 'An unexpected error occurred', variant: 'error' });
    }
  }, [toast]);
}

function CommitDialog({ open, onOpenChange, uncommittedCount, uncommittedAdditions, uncommittedDeletions, isGitLoading, onCommit }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  uncommittedCount: number;
  uncommittedAdditions: number;
  uncommittedDeletions: number;
  isGitLoading: boolean;
  onCommit: (message: string, stageAll: boolean) => void;
}) {
  const [commitMessage, setCommitMessage] = useState('');
  const [stageAll, setStageAll] = useState(true);

  const handleOpen = (isOpen: boolean) => {
    if (isOpen) { setCommitMessage(''); setStageAll(true); }
    onOpenChange(isOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpen}>
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
                  <span className="ml-2">(<span className="text-green-600">+{uncommittedAdditions}</span>{' / '}<span className="text-red-600">-{uncommittedDeletions}</span>)</span>
                )}
              </span>
            ) : <span>No changes to commit</span>}
          </div>
          <Textarea placeholder="Enter commit message..." value={commitMessage} onChange={(e) => setCommitMessage(e.target.value)} rows={4} className="resize-none" autoFocus />
          <div className="flex items-center gap-2">
            <Checkbox id="stage-all-mobile" checked={stageAll} onCheckedChange={(checked) => setStageAll(checked === true)} />
            <Label htmlFor="stage-all-mobile" className="text-sm text-muted-foreground cursor-pointer">Stage all changes before committing</Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild><Button type="button" variant="outline">Cancel</Button></DialogClose>
          <Button onClick={() => onCommit(commitMessage.trim(), stageAll)} disabled={!commitMessage.trim() || isGitLoading}>
            {isGitLoading ? <><IconLoader2 className="h-4 w-4 animate-spin mr-2" />Committing...</> : <><IconCheck className="h-4 w-4 mr-2" />Commit</>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PRDialog({ open, onOpenChange, displayBranch, baseBranch, isGitLoading, taskTitle, onCreatePR }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  displayBranch: string | undefined;
  baseBranch: string | undefined;
  isGitLoading: boolean;
  taskTitle: string | undefined;
  onCreatePR: (title: string, body: string, draft: boolean) => void;
}) {
  const [prTitle, setPrTitle] = useState('');
  const [prBody, setPrBody] = useState('');
  const [prDraft, setPrDraft] = useState(true);

  const handleOpen = (isOpen: boolean) => {
    if (isOpen) { setPrTitle(taskTitle || ''); setPrBody(''); }
    onOpenChange(isOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpen}>
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
              <span>Creating PR from <span className="font-medium text-foreground">{displayBranch}</span> to <span className="font-medium text-foreground">{baseBranch}</span></span>
            ) : (
              <span>Creating PR from <span className="font-medium text-foreground">{displayBranch}</span></span>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="pr-title-mobile" className="text-sm">Title</Label>
            <input id="pr-title-mobile" type="text" placeholder="Pull request title..." value={prTitle} onChange={(e) => setPrTitle(e.target.value)} className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" autoFocus />
          </div>
          <div className="space-y-2">
            <Label htmlFor="pr-body-mobile" className="text-sm">Description</Label>
            <Textarea id="pr-body-mobile" placeholder="Describe your changes..." value={prBody} onChange={(e) => setPrBody(e.target.value)} rows={4} className="resize-none" />
          </div>
          <div className="flex items-center space-x-2">
            <Checkbox id="pr-draft-mobile" checked={prDraft} onCheckedChange={(checked) => setPrDraft(checked === true)} />
            <Label htmlFor="pr-draft-mobile" className="text-sm cursor-pointer">Create as draft</Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild><Button type="button" variant="outline">Cancel</Button></DialogClose>
          <Button onClick={() => onCreatePR(prTitle.trim(), prBody.trim(), prDraft)} disabled={!prTitle.trim() || isGitLoading} className="bg-cyan-600 hover:bg-cyan-700 text-white">
            {isGitLoading ? <><IconLoader2 className="h-4 w-4 animate-spin mr-2" />Creating...</> : <><IconGitPullRequest className="h-4 w-4 mr-2" />Create PR</>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type GitActionsDropdownProps = {
  sessionId: string | null | undefined;
  isGitLoading: boolean;
  uncommittedCount: number;
  baseBranch: string | undefined;
  onCommitClick: () => void;
  onPRClick: () => void;
  onPull: () => void;
  onPush: (force?: boolean) => void;
  onRebase: () => void;
  onMerge: () => void;
};

function GitActionsDropdown({ sessionId, isGitLoading, uncommittedCount, baseBranch, onCommitClick, onPRClick, onPull, onPush, onRebase, onMerge }: GitActionsDropdownProps) {
  const disabled = isGitLoading || !sessionId;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="icon-sm" variant="ghost" className="cursor-pointer">
          {isGitLoading ? <IconLoader2 className="h-4 w-4 animate-spin" /> : <IconDots className="h-4 w-4" />}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuItem className="cursor-pointer gap-3" onClick={onCommitClick} disabled={disabled}>
          <IconGitCommit className={`h-4 w-4 ${uncommittedCount > 0 ? 'text-amber-500' : ''}`} />
          <span className="flex-1">Commit</span>
          {uncommittedCount > 0 && <span className="rounded-full bg-amber-500/20 px-1.5 py-0.5 text-xs font-medium text-amber-600">{uncommittedCount}</span>}
        </DropdownMenuItem>
        <DropdownMenuItem className="cursor-pointer gap-3" onClick={onPRClick} disabled={disabled}>
          <IconGitPullRequest className="h-4 w-4 text-cyan-500" /><span className="flex-1">Create PR</span>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="cursor-pointer gap-3" onClick={onPull} disabled={disabled}>
          <IconCloudDownload className="h-4 w-4 text-blue-500" /><span className="flex-1">Pull</span>
        </DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger className="cursor-pointer gap-3" disabled={disabled}>
            <IconCloudUpload className="h-4 w-4 text-green-500" /><span className="flex-1">Push</span>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuItem className="cursor-pointer gap-3" onClick={() => onPush(false)} disabled={disabled}>
              <IconCloudUpload className="h-4 w-4 text-green-500" /><span>Push</span>
            </DropdownMenuItem>
            <DropdownMenuItem className="cursor-pointer gap-3" onClick={() => onPush(true)} disabled={disabled}>
              <IconAlertTriangle className="h-4 w-4 text-orange-500" /><span>Force Push</span>
            </DropdownMenuItem>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="cursor-pointer gap-3" onClick={onRebase} disabled={disabled}>
          <IconGitCherryPick className="h-4 w-4 text-orange-500" />
          <span className="flex-1">Rebase</span>
          <span className="text-xs text-muted-foreground">onto {baseBranch || 'main'}</span>
        </DropdownMenuItem>
        <DropdownMenuItem className="cursor-pointer gap-3" onClick={onMerge} disabled={disabled}>
          <IconGitMerge className="h-4 w-4 text-purple-500" />
          <span className="flex-1">Merge</span>
          <span className="text-xs text-muted-foreground">from {baseBranch || 'main'}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function useMobileGitActions(sessionId: string | null | undefined, baseBranch: string | undefined, setCommitDialogOpen: (v: boolean) => void, setPrDialogOpen: (v: boolean) => void) {
  const { toast } = useToast();
  const { pull, push, rebase, merge, commit, createPR, isLoading: isGitLoading } = useGitOperations(sessionId ?? null);
  const handleGitOperation = useGitToast();

  const handlePull = useCallback(() => handleGitOperation(() => pull(), 'Pull'), [handleGitOperation, pull]);
  const handlePush = useCallback((force = false) => handleGitOperation(() => push({ force }), force ? 'Force Push' : 'Push'), [handleGitOperation, push]);
  const handleRebase = useCallback(() => {
    const target = baseBranch?.replace(/^origin\//, '') || 'main';
    return handleGitOperation(() => rebase(target), 'Rebase');
  }, [handleGitOperation, rebase, baseBranch]);
  const handleMerge = useCallback(() => {
    const target = baseBranch?.replace(/^origin\//, '') || 'main';
    return handleGitOperation(() => merge(target), 'Merge');
  }, [handleGitOperation, merge, baseBranch]);

  const handleCommit = useCallback(async (message: string, stageAll: boolean) => {
    setCommitDialogOpen(false);
    await handleGitOperation(() => commit(message, stageAll), 'Commit');
  }, [handleGitOperation, commit, setCommitDialogOpen]);

  const handleCreatePR = useCallback(async (title: string, body: string, draft: boolean) => {
    setPrDialogOpen(false);
    try {
      const result = await createPR(title, body, baseBranch, draft);
      if (result.success) {
        toast({ title: draft ? 'Draft Pull Request created' : 'Pull Request created', description: result.pr_url || 'PR created successfully', variant: 'success' });
        if (result.pr_url) window.open(result.pr_url, '_blank');
      } else {
        toast({ title: 'Create PR failed', description: result.error || 'An error occurred', variant: 'error' });
      }
    } catch (e) {
      toast({ title: 'Create PR failed', description: e instanceof Error ? e.message : 'An error occurred', variant: 'error' });
    }
  }, [createPR, baseBranch, toast, setPrDialogOpen]);

  return { isGitLoading, handlePull, handlePush, handleRebase, handleMerge, handleCommit, handleCreatePR };
}

export const SessionMobileTopBar = memo(function SessionMobileTopBar({
  taskTitle, sessionId, baseBranch, worktreeBranch, onMenuClick, showApproveButton = false, onApprove,
}: SessionMobileTopBarProps) {
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [prDialogOpen, setPrDialogOpen] = useState(false);

  const gitStatus = useSessionGitStatus(sessionId ?? null);
  const { commits } = useSessionCommits(sessionId ?? null);
  const { isGitLoading, handlePull, handlePush, handleRebase, handleMerge, handleCommit, handleCreatePR } = useMobileGitActions(sessionId, baseBranch, setCommitDialogOpen, setPrDialogOpen);

  const displayBranch = worktreeBranch || baseBranch;
  const { additions: uncommittedAdditions, deletions: uncommittedDeletions, count: uncommittedCount } = computeUncommittedStats(gitStatus?.files);
  const totalAdditions = uncommittedAdditions + commits.reduce((s, c) => s + c.insertions, 0);
  const totalDeletions = uncommittedDeletions + commits.reduce((s, c) => s + c.deletions, 0);

  return (
    <header className="flex items-center justify-between px-2 py-2 bg-background">
      <div className="flex items-center gap-2 min-w-0 flex-1">
        <Button variant="ghost" size="icon-sm" asChild>
          <Link href="/"><IconArrowLeft className="h-4 w-4" /></Link>
        </Button>
        <div className="flex flex-col min-w-0 flex-1">
          <span className="text-sm font-medium truncate">{taskTitle ?? 'Task details'}</span>
          {displayBranch && (
            <div className="flex items-center gap-1.5">
              <IconGitBranch className="h-3 w-3 text-muted-foreground flex-shrink-0" />
              <span className="text-xs text-muted-foreground truncate">{displayBranch}</span>
              {(totalAdditions > 0 || totalDeletions > 0) && <LineStat added={totalAdditions} removed={totalDeletions} />}
            </div>
          )}
        </div>
      </div>
      <div className="flex items-center gap-1">
        {showApproveButton && onApprove && (
          <Button size="sm" className="h-7 gap-1 px-2 cursor-pointer bg-emerald-600 hover:bg-emerald-700 text-white text-xs" onClick={onApprove}>
            <IconCheck className="h-3.5 w-3.5" />Approve
          </Button>
        )}
        <GitActionsDropdown sessionId={sessionId} isGitLoading={isGitLoading} uncommittedCount={uncommittedCount} baseBranch={baseBranch} onCommitClick={() => setCommitDialogOpen(true)} onPRClick={() => setPrDialogOpen(true)} onPull={handlePull} onPush={handlePush} onRebase={handleRebase} onMerge={handleMerge} />
        <Button variant="ghost" size="icon-sm" className="cursor-pointer" onClick={onMenuClick}>
          <IconMenu2 className="h-4 w-4" />
        </Button>
      </div>
      <CommitDialog open={commitDialogOpen} onOpenChange={setCommitDialogOpen} uncommittedCount={uncommittedCount} uncommittedAdditions={uncommittedAdditions} uncommittedDeletions={uncommittedDeletions} isGitLoading={isGitLoading} onCommit={handleCommit} />
      <PRDialog open={prDialogOpen} onOpenChange={setPrDialogOpen} displayBranch={displayBranch} baseBranch={baseBranch} isGitLoading={isGitLoading} taskTitle={taskTitle} onCreatePR={handleCreatePR} />
    </header>
  );
});
