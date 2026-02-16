'use client';

import { memo, useMemo, useCallback, createRef, useState } from 'react';
import { Dialog, DialogContent, DialogTitle } from '@kandev/ui/dialog';
import type { DiffComment } from '@/lib/diff/types';
import type { FileInfo, CumulativeDiff } from '@/lib/state/slices/session-runtime/types';
import { useDiffCommentsStore } from '@/lib/state/slices/diff-comments/diff-comments-slice';
import { useSessionFileReviews } from '@/hooks/use-session-file-reviews';
import { useGitOperations } from '@/hooks/use-git-operations';
import { useAppStore } from '@/components/state-provider';
import { useToast } from '@/components/toast-provider';
import { ReviewTopBar } from './review-top-bar';
import { ReviewFileTree } from './review-file-tree';
import { ReviewDiffList } from './review-diff-list';
import type { ReviewFile } from './types';
import { hashDiff, normalizeDiffContent } from './types';

type ReviewDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sessionId: string;
  baseBranch?: string;
  onSendComments: (comments: DiffComment[]) => void;
  onOpenFile?: (filePath: string) => void;
  gitStatusFiles: Record<string, FileInfo> | null;
  cumulativeDiff: CumulativeDiff | null;
};


export const ReviewDialog = memo(function ReviewDialog({
  open,
  onOpenChange,
  sessionId,
  baseBranch,
  onSendComments,
  onOpenFile,
  gitStatusFiles,
  cumulativeDiff,
}: ReviewDialogProps) {
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [splitView, setSplitView] = useState(() => {
    if (typeof window === 'undefined') return false;
    return localStorage.getItem('diff-view-mode') === 'split';
  });
  const [wordWrap, setWordWrap] = useState(false);
  const autoMarkOnScroll = useAppStore((s) => s.userSettings.reviewAutoMarkOnScroll);

  const { reviews, markReviewed, markUnreviewed } =
    useSessionFileReviews(sessionId);

  const bySession = useDiffCommentsStore((s) => s.bySession);
  const getPendingComments = useDiffCommentsStore((s) => s.getPendingComments);
  const markCommentsSent = useDiffCommentsStore((s) => s.markCommentsSent);

  const { discard } = useGitOperations(sessionId);
  const { toast } = useToast();

  // Merge uncommitted + committed files into a single list
  const allFiles = useMemo<ReviewFile[]>(() => {
    const fileMap = new Map<string, ReviewFile>();

    // Uncommitted changes from git status
    if (gitStatusFiles) {
      for (const [path, file] of Object.entries(gitStatusFiles)) {
        const diff = file.diff ? normalizeDiffContent(file.diff) : '';
        if (diff) {
          fileMap.set(path, {
            path,
            diff,
            status: file.status,
            additions: file.additions ?? 0,
            deletions: file.deletions ?? 0,
            staged: file.staged,
            source: 'uncommitted',
          });
        }
      }
    }

    // Committed changes from cumulative diff
    if (cumulativeDiff?.files) {
      for (const [path, file] of Object.entries(cumulativeDiff.files)) {
        if (fileMap.has(path)) continue; // Uncommitted takes priority
        const diff = file.diff ? normalizeDiffContent(file.diff) : '';
        if (diff) {
          fileMap.set(path, {
            path,
            diff,
            status: file.status || 'modified',
            additions: file.additions ?? 0,
            deletions: file.deletions ?? 0,
            staged: false,
            source: 'committed',
          });
        }
      }
    }

    return Array.from(fileMap.values()).sort((a, b) => a.path.localeCompare(b.path));
  }, [gitStatusFiles, cumulativeDiff]);

  // Compute stale files and reviewed files sets
  const { reviewedFiles, staleFiles } = useMemo(() => {
    const reviewed = new Set<string>();
    const stale = new Set<string>();

    for (const file of allFiles) {
      const reviewState = reviews.get(file.path);
      if (!reviewState?.reviewed) continue;

      const currentHash = hashDiff(file.diff);
      if (reviewState.diffHash && reviewState.diffHash !== currentHash) {
        stale.add(file.path);
      } else {
        reviewed.add(file.path);
      }
    }

    return { reviewedFiles: reviewed, staleFiles: stale };
  }, [allFiles, reviews]);

  // Compute comment counts per file
  const commentCountByFile = useMemo(() => {
    const counts: Record<string, number> = {};
    const sessionComments = bySession[sessionId];
    if (!sessionComments) return counts;

    for (const [filePath, comments] of Object.entries(sessionComments)) {
      if (comments.length > 0) {
        counts[filePath] = comments.length;
      }
    }
    return counts;
  }, [bySession, sessionId]);

  const totalCommentCount = useMemo(() => {
    return Object.values(commentCountByFile).reduce((sum, c) => sum + c, 0);
  }, [commentCountByFile]);

  // File refs for scrollIntoView
  const fileRefs = useMemo(() => {
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const file of allFiles) {
      refs.set(file.path, createRef<HTMLDivElement>());
    }
    return refs;
  }, [allFiles]);

  const handleToggleSplitView = useCallback((split: boolean) => {
    setSplitView(split);
    const mode = split ? 'split' : 'unified';
    localStorage.setItem('diff-view-mode', mode);
    window.dispatchEvent(new CustomEvent('diff-view-mode-change', { detail: mode }));
  }, []);

  const handleSelectFile = useCallback(
    (path: string) => {
      setSelectedFile(path);
      const ref = fileRefs.get(path);
      if (ref?.current) {
        ref.current.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
    },
    [fileRefs]
  );

  const handleToggleReviewed = useCallback(
    (path: string, reviewed: boolean) => {
      if (reviewed) {
        const file = allFiles.find((f) => f.path === path);
        const diffHash = file ? hashDiff(file.diff) : '';
        markReviewed(path, diffHash);
      } else {
        markUnreviewed(path);
      }
    },
    [allFiles, markReviewed, markUnreviewed]
  );

  const handleSendComments = useCallback(
    (comments: DiffComment[]) => {
      onSendComments(comments);
      onOpenChange(false);
    },
    [onSendComments, onOpenChange]
  );

  const handleDiscard = useCallback(
    async (path: string) => {
      try {
        const result = await discard([path]);
        if (result.success) {
          toast({ title: 'Changes discarded', description: path, variant: 'success' });
        } else {
          toast({ title: 'Discard failed', description: result.error || 'An error occurred', variant: 'error' });
        }
      } catch (e) {
        toast({ title: 'Discard failed', description: e instanceof Error ? e.message : 'An error occurred', variant: 'error' });
      }
    },
    [discard, toast]
  );

  const reviewedCount = reviewedFiles.size;
  const totalCount = allFiles.length;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-7xl max-w-[calc(100vw-2rem)] max-h-[85vh] w-full h-[85vh] p-0 gap-0 flex flex-col"
        showCloseButton={false}
        overlayClassName="backdrop-blur-none bg-black/40"
      >
        <DialogTitle className="sr-only">Review Changes</DialogTitle>
        {/* Top bar */}
        <ReviewTopBar
          sessionId={sessionId}
          reviewedCount={reviewedCount}
          totalCount={totalCount}
          commentCount={totalCommentCount}
          baseBranch={baseBranch}
          splitView={splitView}
          onToggleSplitView={handleToggleSplitView}
          wordWrap={wordWrap}
          onToggleWordWrap={setWordWrap}
          onSendComments={handleSendComments}
          onClose={() => onOpenChange(false)}
          getPendingComments={getPendingComments}
          markCommentsSent={markCommentsSent}
        />

        {/* Content area */}
        <div className="flex flex-1 min-h-0">
          {/* File tree sidebar */}
          <div className="w-[280px] min-w-[220px] border-r border-border flex-shrink-0 overflow-hidden">
            <ReviewFileTree
              files={allFiles}
              reviewedFiles={reviewedFiles}
              staleFiles={staleFiles}
              commentCountByFile={commentCountByFile}
              selectedFile={selectedFile}
              onSelectFile={handleSelectFile}
              onToggleReviewed={handleToggleReviewed}
            />
          </div>

          {/* Diff list */}
          <div className="flex-1 min-w-0 overflow-hidden">
            {allFiles.length > 0 ? (
              <ReviewDiffList
                files={allFiles}
                reviewedFiles={reviewedFiles}
                staleFiles={staleFiles}
                sessionId={sessionId}
                autoMarkOnScroll={autoMarkOnScroll}
                wordWrap={wordWrap}
                onToggleReviewed={handleToggleReviewed}
                onDiscard={handleDiscard}
                onOpenFile={onOpenFile}
                fileRefs={fileRefs}
              />
            ) : (
              <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                No changes to review
              </div>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
});
