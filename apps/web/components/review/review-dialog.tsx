"use client";

import { memo, useMemo, useCallback, createRef, useState, useRef, useEffect } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import type { DiffComment } from "@/lib/diff/types";
import type { FileInfo, CumulativeDiff } from "@/lib/state/slices/session-runtime/types";
import type { PRDiffFile } from "@/lib/types/github";
import { useCommentsStore, isDiffComment } from "@/lib/state/slices/comments";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { ReviewTopBar } from "./review-top-bar";
import { ReviewFileTree } from "./review-file-tree";
import { ReviewDiffList } from "./review-diff-list";
import type { ReviewFile } from "./types";
import {
  hashDiff,
  normalizeDiffContent,
  reviewFileKey,
  splitReviewFileKey as splitFileKey,
} from "./types";

/**
 * Multi-repo dedup: keying ReviewFile entries by `path` only collapses
 * `kandev/README.md` and `lvc/README.md` into a single row. Use the shared
 * `reviewFileKey` helper from `./types` so the in-memory state, persisted
 * reviews, and the file tree all agree on one key shape. `fileMapKey` is
 * the lower-level form for code that has the path + repo name as separate
 * variables (e.g. when building maps from `Object.entries` of the backend
 * payload).
 */
function fileMapKey(path: string, repositoryName?: string): string {
  return reviewFileKey({ path, repository_name: repositoryName });
}

function addCumulativeDiffFiles(
  fileMap: Map<string, ReviewFile>,
  files: CumulativeDiff["files"],
  gitStatusFiles: Record<string, FileInfo> | null,
) {
  // Multi-repo: backend always stamps each per-file payload with
  // `repository_name` + the repo-relative `path`. Single-repo (or legacy)
  // payloads keep the bare path on the map key. We trust `file.path` to be
  // set in the multi-repo shape — a missing `path` would be a backend
  // contract violation and indicates the caller is on an outdated agentctl;
  // surface it loudly via a console warning rather than silently injecting
  // the composite map key into the displayed path (which used to corrupt
  // the file tree node names).
  for (const [mapKey, file] of Object.entries(files)) {
    const repoName = file.repository_name;
    const path = file.path;
    if (!path) {
      console.warn("[review] cumulative diff entry missing `path` field; skipping", {
        mapKey,
        repoName,
      });
      continue;
    }
    const key = fileMapKey(path, repoName);
    if (fileMap.has(key)) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    if (diff) {
      const matchingUncommitted = findUncommittedByPathAndRepo(gitStatusFiles, path, repoName);
      fileMap.set(key, {
        path,
        diff,
        status: file.status || "modified",
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
        staged: matchingUncommitted?.staged ?? false,
        source: matchingUncommitted ? "uncommitted" : "committed",
        repository_name: repoName,
      });
    }
  }
}

/** Looks up a FileInfo in the (possibly composite-keyed) gitStatus map by
 *  (path, repository_name). When repo is undefined or empty (single-repo),
 *  returns the first match by path. */
function findUncommittedByPathAndRepo(
  gitStatusFiles: Record<string, FileInfo> | null,
  path: string,
  repositoryName: string | undefined,
): FileInfo | undefined {
  if (!gitStatusFiles) return undefined;
  for (const file of Object.values(gitStatusFiles)) {
    if (file.path !== path) continue;
    if (!repositoryName) return file;
    if (file.repository_name === repositoryName) return file;
  }
  return undefined;
}

function addUncommittedFiles(
  fileMap: Map<string, ReviewFile>,
  gitStatusFiles: Record<string, FileInfo>,
) {
  for (const file of Object.values(gitStatusFiles)) {
    const key = fileMapKey(file.path, file.repository_name);
    if (fileMap.has(key)) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    if (diff)
      fileMap.set(key, {
        path: file.path,
        diff,
        status: file.status,
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
        staged: file.staged,
        source: "uncommitted",
        repository_name: file.repository_name,
      });
  }
}

function prFileStatus(status: string): "added" | "deleted" | "modified" {
  if (status === "added") return "added";
  if (status === "removed") return "deleted";
  return "modified";
}

function addPRFiles(fileMap: Map<string, ReviewFile>, files: PRDiffFile[]) {
  for (const file of files) {
    if (fileMap.has(file.filename)) continue;
    const diff = file.patch ? normalizeDiffContent(file.patch) : "";
    if (diff)
      fileMap.set(file.filename, {
        path: file.filename,
        diff,
        status: prFileStatus(file.status),
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
        staged: false,
        source: "pr",
      });
  }
}

function buildAllFiles(
  gitStatusFiles: Record<string, FileInfo> | null,
  cumulativeDiff: CumulativeDiff | null,
  prDiffFiles?: PRDiffFile[],
): ReviewFile[] {
  const fileMap = new Map<string, ReviewFile>();
  if (prDiffFiles) addPRFiles(fileMap, prDiffFiles);
  if (cumulativeDiff?.files) addCumulativeDiffFiles(fileMap, cumulativeDiff.files, gitStatusFiles);
  if (gitStatusFiles) addUncommittedFiles(fileMap, gitStatusFiles);
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
  prDiffFiles?: PRDiffFile[];
};

function computeReviewSets(
  allFiles: ReviewFile[],
  reviews: ReturnType<typeof useSessionFileReviews>["reviews"],
) {
  const reviewed = new Set<string>();
  const stale = new Set<string>();
  for (const file of allFiles) {
    const key = reviewFileKey(file);
    const reviewState = reviews.get(key);
    if (!reviewState?.reviewed) continue;
    const currentHash = hashDiff(file.diff);
    if (reviewState.diffHash && reviewState.diffHash !== currentHash) stale.add(key);
    else reviewed.add(key);
  }
  return { reviewedFiles: reviewed, staleFiles: stale };
}

/**
 * Counts diff comments per file, scoped by repo when known. Multi-repo:
 * comments carrying `repositoryId` are matched only against files in that
 * repo (translated from `repository_name` via `repositoryNameToId`); legacy
 * comments without `repositoryId` and same-repo unattributed comments
 * match by path. Returned record is keyed by `reviewFileKey(file)` so the
 * file tree's per-row badge correctly disambiguates same-named files.
 */
function computeCommentCounts(
  byId: Record<string, import("@/lib/state/slices/comments").Comment>,
  sessionCommentIds: string[] | undefined,
  allFiles: ReviewFile[],
  repositoryNameToId: Map<string, string>,
): Record<string, number> {
  const counts: Record<string, number> = {};
  if (!sessionCommentIds) return counts;

  type BucketKey = string;
  const bucketKey = (repoId: string, path: string) => `${repoId}::${path}`;
  const bucket = new Map<BucketKey, number>();
  for (const id of sessionCommentIds) {
    const comment = byId[id];
    if (!comment || !isDiffComment(comment)) continue;
    const k = bucketKey(comment.repositoryId ?? "", comment.filePath);
    bucket.set(k, (bucket.get(k) ?? 0) + 1);
  }

  for (const file of allFiles) {
    let total = 0;
    if (file.repository_name) {
      const repoId = repositoryNameToId.get(file.repository_name) ?? "";
      if (repoId) total += bucket.get(bucketKey(repoId, file.path)) ?? 0;
    }
    // Path-only comments (legacy / pre-multi-repo data) match every file
    // sharing the path; this preserves the prior single-repo behavior and
    // keeps existing comments visible after the multi-repo rollout.
    total += bucket.get(bucketKey("", file.path)) ?? 0;
    if (total > 0) counts[reviewFileKey(file)] = total;
  }
  return counts;
}

type ReviewDialogHandlerOptions = {
  allFiles: ReviewFile[];
  markReviewed: (path: string, hash: string) => void;
  markUnreviewed: (path: string) => void;
  onSendComments: ReviewDialogProps["onSendComments"];
  onOpenChange: ReviewDialogProps["onOpenChange"];
  sessionId: string;
};

function useReviewDialogHandlers(opts: ReviewDialogHandlerOptions) {
  const { allFiles, markReviewed, markUnreviewed, onSendComments, onOpenChange, sessionId } = opts;
  const { discard } = useGitOperations(sessionId);
  const { toast } = useToast();

  const handleToggleSplitView = useCallback((split: boolean) => {
    const mode = split ? "split" : "unified";
    localStorage.setItem("diff-view-mode", mode);
    window.dispatchEvent(new CustomEvent("diff-view-mode-change", { detail: mode }));
  }, []);

  const handleSelectFile = useCallback((key: string, setSelectedFile: (k: string) => void) => {
    setSelectedFile(key);
    // Note: scrolling is now handled by FileDiffSection when isSelected changes
    // This ensures proper timing after the section expands
  }, []);

  const handleToggleReviewed = useCallback(
    (key: string, reviewed: boolean) => {
      if (reviewed) {
        // Look up by composite key so two same-name files in different repos
        // don't share their reviewed/diff-hash.
        const file = allFiles.find((f) => reviewFileKey(f) === key);
        markReviewed(key, file ? hashDiff(file.diff) : "");
      } else markUnreviewed(key);
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
    async (key: string) => {
      // Caller passes the composite key; split out the repo + path so
      // discard runs in the correct repo's worktree.
      const { repositoryName, path } = splitFileKey(key);
      try {
        const result = await discard([path], repositoryName || undefined);
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

  return {
    handleToggleSplitView,
    handleSelectFile,
    handleToggleReviewed,
    handleSendComments,
    handleDiscard,
  };
}

function useReviewDialogState(props: ReviewDialogProps) {
  const {
    open,
    onOpenChange,
    sessionId,
    onSendComments,
    gitStatusFiles,
    cumulativeDiff,
    prDiffFiles,
  } = props;
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

  const [filter, setFilter] = useState("");
  const allFiles = useMemo<ReviewFile[]>(
    () => buildAllFiles(gitStatusFiles, cumulativeDiff, prDiffFiles),
    [gitStatusFiles, cumulativeDiff, prDiffFiles],
  );
  const filteredFiles = useMemo(() => {
    if (!filter.trim()) return allFiles;
    const q = filter.toLowerCase();
    return allFiles.filter((f) => f.path.toLowerCase().includes(q));
  }, [allFiles, filter]);
  const { reviewedFiles, staleFiles } = useMemo(
    () => computeReviewSets(allFiles, reviews),
    [allFiles, reviews],
  );
  // Build a `repository_name` → `repositoryId` map from the workspace repos
  // slice. Comments store `repositoryId` (UUID) but ReviewFiles carry
  // `repository_name` (e.g. "kandev"); the dialog needs both to scope the
  // per-repo comment counts. Falls back to an empty map (legacy single-repo
  // counts by path only).
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const repositoryNameToId = useMemo(() => {
    const m = new Map<string, string>();
    for (const list of Object.values(reposByWorkspace)) {
      for (const r of list) m.set(r.name, r.id);
    }
    return m;
  }, [reposByWorkspace]);
  const commentCountByFile = useMemo(
    () => computeCommentCounts(byId, sessionCommentIds, allFiles, repositoryNameToId),
    [byId, sessionCommentIds, allFiles, repositoryNameToId],
  );
  const totalCommentCount = useMemo(
    () => Object.values(commentCountByFile).reduce((sum, c) => sum + c, 0),
    [commentCountByFile],
  );
  // Composite-key the file refs so two files with the same path in different
  // repos get distinct refs (otherwise auto-scroll-on-select / "selected"
  // styling lands on the wrong row).
  const fileRefs = useMemo(() => {
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const file of allFiles) refs.set(reviewFileKey(file), createRef<HTMLDivElement>());
    return refs;
  }, [allFiles]);
  const prevCountRef = useRef<number | null>(null);

  useEffect(() => {
    const prevCount = prevCountRef.current;
    if (open && prevCount !== null && prevCount > 0 && allFiles.length === 0) onOpenChange(false);
    prevCountRef.current = allFiles.length;
  }, [open, allFiles.length, onOpenChange]);

  const handlers = useReviewDialogHandlers({
    allFiles,
    markReviewed,
    markUnreviewed,
    onSendComments,
    onOpenChange,
    sessionId,
  });

  return {
    selectedFile,
    splitView,
    wordWrap,
    setWordWrap,
    autoMarkOnScroll,
    filter,
    setFilter,
    allFiles,
    filteredFiles,
    reviewedFiles,
    staleFiles,
    commentCountByFile,
    totalCommentCount,
    fileRefs,
    getPendingComments,
    markCommentsSent,
    handleToggleSplitView: (split: boolean) => {
      setSplitView(split);
      handlers.handleToggleSplitView(split);
    },
    handleSelectFile: (path: string) => handlers.handleSelectFile(path, setSelectedFile),
    handleToggleReviewed: handlers.handleToggleReviewed,
    handleSendComments: handlers.handleSendComments,
    handleDiscard: handlers.handleDiscard,
  };
}

export const ReviewDialog = memo(function ReviewDialog(props: ReviewDialogProps) {
  const { open, onOpenChange, sessionId, baseBranch, onOpenFile } = props;
  const s = useReviewDialogState(props);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="!max-w-[100vw] !w-[100vw] sm:!max-w-[80vw] sm:!w-[80vw] max-h-[85vh] h-[85vh] p-0 gap-0 flex flex-col shadow-2xl"
        showCloseButton={false}
        overlayClassName="bg-transparent"
      >
        <DialogTitle className="sr-only">Review Changes</DialogTitle>
        <ReviewTopBar
          sessionId={sessionId}
          reviewedCount={s.reviewedFiles.size}
          totalCount={s.allFiles.length}
          commentCount={s.totalCommentCount}
          baseBranch={baseBranch}
          splitView={s.splitView}
          onToggleSplitView={s.handleToggleSplitView}
          wordWrap={s.wordWrap}
          onToggleWordWrap={s.setWordWrap}
          onSendComments={s.handleSendComments}
          onClose={() => onOpenChange(false)}
          getPendingComments={s.getPendingComments}
          markCommentsSent={s.markCommentsSent}
        />
        <div className="flex flex-1 min-h-0">
          <div className="w-[280px] sm:w-[220px] min-w-[100px] border-r border-border flex-shrink-0 overflow-hidden hidden sm:flex flex-col">
            <ReviewFileTree
              files={s.filteredFiles}
              reviewedFiles={s.reviewedFiles}
              staleFiles={s.staleFiles}
              commentCountByFile={s.commentCountByFile}
              selectedFile={s.selectedFile}
              filter={s.filter}
              onFilterChange={s.setFilter}
              onSelectFile={s.handleSelectFile}
              onToggleReviewed={s.handleToggleReviewed}
            />
          </div>
          <div className="flex-1 min-w-0 overflow-hidden">
            {s.filteredFiles.length > 0 ? (
              <ReviewDiffList
                files={s.filteredFiles}
                selectedFile={s.selectedFile}
                reviewedFiles={s.reviewedFiles}
                staleFiles={s.staleFiles}
                sessionId={sessionId}
                autoMarkOnScroll={s.autoMarkOnScroll}
                wordWrap={s.wordWrap}
                onToggleReviewed={s.handleToggleReviewed}
                onDiscard={s.handleDiscard}
                onOpenFile={onOpenFile}
                fileRefs={s.fileRefs}
              />
            ) : (
              <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                {s.filter.trim() ? "No files match the filter" : "No changes to review"}
              </div>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
});
