"use client";

import { memo, useMemo, useCallback, createRef, useState, useRef, useEffect } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import type { DiffComment } from "@/lib/diff/types";
import type { FileInfo, CumulativeDiff } from "@/lib/state/slices/session-runtime/types";
import { useCommentsStore, isDiffComment } from "@/lib/state/slices/comments";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { ReviewTopBar } from "./review-top-bar";
import { ReviewFileTree } from "./review-file-tree";
import { ReviewDiffList } from "./review-diff-list";
import type { ReviewFile } from "./types";
import { hashDiff, normalizeDiffContent } from "./types";

function addUncommittedFiles(
  fileMap: Map<string, ReviewFile>,
  gitStatusFiles: Record<string, FileInfo>,
) {
  for (const [path, file] of Object.entries(gitStatusFiles)) {
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    if (diff)
      fileMap.set(path, {
        path,
        diff,
        status: file.status,
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
        staged: file.staged,
        source: "uncommitted",
      });
  }
}

function addCommittedFiles(fileMap: Map<string, ReviewFile>, files: CumulativeDiff["files"]) {
  for (const [path, file] of Object.entries(files)) {
    if (fileMap.has(path)) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    if (diff)
      fileMap.set(path, {
        path,
        diff,
        status: file.status || "modified",
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
        staged: false,
        source: "committed",
      });
  }
}

function buildAllFiles(
  gitStatusFiles: Record<string, FileInfo> | null,
  cumulativeDiff: CumulativeDiff | null,
): ReviewFile[] {
  const fileMap = new Map<string, ReviewFile>();
  if (gitStatusFiles) addUncommittedFiles(fileMap, gitStatusFiles);
  if (cumulativeDiff?.files) addCommittedFiles(fileMap, cumulativeDiff.files);
  return Array.from(fileMap.values()).sort((a, b) => a.path.localeCompare(b.path));
}

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

function computeReviewSets(
  allFiles: ReviewFile[],
  reviews: ReturnType<typeof useSessionFileReviews>["reviews"],
) {
  const reviewed = new Set<string>();
  const stale = new Set<string>();
  for (const file of allFiles) {
    const reviewState = reviews.get(file.path);
    if (!reviewState?.reviewed) continue;
    const currentHash = hashDiff(file.diff);
    if (reviewState.diffHash && reviewState.diffHash !== currentHash) stale.add(file.path);
    else reviewed.add(file.path);
  }
  return { reviewedFiles: reviewed, staleFiles: stale };
}

function computeCommentCounts(
  byId: Record<string, import("@/lib/state/slices/comments").Comment>,
  sessionCommentIds: string[] | undefined,
): Record<string, number> {
  const counts: Record<string, number> = {};
  if (!sessionCommentIds) return counts;
  for (const id of sessionCommentIds) {
    const comment = byId[id];
    if (comment && isDiffComment(comment)) {
      counts[comment.filePath] = (counts[comment.filePath] ?? 0) + 1;
    }
  }
  return counts;
}

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
  const [splitView, setSplitView] = useState(() =>
    typeof window === "undefined" ? false : localStorage.getItem("diff-view-mode") === "split",
  );
  const [wordWrap, setWordWrap] = useState(false);
  const autoMarkOnScroll = useAppStore((s) => s.userSettings.reviewAutoMarkOnScroll);
  const { reviews, markReviewed, markUnreviewed } = useSessionFileReviews(sessionId);
  const byId = useCommentsStore((s) => s.byId);
  const sessionCommentIds = useCommentsStore((s) => s.bySession[sessionId]);
  const getStorePendingComments = useCommentsStore((s) => s.getPendingComments);
  const getPendingComments = useCallback((): DiffComment[] => {
    return getStorePendingComments().filter(isDiffComment);
  }, [getStorePendingComments]);
  const markCommentsSent = useCommentsStore((s) => s.markCommentsSent);
  const { discard } = useGitOperations(sessionId);
  const { toast } = useToast();

  const allFiles = useMemo<ReviewFile[]>(
    () => buildAllFiles(gitStatusFiles, cumulativeDiff),
    [gitStatusFiles, cumulativeDiff],
  );
  const { reviewedFiles, staleFiles } = useMemo(
    () => computeReviewSets(allFiles, reviews),
    [allFiles, reviews],
  );
  const commentCountByFile = useMemo(
    () => computeCommentCounts(byId, sessionCommentIds),
    [byId, sessionCommentIds],
  );
  const totalCommentCount = useMemo(
    () => Object.values(commentCountByFile).reduce((sum, c) => sum + c, 0),
    [commentCountByFile],
  );
  const fileRefs = useMemo(() => {
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const file of allFiles) refs.set(file.path, createRef<HTMLDivElement>());
    return refs;
  }, [allFiles]);
  const prevCountRef = useRef<number | null>(null);

  useEffect(() => {
    const prevCount = prevCountRef.current;
    if (open && prevCount !== null && prevCount > 0 && allFiles.length === 0) {
      onOpenChange(false);
    }
    prevCountRef.current = allFiles.length;
  }, [open, allFiles.length, onOpenChange]);

  const handleToggleSplitView = useCallback((split: boolean) => {
    setSplitView(split);
    const mode = split ? "split" : "unified";
    localStorage.setItem("diff-view-mode", mode);
    window.dispatchEvent(new CustomEvent("diff-view-mode-change", { detail: mode }));
  }, []);

  const handleSelectFile = useCallback(
    (path: string) => {
      setSelectedFile(path);
      const ref = fileRefs.get(path);
      if (ref?.current) ref.current.scrollIntoView({ behavior: "smooth", block: "start" });
    },
    [fileRefs],
  );

  const handleToggleReviewed = useCallback(
    (path: string, reviewed: boolean) => {
      if (reviewed) {
        const file = allFiles.find((f) => f.path === path);
        markReviewed(path, file ? hashDiff(file.diff) : "");
      } else markUnreviewed(path);
    },
    [allFiles, markReviewed, markUnreviewed],
  );

  const handleSendComments = useCallback(
    (comments: DiffComment[]) => {
      onSendComments(comments);
      onOpenChange(false);
    },
    [onSendComments, onOpenChange],
  );

  const handleDiscard = useCallback(
    async (path: string) => {
      try {
        const result = await discard([path]);
        if (result.success)
          toast({ title: "Changes discarded", description: path, variant: "success" });
        else
          toast({
            title: "Discard failed",
            description: result.error || "An error occurred",
            variant: "error",
          });
      } catch (e) {
        toast({
          title: "Discard failed",
          description: e instanceof Error ? e.message : "An error occurred",
          variant: "error",
        });
      }
    },
    [discard, toast],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-7xl max-w-[calc(100vw-2rem)] max-h-[85vh] w-full h-[85vh] p-0 gap-0 flex flex-col"
        showCloseButton={false}
        overlayClassName="backdrop-blur-none bg-black/40"
      >
        <DialogTitle className="sr-only">Review Changes</DialogTitle>
        <ReviewTopBar
          sessionId={sessionId}
          reviewedCount={reviewedFiles.size}
          totalCount={allFiles.length}
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
        <div className="flex flex-1 min-h-0">
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
