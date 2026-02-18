'use client';

import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';
import {
  IconGitCommit,
  IconGitPullRequest,
  IconLoader2,
  IconCheck,
} from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from '@kandev/ui/dialog';
import { Button } from '@kandev/ui/button';
import { Checkbox } from '@kandev/ui/checkbox';
import { Label } from '@kandev/ui/label';
import { Textarea } from '@kandev/ui/textarea';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useGitOperations } from '@/hooks/use-git-operations';
import { useGitWithFeedback } from '@/hooks/use-git-with-feedback';
import { useToast } from '@/components/toast-provider';
import type { FileInfo } from '@/lib/state/slices';

type VcsDialogsContextValue = {
  openCommitDialog: () => void;
  openPRDialog: () => void;
};

const VcsDialogsContext = createContext<VcsDialogsContextValue | null>(null);

export function useVcsDialogs() {
  const ctx = useContext(VcsDialogsContext);
  if (!ctx) throw new Error('useVcsDialogs must be used within VcsDialogsProvider');
  return ctx;
}

type VcsDialogsProviderProps = {
  sessionId: string | null;
  baseBranch?: string;
  taskTitle?: string;
  displayBranch?: string | null;
  children: ReactNode;
};

function computeFileSummary(files: Record<string, FileInfo> | undefined) {
  const count = files ? Object.keys(files).length : 0;
  let additions = 0;
  let deletions = 0;
  if (files && count > 0) {
    for (const file of Object.values(files) as FileInfo[]) {
      additions += file.additions || 0;
      deletions += file.deletions || 0;
    }
  }
  return { count, additions, deletions };
}

type CommitDialogProps = {
  open: boolean; onOpenChange: (v: boolean) => void;
  fileSummary: { count: number; additions: number; deletions: number };
  commitMessage: string; onCommitMessageChange: (v: string) => void;
  stageAll: boolean; onStageAllChange: (v: boolean) => void;
  isGitLoading: boolean; onCommit: () => void;
};

function CommitDialog({ open, onOpenChange, fileSummary, commitMessage, onCommitMessageChange, stageAll, onStageAllChange, isGitLoading, onCommit }: CommitDialogProps) {
  const { count, additions, deletions } = fileSummary;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader><DialogTitle className="flex items-center gap-2"><IconGitCommit className="h-5 w-5" />Commit Changes</DialogTitle></DialogHeader>
        <div className="space-y-4 py-2">
          <div className="text-sm text-muted-foreground">
            {count > 0 ? (
              <span>
                <span className="font-medium text-foreground">{count}</span>{' '}file{count !== 1 ? 's' : ''} changed
                {(additions > 0 || deletions > 0) && <span className="ml-2">(<span className="text-green-600">+{additions}</span>{' / '}<span className="text-red-600">-{deletions}</span>)</span>}
              </span>
            ) : <span>No changes to commit</span>}
          </div>
          <Textarea placeholder="Enter commit message..." value={commitMessage} onChange={(e) => onCommitMessageChange(e.target.value)} rows={4} className="resize-none" autoFocus />
          <div className="flex items-center gap-2">
            <Checkbox id="vcs-stage-all" checked={stageAll} onCheckedChange={(checked) => onStageAllChange(checked === true)} />
            <Label htmlFor="vcs-stage-all" className="text-sm text-muted-foreground cursor-pointer">Stage all changes before committing</Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild><Button type="button" variant="outline">Cancel</Button></DialogClose>
          <Button onClick={onCommit} disabled={!commitMessage.trim() || isGitLoading}>
            {isGitLoading ? <><IconLoader2 className="h-4 w-4 animate-spin mr-2" />Committing...</> : <><IconCheck className="h-4 w-4 mr-2" />Commit</>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type PRDialogProps = {
  open: boolean; onOpenChange: (v: boolean) => void;
  displayBranch?: string | null; baseBranch?: string;
  prTitle: string; onPrTitleChange: (v: string) => void;
  prBody: string; onPrBodyChange: (v: string) => void;
  prDraft: boolean; onPrDraftChange: (v: boolean) => void;
  isGitLoading: boolean; onCreatePR: () => void;
};

function PRDialog({ open, onOpenChange, displayBranch, baseBranch, prTitle, onPrTitleChange, prBody, onPrBodyChange, prDraft, onPrDraftChange, isGitLoading, onCreatePR }: PRDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader><DialogTitle className="flex items-center gap-2"><IconGitPullRequest className="h-5 w-5" />Create Pull Request</DialogTitle></DialogHeader>
        <div className="space-y-4 py-2">
          {displayBranch && (
            <div className="text-sm text-muted-foreground">
              {baseBranch
                ? <span>Creating PR from <span className="font-medium text-foreground">{displayBranch}</span>{' → '}<span className="font-medium text-foreground">{baseBranch}</span></span>
                : <span>Creating PR from <span className="font-medium text-foreground">{displayBranch}</span></span>
              }
            </div>
          )}
          <div className="space-y-2">
            <Label htmlFor="vcs-pr-title" className="text-sm">Title</Label>
            <input id="vcs-pr-title" type="text" placeholder="Pull request title..." value={prTitle} onChange={(e) => onPrTitleChange(e.target.value)}
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50" autoFocus />
          </div>
          <div className="space-y-2">
            <Label htmlFor="vcs-pr-body" className="text-sm">Description</Label>
            <Textarea id="vcs-pr-body" placeholder="Describe your changes..." value={prBody} onChange={(e) => onPrBodyChange(e.target.value)} rows={6} className="resize-none" />
          </div>
          <div className="flex items-center space-x-2">
            <Checkbox id="vcs-pr-draft" checked={prDraft} onCheckedChange={(checked) => onPrDraftChange(checked === true)} />
            <Label htmlFor="vcs-pr-draft" className="text-sm cursor-pointer">Create as draft</Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild><Button type="button" variant="outline">Cancel</Button></DialogClose>
          <Button onClick={onCreatePR} disabled={!prTitle.trim() || isGitLoading}>
            {isGitLoading ? <><IconLoader2 className="h-4 w-4 animate-spin mr-2" />Creating...</> : <><IconGitPullRequest className="h-4 w-4 mr-2" />Create PR</>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function VcsDialogsProvider({ sessionId, baseBranch, taskTitle, displayBranch, children }: VcsDialogsProviderProps) {
  const [commitDialogOpen, setCommitDialogOpen] = useState(false);
  const [commitMessage, setCommitMessage] = useState('');
  const [stageAll, setStageAll] = useState(true);
  const [prDialogOpen, setPrDialogOpen] = useState(false);
  const [prTitle, setPrTitle] = useState('');
  const [prBody, setPrBody] = useState('');
  const [prDraft, setPrDraft] = useState(true);

  const { toast } = useToast();
  const gitWithFeedback = useGitWithFeedback();
  const gitStatus = useSessionGitStatus(sessionId);
  const { commit, createPR, isLoading: isGitLoading } = useGitOperations(sessionId);

  const fileSummary = computeFileSummary(gitStatus?.files);

  const openCommitDialog = useCallback(() => { setCommitMessage(''); setStageAll(true); setCommitDialogOpen(true); }, []);

  const handleCommit = useCallback(async () => {
    if (!commitMessage.trim()) return;
    setCommitDialogOpen(false);
    await gitWithFeedback(() => commit(commitMessage.trim(), stageAll), 'Commit');
    setCommitMessage('');
  }, [commitMessage, stageAll, gitWithFeedback, commit]);

  const openPRDialog = useCallback(() => { setPrTitle(taskTitle || ''); setPrBody(''); setPrDialogOpen(true); }, [taskTitle]);

  const handleCreatePR = useCallback(async () => {
    if (!prTitle.trim()) return;
    setPrDialogOpen(false);
    try {
      const result = await createPR(prTitle.trim(), prBody.trim(), baseBranch, prDraft);
      if (result.success) {
        toast({ title: prDraft ? 'Draft PR created' : 'PR created', description: result.pr_url || 'PR created successfully', variant: 'success' });
        if (result.pr_url) window.open(result.pr_url, '_blank');
      } else {
        toast({ title: 'Create PR failed', description: result.error || 'An error occurred', variant: 'error' });
      }
    } catch (e) {
      toast({ title: 'Create PR failed', description: e instanceof Error ? e.message : 'An error occurred', variant: 'error' });
    }
    setPrTitle(''); setPrBody('');
  }, [prTitle, prBody, baseBranch, prDraft, createPR, toast]);

  return (
    <VcsDialogsContext.Provider value={{ openCommitDialog, openPRDialog }}>
      {children}
      <CommitDialog open={commitDialogOpen} onOpenChange={setCommitDialogOpen} fileSummary={fileSummary} commitMessage={commitMessage}
        onCommitMessageChange={setCommitMessage} stageAll={stageAll} onStageAllChange={setStageAll} isGitLoading={isGitLoading} onCommit={handleCommit}
      />
      <PRDialog open={prDialogOpen} onOpenChange={setPrDialogOpen} displayBranch={displayBranch} baseBranch={baseBranch} prTitle={prTitle}
        onPrTitleChange={setPrTitle} prBody={prBody} onPrBodyChange={setPrBody} prDraft={prDraft} onPrDraftChange={setPrDraft}
        isGitLoading={isGitLoading} onCreatePR={handleCreatePR}
      />
    </VcsDialogsContext.Provider>
  );
}
