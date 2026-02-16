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

export function VcsDialogsProvider({
  sessionId,
  baseBranch,
  taskTitle,
  displayBranch,
  children,
}: VcsDialogsProviderProps) {
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

  const openCommitDialog = useCallback(() => {
    setCommitMessage('');
    setStageAll(true);
    setCommitDialogOpen(true);
  }, []);

  const handleCommit = useCallback(async () => {
    if (!commitMessage.trim()) return;
    setCommitDialogOpen(false);
    await gitWithFeedback(
      () => commit(commitMessage.trim(), stageAll),
      'Commit'
    );
    setCommitMessage('');
  }, [commitMessage, stageAll, gitWithFeedback, commit]);

  const openPRDialog = useCallback(() => {
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
        description: e instanceof Error ? e.message : 'An error occurred',
        variant: 'error',
      });
    }
    setPrTitle('');
    setPrBody('');
  }, [prTitle, prBody, baseBranch, prDraft, createPR, toast]);

  return (
    <VcsDialogsContext.Provider value={{ openCommitDialog, openPRDialog }}>
      {children}

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
                      <span className="text-green-600">+{uncommittedAdditions}</span>
                      {' / '}
                      <span className="text-red-600">-{uncommittedDeletions}</span>
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
              <Label htmlFor="vcs-stage-all" className="text-sm text-muted-foreground cursor-pointer">
                Stage all changes before committing
              </Label>
            </div>
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">Cancel</Button>
            </DialogClose>
            <Button onClick={handleCommit} disabled={!commitMessage.trim() || isGitLoading}>
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
                    <span className="font-medium text-foreground">{displayBranch}</span>
                    {' → '}
                    <span className="font-medium text-foreground">{baseBranch}</span>
                  </span>
                ) : (
                  <span>
                    Creating PR from{' '}
                    <span className="font-medium text-foreground">{displayBranch}</span>
                  </span>
                )}
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="vcs-pr-title" className="text-sm">Title</Label>
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
              <Label htmlFor="vcs-pr-body" className="text-sm">Description</Label>
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
              <Label htmlFor="vcs-pr-draft" className="text-sm cursor-pointer">
                Create as draft
              </Label>
            </div>
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">Cancel</Button>
            </DialogClose>
            <Button onClick={handleCreatePR} disabled={!prTitle.trim() || isGitLoading}>
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
    </VcsDialogsContext.Provider>
  );
}
