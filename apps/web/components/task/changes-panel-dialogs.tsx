'use client';

import {
  IconGitCommit,
  IconGitPullRequest,
  IconLoader2,
  IconCheck,
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
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@kandev/ui/alert-dialog';
import { Checkbox } from '@kandev/ui/checkbox';
import { Label } from '@kandev/ui/label';
import { Textarea } from '@kandev/ui/textarea';

// --- Discard Confirmation Dialog ---

type DiscardDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileToDiscard: string | null;
  onConfirm: () => void;
};

export function DiscardDialog({ open, onOpenChange, fileToDiscard, onConfirm }: DiscardDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Discard changes?</AlertDialogTitle>
          <AlertDialogDescription>
            This will permanently discard all changes to{' '}
            <span className="font-semibold">{fileToDiscard}</span>. This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
            Discard
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// --- Commit Dialog ---

type CommitDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  commitMessage: string;
  onCommitMessageChange: (value: string) => void;
  onCommit: () => void;
  isLoading: boolean;
  stagedFileCount: number;
  stagedAdditions: number;
  stagedDeletions: number;
};

export function CommitDialog({
  open,
  onOpenChange,
  commitMessage,
  onCommitMessageChange,
  onCommit,
  isLoading,
  stagedFileCount,
  stagedAdditions,
  stagedDeletions,
}: CommitDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitCommit className="h-5 w-5" />
            Commit Changes
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <CommitDialogStats
            stagedFileCount={stagedFileCount}
            stagedAdditions={stagedAdditions}
            stagedDeletions={stagedDeletions}
          />
          <Textarea
            placeholder="Enter commit message..."
            value={commitMessage}
            onChange={(e) => onCommitMessageChange(e.target.value)}
            rows={4}
            className="resize-none"
            autoFocus
          />
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline">
              Cancel
            </Button>
          </DialogClose>
          <Button
            onClick={onCommit}
            disabled={!commitMessage.trim() || isLoading}
          >
            {isLoading ? (
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
  );
}

function CommitDialogStats({
  stagedFileCount,
  stagedAdditions,
  stagedDeletions,
}: {
  stagedFileCount: number;
  stagedAdditions: number;
  stagedDeletions: number;
}) {
  return (
    <div className="text-sm text-muted-foreground">
      {stagedFileCount > 0 ? (
        <span>
          <span className="font-medium text-foreground">{stagedFileCount}</span>{' '}
          staged file{stagedFileCount !== 1 ? 's' : ''}
          {(stagedAdditions > 0 || stagedDeletions > 0) && (
            <span className="ml-2">
              (<span className="text-green-600">+{stagedAdditions}</span>
              {' / '}
              <span className="text-red-600">-{stagedDeletions}</span>)
            </span>
          )}
        </span>
      ) : (
        <span>No staged files to commit</span>
      )}
    </div>
  );
}

// --- Create PR Dialog ---

type PRDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  prTitle: string;
  onPrTitleChange: (value: string) => void;
  prBody: string;
  onPrBodyChange: (value: string) => void;
  prDraft: boolean;
  onPrDraftChange: (value: boolean) => void;
  onCreatePR: () => void;
  isLoading: boolean;
  displayBranch: string | null;
  baseBranch: string | undefined;
};

export function PRDialog({
  open,
  onOpenChange,
  prTitle,
  onPrTitleChange,
  prBody,
  onPrBodyChange,
  prDraft,
  onPrDraftChange,
  onCreatePR,
  isLoading,
  displayBranch,
  baseBranch,
}: PRDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitPullRequest className="h-5 w-5" />
            Create Pull Request
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          {displayBranch && (
            <PRBranchInfo displayBranch={displayBranch} baseBranch={baseBranch} />
          )}
          <div className="space-y-2">
            <Label htmlFor="changes-pr-title" className="text-sm">
              Title
            </Label>
            <input
              id="changes-pr-title"
              type="text"
              placeholder="Pull request title..."
              value={prTitle}
              onChange={(e) => onPrTitleChange(e.target.value)}
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="changes-pr-body" className="text-sm">
              Description
            </Label>
            <Textarea
              id="changes-pr-body"
              placeholder="Describe your changes..."
              value={prBody}
              onChange={(e) => onPrBodyChange(e.target.value)}
              rows={6}
              className="resize-none"
            />
          </div>
          <div className="flex items-center space-x-2">
            <Checkbox
              id="changes-pr-draft"
              checked={prDraft}
              onCheckedChange={(checked) => onPrDraftChange(checked === true)}
            />
            <Label htmlFor="changes-pr-draft" className="text-sm cursor-pointer">
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
            onClick={onCreatePR}
            disabled={!prTitle.trim() || isLoading}
          >
            {isLoading ? (
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
  );
}

function PRBranchInfo({
  displayBranch,
  baseBranch,
}: {
  displayBranch: string;
  baseBranch: string | undefined;
}) {
  return (
    <div className="text-sm text-muted-foreground">
      {baseBranch ? (
        <span>
          Creating PR from{' '}
          <span className="font-medium text-foreground">{displayBranch}</span>{' '}
          &rarr;{' '}
          <span className="font-medium text-foreground">{baseBranch}</span>
        </span>
      ) : (
        <span>
          Creating PR from{' '}
          <span className="font-medium text-foreground">{displayBranch}</span>
        </span>
      )}
    </div>
  );
}
